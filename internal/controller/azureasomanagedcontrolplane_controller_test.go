/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20231001"
	asoannotations "github.com/Azure/azure-service-operator/v2/pkg/common/annotations"
	"github.com/Azure/azure-service-operator/v2/pkg/genruntime"
	"github.com/Azure/azure-service-operator/v2/pkg/genruntime/conditions"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-azure/internal/mutators"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/secret"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAzureASOManagedControlPlaneReconcile(t *testing.T) {
	ctx := context.Background()

	s := runtime.NewScheme()
	sb := runtime.NewSchemeBuilder(
		corev1.AddToScheme,
		infrav1.AddToScheme,
		clusterv1.AddToScheme,
		expv1.AddToScheme,
		asocontainerservicev1.AddToScheme,
	)
	t.Run("build scheme", expectSuccess(sb.AddToScheme(s)))
	fakeClientBuilder := func() *fakeclient.ClientBuilder {
		return fakeclient.NewClientBuilder().
			WithScheme(s).
			WithStatusSubresource(&infrav1.AzureASOManagedControlPlane{})
	}

	t.Run("AzureASOManagedControlPlane does not exist", func(t *testing.T) {
		c := fakeClientBuilder().
			Build()
		r := &AzureASOManagedControlPlaneReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "doesn't", Name: "exist"}})
		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
	})

	t.Run("no Cluster ownerref", func(t *testing.T) {
		asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "amcp",
				Namespace:  "ns",
				Generation: 1,
			},
		}
		c := fakeClientBuilder().
			WithObjects(asoManagedControlPlane).
			Build()

		r := &AzureASOManagedControlPlaneReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedControlPlane)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update status.observedGeneration", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedControlPlane), asoManagedControlPlane)))
			t.Run("metadata.generation", checkEqual(asoManagedControlPlane.Generation, 1))
			t.Run("status.observedGeneration", checkEqual(asoManagedControlPlane.Status.ObservedGeneration, 1))
		})
	})

	t.Run("Cluster does not exist", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
		}
		asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amcp",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
					},
				},
				Generation: 1,
			},
		}
		c := fakeClientBuilder().
			WithObjects(asoManagedControlPlane).
			Build()

		r := &AzureASOManagedControlPlaneReconciler{
			Client: c,
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedControlPlane)})

		t.Run("should fail", checkEqual(apierrors.IsNotFound(err), true))
		t.Run("should not update status.observedGeneration", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedControlPlane), asoManagedControlPlane)))
			t.Run("metadata.generation", checkEqual(asoManagedControlPlane.Generation, 1))
			t.Run("status.observedGeneration", checkEqual(asoManagedControlPlane.Status.ObservedGeneration, 0))
		})
	})

	t.Run("Cluster does not also use AzureASOManagedControlPlane", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				InfrastructureRef: &corev1.ObjectReference{
					Kind: "not-AzureASOManagedCluster",
				},
			},
		}
		asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amcp",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
					},
				},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedControlPlane).
			Build()

		r := &AzureASOManagedControlPlaneReconciler{
			Client: c,
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedControlPlane)})

		t.Run("should fail", checkEqual(errors.Is(err, invalidClusterKindErr), true))
	})

	t.Run("Finalizer and block-move annotation are added", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				InfrastructureRef: &corev1.ObjectReference{
					Kind: "AzureASOManagedCluster",
				},
			},
		}
		asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amcp",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
					},
				},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedControlPlane).
			Build()

		r := &AzureASOManagedControlPlaneReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedControlPlane)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should requeue", checkEqual(result, ctrl.Result{Requeue: true}))
		t.Run("should update the resource", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedControlPlane), asoManagedControlPlane)))
			t.Run("metadata.finalizers[0]", checkEqual(asoManagedControlPlane.Finalizers[0], clusterv1.ClusterFinalizer))
			_, hasBlockMove := asoManagedControlPlane.Annotations[clusterctlv1.BlockMoveAnnotation]
			t.Run("has block-move annotation", checkEqual(hasBlockMove, true))
		})
	})

	t.Run("no ManagedCluster defined", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				InfrastructureRef: &corev1.ObjectReference{
					Kind: "AzureASOManagedCluster",
				},
			},
		}
		asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amcp",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
					},
				},
				Finalizers: []string{clusterv1.ClusterFinalizer},
				Annotations: map[string]string{
					clusterctlv1.BlockMoveAnnotation: "true",
				},
			},
			Spec: infrav1.AzureASOManagedControlPlaneSpec{
				AzureASOManagedControlPlaneTemplateResourceSpec: infrav1.AzureASOManagedControlPlaneTemplateResourceSpec{
					Resources: []runtime.RawExtension{},
				},
			},
			Status: infrav1.AzureASOManagedControlPlaneStatus{
				Ready: true,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedControlPlane).
			Build()

		r := &AzureASOManagedControlPlaneReconciler{
			Client: c,
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedControlPlane)})

		t.Run("should fail", checkEqual(errors.Is(err, mutators.NoManagedClusterDefinedErr), true))
	})

	t.Run("ManagedCluster is not ready", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				InfrastructureRef: &corev1.ObjectReference{
					Kind: "AzureASOManagedCluster",
				},
			},
		}
		mc := &asocontainerservicev1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mc",
				Namespace: cluster.Namespace,
			},
			Status: asocontainerservicev1.ManagedCluster_STATUS{
				AgentPoolProfiles: []asocontainerservicev1.ManagedClusterAgentPoolProfile_STATUS{{}},
			},
		}
		asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amcp",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
					},
				},
				Finalizers: []string{clusterv1.ClusterFinalizer},
				Annotations: map[string]string{
					clusterctlv1.BlockMoveAnnotation: "true",
				},
			},
			Spec: infrav1.AzureASOManagedControlPlaneSpec{
				AzureASOManagedControlPlaneTemplateResourceSpec: infrav1.AzureASOManagedControlPlaneTemplateResourceSpec{
					Resources: []runtime.RawExtension{
						{
							Raw: mcJSON(t, &asocontainerservicev1.ManagedCluster{
								ObjectMeta: metav1.ObjectMeta{
									Name: "mc",
								},
							}),
						},
					},
				},
			},
			Status: infrav1.AzureASOManagedControlPlaneStatus{
				Ready: true,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedControlPlane, mc).
			Build()

		r := &AzureASOManagedControlPlaneReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedControlPlane *infrav1.AzureASOManagedControlPlane, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					owner: asoManagedControlPlane,
					reconcileFunc: func(_ context.Context, asoManagedControlPlane resourceStatusObject) error {
						asoManagedControlPlane.SetResourceStatuses([]infrav1.ResourceStatus{
							{Condition: conditions.Condition{Status: metav1.ConditionFalse}},
						})
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedControlPlane)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update the AzureASOManagedControlPlane", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedControlPlane), asoManagedControlPlane)))
			t.Run("status.ready", checkEqual(asoManagedControlPlane.Status.Ready, false))
		})
	})

	t.Run("successful reconcile", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				InfrastructureRef: &corev1.ObjectReference{
					Kind: "AzureASOManagedCluster",
				},
			},
		}
		mp := &expv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mp",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
			},
			Spec: expv1.MachinePoolSpec{
				Replicas: ptr.To[int32](5),
			},
		}
		asoManagedMachinePool := &infrav1.AzureASOManagedMachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pool0",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: expv1.GroupVersion.Identifier(),
						Kind:       "MachinePool",
						Name:       mp.Name,
					},
				},
			},
			Spec: infrav1.AzureASOManagedMachinePoolSpec{
				AzureASOManagedMachinePoolTemplateResourceSpec: infrav1.AzureASOManagedMachinePoolTemplateResourceSpec{
					Resources: []runtime.RawExtension{
						{
							Raw: apJSON(t, &asocontainerservicev1.ManagedClustersAgentPool{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "pool0",
									Namespace: cluster.Namespace,
								},
							}),
						},
					},
				},
			},
		}
		mc := &asocontainerservicev1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mc",
				Namespace: cluster.Namespace,
			},
			Spec: asocontainerservicev1.ManagedCluster_Spec{
				OperatorSpec: &asocontainerservicev1.ManagedClusterOperatorSpec{
					Secrets: &asocontainerservicev1.ManagedClusterOperatorSecrets{
						AdminCredentials: &genruntime.SecretDestination{
							Name: secret.Name(cluster.Name, secret.Kubeconfig),
							Key:  secret.KubeconfigDataName,
						},
					},
				},
			},
			Status: asocontainerservicev1.ManagedCluster_STATUS{
				Fqdn:                     ptr.To("fqdn"),
				CurrentKubernetesVersion: ptr.To("0.0.0"),
			},
		}
		kubeconfig := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mc.Spec.OperatorSpec.Secrets.AdminCredentials.Name,
				Namespace: cluster.Namespace,
			},
			Data: map[string][]byte{
				mc.Spec.OperatorSpec.Secrets.AdminCredentials.Key: []byte("some data"),
			},
		}
		asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amcp",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
					},
				},
				Finalizers: []string{clusterv1.ClusterFinalizer},
				Annotations: map[string]string{
					clusterctlv1.BlockMoveAnnotation: "true",
				},
			},
			Spec: infrav1.AzureASOManagedControlPlaneSpec{
				AzureASOManagedControlPlaneTemplateResourceSpec: infrav1.AzureASOManagedControlPlaneTemplateResourceSpec{
					Resources: []runtime.RawExtension{
						{
							Raw: mcJSON(t, &asocontainerservicev1.ManagedCluster{
								ObjectMeta: metav1.ObjectMeta{
									Name: "mc",
								},
							}),
						},
					},
				},
			},
			Status: infrav1.AzureASOManagedControlPlaneStatus{
				Ready: false,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedControlPlane, asoManagedMachinePool, mp, mc, kubeconfig).
			Build()

		r := &AzureASOManagedControlPlaneReconciler{
			Client: &FakeClient{
				Client: c,
				patchFunc: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					if patch != client.Apply {
						return c.Patch(ctx, obj, patch, opts...)
					}
					t.Run("kubeconfig patch", func(t *testing.T) {
						kubeconfig := obj.(*corev1.Secret)
						t.Run("name", checkEqual(kubeconfig.GetName(), secret.Name(cluster.Name, secret.Kubeconfig)))
						t.Run("value", checkEqual(string(kubeconfig.Data[secret.KubeconfigDataName]), "some data"))
					})
					return nil
				},
			},
			newResourceReconciler: func(asoManagedControlPlane *infrav1.AzureASOManagedControlPlane, resources []*unstructured.Unstructured) resourceReconciler {
				t.Run("reconciled resources", func(t *testing.T) {
					for _, u := range resources {
						if u.GroupVersionKind().Group != asocontainerservicev1.GroupVersion.Group ||
							u.GroupVersionKind().Kind != "ManagedCluster" {
							continue
						}
						t.Run("managedcluster", func(t *testing.T) {
							t.Run("spec.agentPoolProfiles", func(t *testing.T) {
								agentPoolProfiles, found, err := unstructured.NestedSlice(u.UnstructuredContent(), "spec", "agentPoolProfiles")
								t.Run("exists", checkEqual(found, true))
								t.Run("is slice", expectSuccess(err))
								t.Run("length", checkEqual(len(agentPoolProfiles), 1))
								t.Run("0", func(t *testing.T) {
									pool := agentPoolProfiles[0]
									t.Run("count", func(t *testing.T) {
										count, found, err := unstructured.NestedInt64(pool.(map[string]interface{}), "count")
										t.Run("exists", checkEqual(found, true))
										t.Run("is int", expectSuccess(err))
										t.Run("value", checkEqual(count, 5))
									})
								})
							})
						})
					}
				})
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, asoManagedControlPlane resourceStatusObject) error {
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedControlPlane)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update the AzureASOManagedControlPlane", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedControlPlane), asoManagedControlPlane)))
			t.Run("status.version", checkEqual(asoManagedControlPlane.Status.Version, "v0.0.0"))
			t.Run("status.ready", checkEqual(asoManagedControlPlane.Status.Ready, true))
			t.Run("status.initialized", checkEqual(asoManagedControlPlane.Status.Initialized, true))
			_, hasBlockMove := asoManagedControlPlane.Annotations[clusterctlv1.BlockMoveAnnotation]
			t.Run("has block-move annotation", checkEqual(hasBlockMove, true))
		})
	})

	t.Run("Cluster is paused", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				Paused: true,
				InfrastructureRef: &corev1.ObjectReference{
					Kind: "AzureASOManagedCluster",
				},
			},
		}
		mc := &asocontainerservicev1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mc",
				Namespace: cluster.Namespace,
			},
			Spec: asocontainerservicev1.ManagedCluster_Spec{
				OperatorSpec: &asocontainerservicev1.ManagedClusterOperatorSpec{
					Secrets: &asocontainerservicev1.ManagedClusterOperatorSecrets{
						AdminCredentials: &genruntime.SecretDestination{
							Name: secret.Name(cluster.Name, secret.Kubeconfig),
							Key:  secret.KubeconfigDataName,
						},
					},
				},
			},
			Status: asocontainerservicev1.ManagedCluster_STATUS{
				AgentPoolProfiles: []asocontainerservicev1.ManagedClusterAgentPoolProfile_STATUS{{}},
			},
		}
		kubeconfig := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mc.Spec.OperatorSpec.Secrets.AdminCredentials.Name,
				Namespace: cluster.Namespace,
			},
			Data: map[string][]byte{
				mc.Spec.OperatorSpec.Secrets.AdminCredentials.Key: []byte("some data"),
			},
		}
		asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amcp",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
					},
				},
				Finalizers: []string{clusterv1.ClusterFinalizer},
				Annotations: map[string]string{
					clusterctlv1.BlockMoveAnnotation: "true",
				},
			},
			Spec: infrav1.AzureASOManagedControlPlaneSpec{
				AzureASOManagedControlPlaneTemplateResourceSpec: infrav1.AzureASOManagedControlPlaneTemplateResourceSpec{
					Resources: []runtime.RawExtension{
						{
							Raw: mcJSON(t, &asocontainerservicev1.ManagedCluster{
								ObjectMeta: metav1.ObjectMeta{
									Name: "mc",
								},
							}),
						},
					},
				},
			},
			Status: infrav1.AzureASOManagedControlPlaneStatus{
				Ready: false,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedControlPlane, mc, kubeconfig).
			Build()

		r := &AzureASOManagedControlPlaneReconciler{
			Client: &FakeClient{
				Client: c,
				patchFunc: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					if patch != client.Apply {
						return c.Patch(ctx, obj, patch, opts...)
					}
					return nil
				},
			},
			newResourceReconciler: func(asoManagedControlPlane *infrav1.AzureASOManagedControlPlane, resources []*unstructured.Unstructured) resourceReconciler {
				t.Run("reconciled resources", func(t *testing.T) {
					for _, u := range resources {
						if u.GroupVersionKind().Group != asocontainerservicev1.GroupVersion.Group ||
							u.GroupVersionKind().Kind != "ManagedCluster" {
							continue
						}
						t.Run("managedcluster", func(t *testing.T) {
							t.Run("has reconcile-policy skip", checkEqual(u.GetAnnotations()[asoannotations.ReconcilePolicy], string(asoannotations.ReconcilePolicySkip)))
						})
					}
				})
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, asoManagedControlPlane resourceStatusObject) error {
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedControlPlane)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update the AzureASOManagedControlPlane", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedControlPlane), asoManagedControlPlane)))
			_, hasBlockMove := asoManagedControlPlane.Annotations[clusterctlv1.BlockMoveAnnotation]
			t.Run("has no block-move annotation", checkEqual(hasBlockMove, false))
		})
	})

	t.Run("Delete without Cluster ownerref", func(t *testing.T) {
		asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "amcp",
				Namespace:         "ns",
				OwnerReferences:   []metav1.OwnerReference{},
				Finalizers:        []string{clusterv1.ClusterFinalizer},
			},
		}
		c := fakeClientBuilder().
			WithObjects(asoManagedControlPlane).
			Build()

		r := &AzureASOManagedControlPlaneReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedControlPlane)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should delete the AzureASOManagedCluster", checkEqual(apierrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(asoManagedControlPlane), asoManagedControlPlane)), true))
	})

	t.Run("Delete error", func(t *testing.T) {
		namespace := "ns"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "cluster",
			},
		}
		asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "amcp",
				Namespace:         cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
					},
				},
				Finalizers: []string{clusterv1.ClusterFinalizer},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedControlPlane).
			Build()

		deleteErr := errors.New("delete error")
		r := &AzureASOManagedControlPlaneReconciler{
			Client: c,
			newResourceReconciler: func(_ *infrav1.AzureASOManagedControlPlane, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					deleteFunc: func(_ context.Context, _ resourceStatusObject) error {
						return deleteErr
					},
				}
			},
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedControlPlane)})

		t.Run("should fail", checkEqual(errors.Is(err, deleteErr), true))
		t.Run("should not remove the finalizer", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedControlPlane), asoManagedControlPlane)))
			t.Run("finalizers length", checkEqual(len(asoManagedControlPlane.Finalizers), 1))
		})
	})

	t.Run("Delete in progress", func(t *testing.T) {
		namespace := "ns"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "cluster",
			},
		}
		asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "amcp",
				Namespace:         cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
					},
				},
				Finalizers: []string{clusterv1.ClusterFinalizer},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedControlPlane).
			Build()

		r := &AzureASOManagedControlPlaneReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedControlPlane *infrav1.AzureASOManagedControlPlane, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					owner: asoManagedControlPlane,
					deleteFunc: func(_ context.Context, asoManagedControlPlane resourceStatusObject) error {
						asoManagedControlPlane.SetResourceStatuses([]infrav1.ResourceStatus{
							{Condition: conditions.Condition{Status: metav1.ConditionFalse}},
						})
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedControlPlane)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should not remove the finalizer", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedControlPlane), asoManagedControlPlane)))
			t.Run("finalizers length", checkEqual(len(asoManagedControlPlane.Finalizers), 1))
		})
	})

	t.Run("Delete succeeds", func(t *testing.T) {
		namespace := "ns"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "cluster",
			},
		}
		asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "amcp",
				Namespace:         cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
					},
				},
				Finalizers: []string{clusterv1.ClusterFinalizer},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedControlPlane).
			Build()

		r := &AzureASOManagedControlPlaneReconciler{
			Client: c,
			newResourceReconciler: func(_ *infrav1.AzureASOManagedControlPlane, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					deleteFunc: func(_ context.Context, _ resourceStatusObject) error {
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedControlPlane)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should delete the AzureASOManagedCluster", checkEqual(apierrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(asoManagedControlPlane), asoManagedControlPlane)), true))
	})
}

func mcJSON(t *testing.T, rg *asocontainerservicev1.ManagedCluster) []byte {
	t.Helper()
	rg.SetGroupVersionKind(asocontainerservicev1.GroupVersion.WithKind("ManagedCluster"))
	j, err := json.Marshal(rg)
	t.Run("marshal managed cluster", expectSuccess(err))
	return j
}

func apJSON(t *testing.T, ap *asocontainerservicev1.ManagedClustersAgentPool) []byte {
	t.Helper()
	ap.SetGroupVersionKind(asocontainerservicev1.GroupVersion.WithKind("ManagedClustersAgentPool"))
	j, err := json.Marshal(ap)
	t.Run("marshal agent pool", expectSuccess(err))
	return j
}

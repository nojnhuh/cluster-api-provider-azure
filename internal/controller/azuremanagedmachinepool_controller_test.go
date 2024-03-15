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
	"testing"
	"time"

	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20231001"
	"github.com/Azure/azure-service-operator/v2/pkg/genruntime"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/v2/api/v2alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type FakeClusterTracker struct {
	getClientFunc func(context.Context, types.NamespacedName) (client.Client, error)
}

func (c *FakeClusterTracker) GetClient(ctx context.Context, name types.NamespacedName) (client.Client, error) {
	if c.getClientFunc == nil {
		return nil, nil
	}
	return c.getClientFunc(ctx, name)
}

func TestAzureManagedMachinePoolReconcile(t *testing.T) {
	ctx := context.Background()

	s := runtime.NewScheme()
	sb := runtime.NewSchemeBuilder(
		infrav1.AddToScheme,
		clusterv1.AddToScheme,
		expv1.AddToScheme,
		asocontainerservicev1.AddToScheme,
	)
	t.Run("build scheme", expectSuccess(sb.AddToScheme(s)))
	fakeClientBuilder := func() *fakeclient.ClientBuilder {
		return fakeclient.NewClientBuilder().
			WithScheme(s).
			WithStatusSubresource(&infrav1.AzureManagedMachinePool{})
	}

	t.Run("AzureManagedMachinePool does not exist", func(t *testing.T) {
		c := fakeClientBuilder().
			Build()
		r := &AzureManagedMachinePoolReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "doesn't", Name: "exist"}})
		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
	})

	t.Run("no MachinePool ownerref", func(t *testing.T) {
		azureManagedMachinePool := &infrav1.AzureManagedMachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "ammp",
				Namespace:       "ns",
				OwnerReferences: []metav1.OwnerReference{},
				Generation:      1,
			},
		}
		c := fakeClientBuilder().
			WithObjects(azureManagedMachinePool).
			Build()
		r := &AzureManagedMachinePoolReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedMachinePool)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update status.observedGeneration", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedMachinePool), azureManagedMachinePool)))
			t.Run("metadata.generation", checkEqual(azureManagedMachinePool.Generation, 1))
			t.Run("status.observedGeneration", checkEqual(azureManagedMachinePool.Status.ObservedGeneration, 1))
		})
	})

	t.Run("MachinePool does not exist", func(t *testing.T) {
		azureManagedMachinePool := &infrav1.AzureManagedMachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ammp",
				Namespace: "ns",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: expv1.GroupVersion.Identifier(),
						Kind:       "MachinePool",
						Name:       "doesnotexist",
					},
				},
				Generation: 1,
			},
		}
		c := fakeClientBuilder().
			WithObjects(azureManagedMachinePool).
			Build()
		r := &AzureManagedMachinePoolReconciler{
			Client: c,
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedMachinePool)})

		t.Run("should fail", checkEqual(apierrors.IsNotFound(err), true))
		t.Run("should not update status.observedGeneration", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedMachinePool), azureManagedMachinePool)))
			t.Run("metadata.generation", checkEqual(azureManagedMachinePool.Generation, 1))
			t.Run("status.observedGeneration", checkEqual(azureManagedMachinePool.Status.ObservedGeneration, 0))
		})
	})

	t.Run("adds finalizer and block-move annotation", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "ns",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind: "AzureManagedControlPlane",
				},
			},
		}
		machinePool := &expv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mp",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
			},
		}
		azureManagedMachinePool := &infrav1.AzureManagedMachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ammp",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: expv1.GroupVersion.Identifier(),
						Kind:       "MachinePool",
						Name:       machinePool.Name,
					},
				},
			},
		}
		c := fakeClientBuilder().
			WithObjects(azureManagedMachinePool, cluster, machinePool).
			Build()
		r := &AzureManagedMachinePoolReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedMachinePool)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should requeue", checkEqual(result, ctrl.Result{Requeue: true}))
		t.Run("should update the resource", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedMachinePool), azureManagedMachinePool)))
			t.Run("metadata.finalizers[0]", checkEqual(azureManagedMachinePool.Finalizers[0], infrav1.AzureManagedMachinePoolFinalizer))
			_, hasBlockMove := azureManagedMachinePool.Annotations[clusterctlv1.BlockMoveAnnotation]
			t.Run("has block-move annotation", checkEqual(hasBlockMove, true))
		})
	})

	t.Run("reconciled resources not ready", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "ns",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind: "AzureManagedControlPlane",
				},
			},
		}
		machinePool := &expv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mp",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
				Annotations: map[string]string{
					clusterv1.ReplicasManagedByAnnotation: "something",
				},
			},
			Spec: expv1.MachinePoolSpec{
				Replicas: ptr.To[int32](2),
			},
		}
		azureManagedMachinePool := &infrav1.AzureManagedMachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ammp",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: expv1.GroupVersion.Identifier(),
						Kind:       "MachinePool",
						Name:       machinePool.Name,
					},
				},
				Finalizers: []string{infrav1.AzureManagedMachinePoolFinalizer},
				Annotations: map[string]string{
					clusterctlv1.BlockMoveAnnotation: "true",
				},
			},
			Spec: infrav1.AzureManagedMachinePoolSpec{
				AzureManagedMachinePoolTemplateResourceSpec: infrav1.AzureManagedMachinePoolTemplateResourceSpec{
					Resources: []runtime.RawExtension{
						{
							Raw: apJSON(t, &asocontainerservicev1.ManagedClustersAgentPool{
								ObjectMeta: metav1.ObjectMeta{
									Name: "pool0",
								},
							}),
						},
					},
				},
			},
			Status: infrav1.AzureManagedMachinePoolStatus{
				Ready: true,
			},
		}
		c := fakeClientBuilder().
			WithObjects(azureManagedMachinePool, cluster, machinePool).
			Build()
		r := &AzureManagedMachinePoolReconciler{
			Client: c,
			newResourceReconciler: func(azureManagedMachinePool *infrav1.AzureManagedMachinePool, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					owner: azureManagedMachinePool,
					reconcileFunc: func(_ context.Context, azureManagedMachinePool resourceStatusObject) error {
						azureManagedMachinePool.SetResourceStatuses([]infrav1.ResourceStatus{
							{Ready: false},
						})
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedMachinePool)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update the AzureManagedControlPlane", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedMachinePool), azureManagedMachinePool)))
			t.Run("status.ready", checkEqual(azureManagedMachinePool.Status.Ready, false))
		})
	})

	t.Run("successful reconcile", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "ns",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind: "AzureManagedControlPlane",
				},
			},
		}
		machinePool := &expv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mp",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
				Annotations: map[string]string{
					clusterv1.ReplicasManagedByAnnotation: "something",
				},
			},
			Spec: expv1.MachinePoolSpec{
				Replicas: ptr.To[int32](2),
			},
		}
		managedCluster := &asocontainerservicev1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mc",
				Namespace: cluster.Namespace,
			},
			Status: asocontainerservicev1.ManagedCluster_STATUS{
				NodeResourceGroup: ptr.To("MC_rg"),
			},
		}
		agentPool := &asocontainerservicev1.ManagedClustersAgentPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mcap",
				Namespace: cluster.Namespace,
			},
			Spec: asocontainerservicev1.ManagedClusters_AgentPool_Spec{
				AzureName: "azmcap",
				Owner: &genruntime.KnownResourceReference{
					Name: managedCluster.Name,
				},
			},
			Status: asocontainerservicev1.ManagedClusters_AgentPool_STATUS{
				Count: ptr.To(4),
			},
		}
		azureManagedMachinePool := &infrav1.AzureManagedMachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ammp",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: expv1.GroupVersion.Identifier(),
						Kind:       "MachinePool",
						Name:       machinePool.Name,
					},
				},
				Finalizers: []string{infrav1.AzureManagedMachinePoolFinalizer},
				Annotations: map[string]string{
					clusterctlv1.BlockMoveAnnotation: "true",
				},
			},
			Spec: infrav1.AzureManagedMachinePoolSpec{
				AzureManagedMachinePoolTemplateResourceSpec: infrav1.AzureManagedMachinePoolTemplateResourceSpec{
					Resources: []runtime.RawExtension{
						{
							Raw: apJSON(t, &asocontainerservicev1.ManagedClustersAgentPool{
								ObjectMeta: metav1.ObjectMeta{
									Name: agentPool.Name,
								},
								Spec: asocontainerservicev1.ManagedClusters_AgentPool_Spec{
									Owner: &genruntime.KnownResourceReference{
										Name: managedCluster.Name,
									},
								},
							}),
						},
					},
				},
			},
			Status: infrav1.AzureManagedMachinePoolStatus{
				Ready: false,
			},
		}
		c := fakeClientBuilder().
			WithObjects(azureManagedMachinePool, cluster, machinePool, managedCluster, agentPool).
			Build()
		r := &AzureManagedMachinePoolReconciler{
			Client: c,
			newResourceReconciler: func(_ *infrav1.AzureManagedMachinePool, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, _ resourceStatusObject) error {
						return nil
					},
				}
			},
			Tracker: &FakeClusterTracker{
				getClientFunc: func(ctx context.Context, nn types.NamespacedName) (client.Client, error) {
					nodes := []client.Object{
						&corev1.Node{
							ObjectMeta: metav1.ObjectMeta{
								Name:   "node0",
								Labels: expectedNodeLabels(agentPool.AzureName(), *managedCluster.Status.NodeResourceGroup),
							},
							Spec: corev1.NodeSpec{
								ProviderID: "azure://node0",
							},
						},
						&corev1.Node{
							ObjectMeta: metav1.ObjectMeta{
								Name:   "node1",
								Labels: nil,
							},
						},
						&corev1.Node{
							ObjectMeta: metav1.ObjectMeta{
								Name:   "node2",
								Labels: expectedNodeLabels(agentPool.AzureName(), *managedCluster.Status.NodeResourceGroup),
							},
							Spec: corev1.NodeSpec{
								ProviderID: "azure://node2",
							},
						},
					}
					return fakeclient.NewClientBuilder().
						WithObjects(nodes...).
						Build(), nil
				},
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedMachinePool)})
		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update the AzureManagedControlPlane", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedMachinePool), azureManagedMachinePool)))
			t.Run("spec.providerIDList", func(t *testing.T) {
				t.Run("length", checkEqual(len(azureManagedMachinePool.Spec.ProviderIDList), 2))
				t.Run("0", checkEqual(azureManagedMachinePool.Spec.ProviderIDList[0], "azure://node0"))
				t.Run("1", checkEqual(azureManagedMachinePool.Spec.ProviderIDList[1], "azure://node2"))
			})
			t.Run("status.replicas", checkEqual(azureManagedMachinePool.Status.Replicas, 4))
			t.Run("status.ready", checkEqual(azureManagedMachinePool.Status.Ready, true))
		})
		t.Run("should update the MachinePool", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(machinePool), machinePool)))
			t.Run("replicas annotation", checkEqual(machinePool.Annotations[clusterv1.ReplicasManagedByAnnotation], ""))
			t.Run("spec.replicas", checkEqual(*machinePool.Spec.Replicas, 2))
		})
	})

	t.Run("successful reconcile with autoscaling", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "ns",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind: "AzureManagedControlPlane",
				},
			},
		}
		machinePool := &expv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mp",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
			},
		}
		managedCluster := &asocontainerservicev1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mc",
				Namespace: cluster.Namespace,
			},
			Status: asocontainerservicev1.ManagedCluster_STATUS{
				NodeResourceGroup: ptr.To("MC_rg"),
			},
		}
		agentPool := &asocontainerservicev1.ManagedClustersAgentPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mcap",
				Namespace: cluster.Namespace,
			},
			Spec: asocontainerservicev1.ManagedClusters_AgentPool_Spec{
				AzureName: "azmcap",
				Owner: &genruntime.KnownResourceReference{
					Name: managedCluster.Name,
				},
			},
			Status: asocontainerservicev1.ManagedClusters_AgentPool_STATUS{
				Count: ptr.To(4),
			},
		}
		azureManagedMachinePool := &infrav1.AzureManagedMachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ammp",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: expv1.GroupVersion.Identifier(),
						Kind:       "MachinePool",
						Name:       machinePool.Name,
					},
				},
				Finalizers: []string{infrav1.AzureManagedMachinePoolFinalizer},
				Annotations: map[string]string{
					clusterctlv1.BlockMoveAnnotation: "true",
				},
			},
			Spec: infrav1.AzureManagedMachinePoolSpec{
				AzureManagedMachinePoolTemplateResourceSpec: infrav1.AzureManagedMachinePoolTemplateResourceSpec{
					Resources: []runtime.RawExtension{
						{
							Raw: apJSON(t, &asocontainerservicev1.ManagedClustersAgentPool{
								ObjectMeta: metav1.ObjectMeta{
									Name: agentPool.Name,
								},
								Spec: asocontainerservicev1.ManagedClusters_AgentPool_Spec{
									Owner: &genruntime.KnownResourceReference{
										Name: managedCluster.Name,
									},
									EnableAutoScaling: ptr.To(true),
								},
							}),
						},
					},
				},
			},
			Status: infrav1.AzureManagedMachinePoolStatus{
				Ready: false,
			},
		}
		c := fakeClientBuilder().
			WithObjects(azureManagedMachinePool, cluster, machinePool, managedCluster, agentPool).
			Build()
		r := &AzureManagedMachinePoolReconciler{
			Client: c,
			newResourceReconciler: func(_ *infrav1.AzureManagedMachinePool, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, _ resourceStatusObject) error {
						return nil
					},
				}
			},
			Tracker: &FakeClusterTracker{
				getClientFunc: func(ctx context.Context, nn types.NamespacedName) (client.Client, error) {
					nodes := []client.Object{
						&corev1.Node{
							ObjectMeta: metav1.ObjectMeta{
								Name:   "node0",
								Labels: expectedNodeLabels(agentPool.AzureName(), *managedCluster.Status.NodeResourceGroup),
							},
							Spec: corev1.NodeSpec{
								ProviderID: "azure://node0",
							},
						},
						&corev1.Node{
							ObjectMeta: metav1.ObjectMeta{
								Name:   "node1",
								Labels: nil,
							},
						},
						&corev1.Node{
							ObjectMeta: metav1.ObjectMeta{
								Name:   "node2",
								Labels: expectedNodeLabels(agentPool.AzureName(), *managedCluster.Status.NodeResourceGroup),
							},
							Spec: corev1.NodeSpec{
								ProviderID: "azure://node2",
							},
						},
					}
					return fakeclient.NewClientBuilder().
						WithObjects(nodes...).
						Build(), nil
				},
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedMachinePool)})
		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update the AzureManagedControlPlane", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedMachinePool), azureManagedMachinePool)))
			t.Run("spec.providerIDList", func(t *testing.T) {
				t.Run("length", checkEqual(len(azureManagedMachinePool.Spec.ProviderIDList), 2))
				t.Run("0", checkEqual(azureManagedMachinePool.Spec.ProviderIDList[0], "azure://node0"))
				t.Run("1", checkEqual(azureManagedMachinePool.Spec.ProviderIDList[1], "azure://node2"))
			})
			t.Run("status.replicas", checkEqual(azureManagedMachinePool.Status.Replicas, 4))
			t.Run("status.ready", checkEqual(azureManagedMachinePool.Status.Ready, true))
		})
		t.Run("should update the MachinePool", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(machinePool), machinePool)))
			t.Run("replicas annotation", checkEqual(machinePool.Annotations[clusterv1.ReplicasManagedByAnnotation], "aks"))
			t.Run("spec.replicas", checkEqual(*machinePool.Spec.Replicas, 4))
		})
	})

	t.Run("Cluster is paused", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "ns",
			},
			Spec: clusterv1.ClusterSpec{
				Paused: true,
				ControlPlaneRef: &corev1.ObjectReference{
					Kind: "AzureManagedControlPlane",
				},
			},
		}
		machinePool := &expv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mp",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
				Annotations: map[string]string{
					clusterv1.ReplicasManagedByAnnotation: "something",
				},
			},
			Spec: expv1.MachinePoolSpec{
				Replicas: ptr.To[int32](2),
			},
		}
		managedCluster := &asocontainerservicev1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mc",
				Namespace: cluster.Namespace,
			},
			Status: asocontainerservicev1.ManagedCluster_STATUS{
				NodeResourceGroup: ptr.To("MC_rg"),
			},
		}
		agentPool := &asocontainerservicev1.ManagedClustersAgentPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mcap",
				Namespace: cluster.Namespace,
			},
			Spec: asocontainerservicev1.ManagedClusters_AgentPool_Spec{
				AzureName: "azmcap",
				Owner: &genruntime.KnownResourceReference{
					Name: managedCluster.Name,
				},
			},
			Status: asocontainerservicev1.ManagedClusters_AgentPool_STATUS{
				Count: ptr.To(4),
			},
		}
		azureManagedMachinePool := &infrav1.AzureManagedMachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ammp",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: expv1.GroupVersion.Identifier(),
						Kind:       "MachinePool",
						Name:       machinePool.Name,
					},
				},
				Finalizers: []string{infrav1.AzureManagedMachinePoolFinalizer},
				Annotations: map[string]string{
					clusterctlv1.BlockMoveAnnotation: "true",
				},
			},
			Spec: infrav1.AzureManagedMachinePoolSpec{
				AzureManagedMachinePoolTemplateResourceSpec: infrav1.AzureManagedMachinePoolTemplateResourceSpec{
					Resources: []runtime.RawExtension{
						{
							Raw: apJSON(t, &asocontainerservicev1.ManagedClustersAgentPool{
								ObjectMeta: metav1.ObjectMeta{
									Name: agentPool.Name,
								},
								Spec: asocontainerservicev1.ManagedClusters_AgentPool_Spec{
									Owner: &genruntime.KnownResourceReference{
										Name: managedCluster.Name,
									},
								},
							}),
						},
					},
				},
			},
			Status: infrav1.AzureManagedMachinePoolStatus{
				Ready: false,
			},
		}
		c := fakeClientBuilder().
			WithObjects(azureManagedMachinePool, cluster, machinePool, managedCluster, agentPool).
			Build()
		r := &AzureManagedMachinePoolReconciler{
			Client: c,
			newResourceReconciler: func(_ *infrav1.AzureManagedMachinePool, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, _ resourceStatusObject) error {
						return nil
					},
				}
			},
			Tracker: &FakeClusterTracker{
				getClientFunc: func(ctx context.Context, nn types.NamespacedName) (client.Client, error) {
					return fakeclient.NewClientBuilder().
						WithObjects().
						Build(), nil
				},
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedMachinePool)})
		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update the AzureManagedControlPlane", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedMachinePool), azureManagedMachinePool)))
			_, hasBlockMove := azureManagedMachinePool.Annotations[clusterctlv1.BlockMoveAnnotation]
			t.Run("block-move annotation is removed", checkEqual(hasBlockMove, false))
		})
	})

	t.Run("AzureManagedMachinePool delete succeeds", func(t *testing.T) {
		namespace := "ns"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind: "AzureManagedControlPlane",
				},
			},
		}
		machinePool := &expv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mp",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
			},
		}
		azureManagedMachinePool := &infrav1.AzureManagedMachinePool{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "ammp",
				Namespace:         cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: expv1.GroupVersion.Identifier(),
						Kind:       "MachinePool",
						Name:       machinePool.Name,
					},
				},
				Finalizers: []string{infrav1.AzureManagedMachinePoolFinalizer},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, machinePool, azureManagedMachinePool).
			Build()

		r := &AzureManagedMachinePoolReconciler{
			Client: c,
			newResourceReconciler: func(_ *infrav1.AzureManagedMachinePool, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					deleteFunc: func(_ context.Context, _ resourceStatusObject) error {
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedMachinePool)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should delete the AzureManagedMachinePool", checkEqual(apierrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(azureManagedMachinePool), azureManagedMachinePool)), true))
	})

	t.Run("Cluster delete succeeds", func(t *testing.T) {
		namespace := "ns"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Finalizers:        []string{clusterv1.ClusterFinalizer},
				Namespace:         namespace,
				Name:              "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind: "AzureManagedControlPlane",
				},
			},
		}
		machinePool := &expv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mp",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
			},
		}
		azureManagedMachinePool := &infrav1.AzureManagedMachinePool{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "ammp",
				Namespace:         cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: expv1.GroupVersion.Identifier(),
						Kind:       "MachinePool",
						Name:       machinePool.Name,
					},
				},
				Finalizers: []string{infrav1.AzureManagedMachinePoolFinalizer},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, machinePool, azureManagedMachinePool).
			Build()

		r := &AzureManagedMachinePoolReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedMachinePool)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should delete the AzureManagedMachinePool", checkEqual(apierrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(azureManagedMachinePool), azureManagedMachinePool)), true))
	})
}

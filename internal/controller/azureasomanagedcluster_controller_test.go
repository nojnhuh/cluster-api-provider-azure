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
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
)

type fakeResourceReconciler struct {
	owner         resourceStatusObject
	reconcileFunc func(context.Context, resourceStatusObject) error
	deleteFunc    func(context.Context, resourceStatusObject) error
}

func (r *fakeResourceReconciler) Reconcile(ctx context.Context) error {
	if r.reconcileFunc == nil {
		return nil
	}
	return r.reconcileFunc(ctx, r.owner)
}

func (r *fakeResourceReconciler) Delete(ctx context.Context) error {
	if r.deleteFunc == nil {
		return nil
	}
	return r.deleteFunc(ctx, r.owner)
}

func TestAzureASOManagedClusterReconcile(t *testing.T) {
	ctx := context.Background()

	s := runtime.NewScheme()
	sb := runtime.NewSchemeBuilder(
		infrav1.AddToScheme,
		clusterv1.AddToScheme,
	)
	t.Run("build scheme", expectSuccess(sb.AddToScheme(s)))
	fakeClientBuilder := func() *fakeclient.ClientBuilder {
		return fakeclient.NewClientBuilder().
			WithScheme(s).
			WithStatusSubresource(&infrav1.AzureASOManagedCluster{})
	}

	t.Run("AzureASOManagedCluster does not exist", func(t *testing.T) {
		c := fakeClientBuilder().
			Build()
		r := &AzureASOManagedClusterReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "doesn't", Name: "exist"}})
		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
	})

	t.Run("no Cluster ownerref", func(t *testing.T) {
		asoManagedCluster := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "amc",
				Namespace:  "ns",
				Generation: 1,
			},
		}
		c := fakeClientBuilder().
			WithObjects(asoManagedCluster).
			Build()

		r := &AzureASOManagedClusterReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update status.observedGeneration", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			t.Run("metadata.generation", checkEqual(asoManagedCluster.Generation, 1))
			t.Run("status.observedGeneration", checkEqual(asoManagedCluster.Status.ObservedGeneration, 1))
		})
	})

	t.Run("Cluster does not exist", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
		}
		asoManagedCluster := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amc",
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
			WithObjects(asoManagedCluster). // no cluster
			Build()

		r := &AzureASOManagedClusterReconciler{
			Client: c,
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should fail", checkEqual(apierrors.IsNotFound(err), true))
		t.Run("should not update status.observedGeneration", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			t.Run("metadata.generation", checkEqual(asoManagedCluster.Generation, 1))
			t.Run("status.observedGeneration", checkEqual(asoManagedCluster.Status.ObservedGeneration, 0))
		})
	})

	t.Run("Cluster does not also use AzureASOManagedControlPlane", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind: "not-AzureASOManagedControlPlane",
				},
			},
		}
		asoManagedCluster := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amc",
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
			WithObjects(cluster, asoManagedCluster).
			Build()

		r := &AzureASOManagedClusterReconciler{
			Client: c,
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should fail", checkEqual(errors.Is(err, invalidControlPlaneKindErr), true))
	})

	t.Run("Finalizer and block-move annotation are added", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind: "AzureASOManagedControlPlane",
				},
			},
		}
		asoManagedCluster := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amc",
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
			WithObjects(cluster, asoManagedCluster).
			Build()

		r := &AzureASOManagedClusterReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should requeue", checkEqual(result, ctrl.Result{Requeue: true}))
		t.Run("should add the finalizer", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			t.Run("metadata.finalizers[0]", checkEqual(asoManagedCluster.Finalizers[0], clusterv1.ClusterFinalizer))
			_, hasBlockMove := asoManagedCluster.Annotations[clusterctlv1.BlockMoveAnnotation]
			t.Run("block-move annotation is added", checkEqual(hasBlockMove, true))
		})
	})

	t.Run("resources get reconciled and are not ready", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind: "AzureASOManagedControlPlane",
				},
			},
		}
		asoManagedCluster := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amc",
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
			Status: infrav1.AzureASOManagedClusterStatus{
				Ready: true,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedCluster).
			Build()

		r := &AzureASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.AzureASOManagedCluster, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					owner: asoManagedCluster,
					reconcileFunc: func(_ context.Context, asoManagedCluster resourceStatusObject) error {
						asoManagedCluster.SetResourceStatuses([]infrav1.ResourceStatus{
							{Ready: true},
							{Ready: false},
							{Ready: true},
						})
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update the AzureASOManagedCluster", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			t.Run("should not be ready", checkEqual(asoManagedCluster.Status.Ready, false))
			t.Run("status.conditions sets ResourcesReady false", checkEqual(conditions.Get(asoManagedCluster, infrav1.ResourcesReady).Status, corev1.ConditionFalse))
		})
	})

	t.Run("resources fail to reconcile", func(t *testing.T) {
		namespace := "ns"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind:      "AzureASOManagedControlPlane",
					Namespace: namespace,
					Name:      "amcp",
				},
			},
		}
		asoManagedCluster := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amc",
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
			Status: infrav1.AzureASOManagedClusterStatus{
				Ready: true,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedCluster).
			Build()

		resourceReconcileErr := errors.New("resource reconcile error")
		r := &AzureASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.AzureASOManagedCluster, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, _ resourceStatusObject) error {
						return resourceReconcileErr
					},
				}
			},
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should fail", checkEqual(errors.Is(err, resourceReconcileErr), true))
		t.Run("should update the AzureASOManagedCluster", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			t.Run("status.ready", checkEqual(asoManagedCluster.Status.Ready, false))
		})
	})

	t.Run("AzureASOManagedControlPlane not found", func(t *testing.T) {
		namespace := "ns"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind:      "AzureASOManagedControlPlane",
					Namespace: namespace,
					Name:      "amcp",
				},
			},
		}
		asoManagedCluster := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amc",
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
			Status: infrav1.AzureASOManagedClusterStatus{
				Ready: true,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedCluster).
			Build()

		r := &AzureASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.AzureASOManagedCluster, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, _ resourceStatusObject) error {
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update the AzureASOManagedCluster", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			t.Run("status.ready", checkEqual(asoManagedCluster.Status.Ready, false))
			t.Run("status.conditions sets ControlPlaneEndpointReady false", checkEqual(conditions.Get(asoManagedCluster, infrav1.ControlPlaneEndpointReady).Status, corev1.ConditionFalse))
			t.Run("spec.controlPlaneEndpoint", checkEqual(asoManagedCluster.Spec.ControlPlaneEndpoint.IsZero(), true))
		})
	})

	t.Run("resources get reconciled and are ready", func(t *testing.T) {
		namespace := "ns"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind:      "AzureASOManagedControlPlane",
					Namespace: namespace,
					Name:      "amcp",
				},
			},
		}
		asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cluster.Spec.ControlPlaneRef.Namespace,
				Name:      cluster.Spec.ControlPlaneRef.Name,
			},
			Status: infrav1.AzureASOManagedControlPlaneStatus{
				ControlPlaneEndpoint: clusterv1.APIEndpoint{
					Host: "bingo",
				},
			},
		}
		asoManagedCluster := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amc",
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
			Status: infrav1.AzureASOManagedClusterStatus{
				Ready: false,
				Conditions: clusterv1.Conditions{
					{
						Type:   clusterv1.ReadyCondition,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   infrav1.ResourcesReady,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   infrav1.ControlPlaneEndpointReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedCluster, asoManagedControlPlane).
			Build()

		r := &AzureASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.AzureASOManagedCluster, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, _ resourceStatusObject) error {
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update the AzureASOManagedCluster", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			t.Run("status.ready", checkEqual(asoManagedCluster.Status.Ready, true))
			t.Run("status.conditions sets Ready true", checkEqual(conditions.Get(asoManagedCluster, clusterv1.ReadyCondition).Status, corev1.ConditionTrue))
			t.Run("status.conditions sets ResourcesReady true", checkEqual(conditions.Get(asoManagedCluster, infrav1.ResourcesReady).Status, corev1.ConditionTrue))
			t.Run("status.conditions sets ControlPlaneEndpointReady true", checkEqual(conditions.Get(asoManagedCluster, infrav1.ControlPlaneEndpointReady).Status, corev1.ConditionTrue))
			t.Run("spec.controlPlaneEndpoint", checkEqual(asoManagedCluster.Spec.ControlPlaneEndpoint, clusterv1.APIEndpoint{Host: "bingo"}))
		})
	})

	t.Run("Cluster is paused", func(t *testing.T) {
		namespace := "ns"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				Paused: true,
				ControlPlaneRef: &corev1.ObjectReference{
					Kind:      "AzureASOManagedControlPlane",
					Namespace: namespace,
					Name:      "amcp",
				},
			},
		}
		asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cluster.Spec.ControlPlaneRef.Namespace,
				Name:      cluster.Spec.ControlPlaneRef.Name,
			},
			Status: infrav1.AzureASOManagedControlPlaneStatus{
				ControlPlaneEndpoint: clusterv1.APIEndpoint{
					Host: "bingo",
				},
			},
		}
		asoManagedCluster := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "amc",
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
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedCluster, asoManagedControlPlane).
			Build()

		r := &AzureASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.AzureASOManagedCluster, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, _ resourceStatusObject) error {
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update the AzureASOManagedCluster", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			_, hasBlockMove := asoManagedCluster.Annotations[clusterctlv1.BlockMoveAnnotation]
			t.Run("block-move annotation is removed", checkEqual(hasBlockMove, false))
		})
	})

	t.Run("Delete without Cluster ownerref", func(t *testing.T) {
		asoManagedCluster := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "amc",
				Namespace:         "ns",
				OwnerReferences:   []metav1.OwnerReference{},
				Finalizers:        []string{clusterv1.ClusterFinalizer},
			},
		}
		c := fakeClientBuilder().
			WithObjects(asoManagedCluster).
			Build()

		r := &AzureASOManagedClusterReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should delete the AzureASOManagedCluster", checkEqual(apierrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)), true))
	})

	t.Run("Delete error", func(t *testing.T) {
		namespace := "ns"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "cluster",
			},
		}
		asoManagedCluster := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "amc",
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
			WithObjects(cluster, asoManagedCluster).
			Build()

		deleteErr := errors.New("delete error")
		r := &AzureASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.AzureASOManagedCluster, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					deleteFunc: func(_ context.Context, _ resourceStatusObject) error {
						return deleteErr
					},
				}
			},
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should fail", checkEqual(errors.Is(err, deleteErr), true))
		t.Run("should not remove the finalizer", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			t.Run("finalizers length", checkEqual(len(asoManagedCluster.Finalizers), 1))
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
		asoManagedCluster := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "amc",
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
			WithObjects(cluster, asoManagedCluster).
			Build()

		r := &AzureASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.AzureASOManagedCluster, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					owner: asoManagedCluster,
					deleteFunc: func(_ context.Context, asoManagedCluster resourceStatusObject) error {
						asoManagedCluster.SetResourceStatuses([]infrav1.ResourceStatus{
							{Ready: false},
						})
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should not remove the finalizer", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			t.Run("finalizers length", checkEqual(len(asoManagedCluster.Finalizers), 1))
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
		asoManagedCluster := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "amc",
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
			WithObjects(cluster, asoManagedCluster).
			Build()

		r := &AzureASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.AzureASOManagedCluster, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					deleteFunc: func(_ context.Context, _ resourceStatusObject) error {
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should delete the AzureASOManagedCluster", checkEqual(apierrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)), true))
	})
}

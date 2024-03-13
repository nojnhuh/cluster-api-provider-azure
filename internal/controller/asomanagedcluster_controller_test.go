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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "sigs.k8s.io/cluster-api-provider-azure/v2/api/v2alpha1"
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

func TestASOManagedClusterReconcile(t *testing.T) {
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
			WithStatusSubresource(&infrav1.ASOManagedCluster{})
	}

	t.Run("ASOManagedCluster does not exist", func(t *testing.T) {
		c := fakeClientBuilder().
			Build()
		r := &ASOManagedClusterReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "doesn't", Name: "exist"}})
		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
	})

	t.Run("no Cluster ownerref", func(t *testing.T) {
		asoManagedCluster := &infrav1.ASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "asomc",
				Namespace:  "ns",
				Generation: 1,
			},
		}
		c := fakeClientBuilder().
			WithObjects(asoManagedCluster).
			Build()

		r := &ASOManagedClusterReconciler{
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
		asoManagedCluster := &infrav1.ASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "asomc",
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

		r := &ASOManagedClusterReconciler{
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

	t.Run("Cluster does not also use ASOManagedControlPlane", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind: "not-ASOManagedControlPlane",
				},
			},
		}
		asoManagedCluster := &infrav1.ASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "asomc",
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

		r := &ASOManagedClusterReconciler{
			Client: c,
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should fail", checkEqual(errors.Is(err, invalidControlPlaneKindErr), true))
	})

	t.Run("Finalizer is added", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind: "ASOManagedControlPlane",
				},
			},
		}
		asoManagedCluster := &infrav1.ASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "asomc",
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

		r := &ASOManagedClusterReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should requeue", checkEqual(result, ctrl.Result{Requeue: true}))
		t.Run("should add the finalizer", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			t.Run("metadata.finalizers[0]", checkEqual(asoManagedCluster.Finalizers[0], clusterv1.ClusterFinalizer))
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
					Kind: "ASOManagedControlPlane",
				},
			},
		}
		asoManagedCluster := &infrav1.ASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "asomc",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
					},
				},
				Finalizers: []string{clusterv1.ClusterFinalizer},
			},
			Status: infrav1.ASOManagedClusterStatus{
				Ready: true,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedCluster).
			Build()

		r := &ASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.ASOManagedCluster) resourceReconciler {
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
		t.Run("should update the ASOManagedCluster", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			t.Run("should not be ready", checkEqual(asoManagedCluster.Status.Ready, false))
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
					Kind:      "ASOManagedControlPlane",
					Namespace: namespace,
					Name:      "asomcp",
				},
			},
		}
		asoManagedCluster := &infrav1.ASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "asomc",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
					},
				},
				Finalizers: []string{clusterv1.ClusterFinalizer},
			},
			Status: infrav1.ASOManagedClusterStatus{
				Ready: true,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedCluster).
			Build()

		resourceReconcileErr := errors.New("resource reconcile error")
		r := &ASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.ASOManagedCluster) resourceReconciler {
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, _ resourceStatusObject) error {
						return resourceReconcileErr
					},
				}
			},
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should fail", checkEqual(errors.Is(err, resourceReconcileErr), true))
		t.Run("should update the ASOManagedCluster", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			t.Run("status.ready", checkEqual(asoManagedCluster.Status.Ready, false))
		})
	})

	t.Run("ASOManagedControlPlane not found", func(t *testing.T) {
		namespace := "ns"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind:      "ASOManagedControlPlane",
					Namespace: namespace,
					Name:      "asomcp",
				},
			},
		}
		asoManagedCluster := &infrav1.ASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "asomc",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
					},
				},
				Finalizers: []string{clusterv1.ClusterFinalizer},
			},
			Status: infrav1.ASOManagedClusterStatus{
				Ready: true,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedCluster).
			Build()

		r := &ASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.ASOManagedCluster) resourceReconciler {
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
		t.Run("should update the ASOManagedCluster", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			t.Run("status.ready", checkEqual(asoManagedCluster.Status.Ready, false))
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
					Kind:      "ASOManagedControlPlane",
					Namespace: namespace,
					Name:      "asomcp",
				},
			},
		}
		asoManagedControlPlane := &infrav1.ASOManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cluster.Spec.ControlPlaneRef.Namespace,
				Name:      cluster.Spec.ControlPlaneRef.Name,
			},
			Status: infrav1.ASOManagedControlPlaneStatus{
				ControlPlaneEndpoint: clusterv1.APIEndpoint{
					Host: "bingo",
				},
			},
		}
		asoManagedCluster := &infrav1.ASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "asomc",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
					},
				},
				Finalizers: []string{clusterv1.ClusterFinalizer},
			},
			Status: infrav1.ASOManagedClusterStatus{
				Ready: false,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoManagedCluster, asoManagedControlPlane).
			Build()

		r := &ASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.ASOManagedCluster) resourceReconciler {
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
		t.Run("should update the ASOManagedCluster", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)))
			t.Run("status.ready", checkEqual(asoManagedCluster.Status.Ready, true))
			t.Run("spec.controlPlaneEndpoint", checkEqual(asoManagedCluster.Spec.ControlPlaneEndpoint, clusterv1.APIEndpoint{Host: "bingo"}))
		})
	})

	t.Run("Delete without Cluster ownerref", func(t *testing.T) {
		asoManagedCluster := &infrav1.ASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "asomc",
				Namespace:         "ns",
				OwnerReferences:   []metav1.OwnerReference{},
				Finalizers:        []string{clusterv1.ClusterFinalizer},
			},
		}
		c := fakeClientBuilder().
			WithObjects(asoManagedCluster).
			Build()

		r := &ASOManagedClusterReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should delete the ASOManagedCluster", checkEqual(apierrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)), true))
	})

	t.Run("Delete error", func(t *testing.T) {
		namespace := "ns"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "cluster",
			},
		}
		asoManagedCluster := &infrav1.ASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "asomc",
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
		r := &ASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.ASOManagedCluster) resourceReconciler {
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
		asoManagedCluster := &infrav1.ASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "asomc",
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

		r := &ASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.ASOManagedCluster) resourceReconciler {
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
		asoManagedCluster := &infrav1.ASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "asomc",
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

		r := &ASOManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoManagedCluster *infrav1.ASOManagedCluster) resourceReconciler {
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
		t.Run("should delete the ASOManagedCluster", checkEqual(apierrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(asoManagedCluster), asoManagedCluster)), true))
	})
}

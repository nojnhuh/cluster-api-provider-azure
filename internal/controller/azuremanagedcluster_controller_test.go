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

func TestAzureManagedClusterReconcile(t *testing.T) {
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
			WithStatusSubresource(&infrav1.AzureManagedCluster{})
	}

	t.Run("AzureManagedCluster does not exist", func(t *testing.T) {
		c := fakeClientBuilder().
			Build()
		r := &AzureManagedClusterReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "doesn't", Name: "exist"}})
		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
	})

	t.Run("no Cluster ownerref", func(t *testing.T) {
		azureManagedCluster := &infrav1.AzureManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "amc",
				Namespace:  "ns",
				Generation: 1,
			},
		}
		c := fakeClientBuilder().
			WithObjects(azureManagedCluster).
			Build()

		r := &AzureManagedClusterReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update status.observedGeneration", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedCluster), azureManagedCluster)))
			t.Run("metadata.generation", checkEqual(azureManagedCluster.Generation, 1))
			t.Run("status.observedGeneration", checkEqual(azureManagedCluster.Status.ObservedGeneration, 1))
		})
	})

	t.Run("Cluster does not exist", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
		}
		azureManagedCluster := &infrav1.AzureManagedCluster{
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
			WithObjects(azureManagedCluster). // no cluster
			Build()

		r := &AzureManagedClusterReconciler{
			Client: c,
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedCluster)})

		t.Run("should fail", checkEqual(apierrors.IsNotFound(err), true))
		t.Run("should not update status.observedGeneration", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedCluster), azureManagedCluster)))
			t.Run("metadata.generation", checkEqual(azureManagedCluster.Generation, 1))
			t.Run("status.observedGeneration", checkEqual(azureManagedCluster.Status.ObservedGeneration, 0))
		})
	})

	t.Run("Cluster does not also use AzureManagedControlPlane", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind: "not-AzureManagedControlPlane",
				},
			},
		}
		azureManagedCluster := &infrav1.AzureManagedCluster{
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
			WithObjects(cluster, azureManagedCluster).
			Build()

		r := &AzureManagedClusterReconciler{
			Client: c,
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedCluster)})

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
					Kind: "AzureManagedControlPlane",
				},
			},
		}
		azureManagedCluster := &infrav1.AzureManagedCluster{
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
			WithObjects(cluster, azureManagedCluster).
			Build()

		r := &AzureManagedClusterReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should requeue", checkEqual(result, ctrl.Result{Requeue: true}))
		t.Run("should add the finalizer", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedCluster), azureManagedCluster)))
			t.Run("metadata.finalizers[0]", checkEqual(azureManagedCluster.Finalizers[0], clusterv1.ClusterFinalizer))
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
					Kind: "AzureManagedControlPlane",
				},
			},
		}
		azureManagedCluster := &infrav1.AzureManagedCluster{
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
			},
			Status: infrav1.AzureManagedClusterStatus{
				Ready: true,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, azureManagedCluster).
			Build()

		r := &AzureManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(azureManagedCluster *infrav1.AzureManagedCluster) resourceReconciler {
				return &fakeResourceReconciler{
					owner: azureManagedCluster,
					reconcileFunc: func(_ context.Context, azureManagedCluster resourceStatusObject) error {
						azureManagedCluster.SetResourceStatuses([]infrav1.ResourceStatus{
							{Ready: true},
							{Ready: false},
							{Ready: true},
						})
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update the AzureManagedCluster", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedCluster), azureManagedCluster)))
			t.Run("should not be ready", checkEqual(azureManagedCluster.Status.Ready, false))
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
					Kind:      "AzureManagedControlPlane",
					Namespace: namespace,
					Name:      "amcp",
				},
			},
		}
		azureManagedCluster := &infrav1.AzureManagedCluster{
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
			},
			Status: infrav1.AzureManagedClusterStatus{
				Ready: true,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, azureManagedCluster).
			Build()

		resourceReconcileErr := errors.New("resource reconcile error")
		r := &AzureManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(azureManagedCluster *infrav1.AzureManagedCluster) resourceReconciler {
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, _ resourceStatusObject) error {
						return resourceReconcileErr
					},
				}
			},
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedCluster)})

		t.Run("should fail", checkEqual(errors.Is(err, resourceReconcileErr), true))
		t.Run("should update the AzureManagedCluster", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedCluster), azureManagedCluster)))
			t.Run("status.ready", checkEqual(azureManagedCluster.Status.Ready, false))
		})
	})

	t.Run("AzureManagedControlPlane not found", func(t *testing.T) {
		namespace := "ns"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "cluster",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					Kind:      "AzureManagedControlPlane",
					Namespace: namespace,
					Name:      "amcp",
				},
			},
		}
		azureManagedCluster := &infrav1.AzureManagedCluster{
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
			},
			Status: infrav1.AzureManagedClusterStatus{
				Ready: true,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, azureManagedCluster).
			Build()

		r := &AzureManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(azureManagedCluster *infrav1.AzureManagedCluster) resourceReconciler {
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, _ resourceStatusObject) error {
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update the AzureManagedCluster", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedCluster), azureManagedCluster)))
			t.Run("status.ready", checkEqual(azureManagedCluster.Status.Ready, false))
			t.Run("spec.controlPlaneEndpoint", checkEqual(azureManagedCluster.Spec.ControlPlaneEndpoint.IsZero(), true))
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
					Kind:      "AzureManagedControlPlane",
					Namespace: namespace,
					Name:      "amcp",
				},
			},
		}
		azureManagedControlPlane := &infrav1.AzureManagedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cluster.Spec.ControlPlaneRef.Namespace,
				Name:      cluster.Spec.ControlPlaneRef.Name,
			},
			Status: infrav1.AzureManagedControlPlaneStatus{
				ControlPlaneEndpoint: clusterv1.APIEndpoint{
					Host: "bingo",
				},
			},
		}
		azureManagedCluster := &infrav1.AzureManagedCluster{
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
			},
			Status: infrav1.AzureManagedClusterStatus{
				Ready: false,
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, azureManagedCluster, azureManagedControlPlane).
			Build()

		r := &AzureManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(azureManagedCluster *infrav1.AzureManagedCluster) resourceReconciler {
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, _ resourceStatusObject) error {
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should update the AzureManagedCluster", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedCluster), azureManagedCluster)))
			t.Run("status.ready", checkEqual(azureManagedCluster.Status.Ready, true))
			t.Run("spec.controlPlaneEndpoint", checkEqual(azureManagedCluster.Spec.ControlPlaneEndpoint, clusterv1.APIEndpoint{Host: "bingo"}))
		})
	})

	t.Run("Delete without Cluster ownerref", func(t *testing.T) {
		azureManagedCluster := &infrav1.AzureManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
				Name:              "amc",
				Namespace:         "ns",
				OwnerReferences:   []metav1.OwnerReference{},
				Finalizers:        []string{clusterv1.ClusterFinalizer},
			},
		}
		c := fakeClientBuilder().
			WithObjects(azureManagedCluster).
			Build()

		r := &AzureManagedClusterReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should delete the AzureManagedCluster", checkEqual(apierrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(azureManagedCluster), azureManagedCluster)), true))
	})

	t.Run("Delete error", func(t *testing.T) {
		namespace := "ns"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "cluster",
			},
		}
		azureManagedCluster := &infrav1.AzureManagedCluster{
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
			WithObjects(cluster, azureManagedCluster).
			Build()

		deleteErr := errors.New("delete error")
		r := &AzureManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(azureManagedCluster *infrav1.AzureManagedCluster) resourceReconciler {
				return &fakeResourceReconciler{
					deleteFunc: func(_ context.Context, _ resourceStatusObject) error {
						return deleteErr
					},
				}
			},
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedCluster)})

		t.Run("should fail", checkEqual(errors.Is(err, deleteErr), true))
		t.Run("should not remove the finalizer", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedCluster), azureManagedCluster)))
			t.Run("finalizers length", checkEqual(len(azureManagedCluster.Finalizers), 1))
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
		azureManagedCluster := &infrav1.AzureManagedCluster{
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
			WithObjects(cluster, azureManagedCluster).
			Build()

		r := &AzureManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(azureManagedCluster *infrav1.AzureManagedCluster) resourceReconciler {
				return &fakeResourceReconciler{
					owner: azureManagedCluster,
					deleteFunc: func(_ context.Context, azureManagedCluster resourceStatusObject) error {
						azureManagedCluster.SetResourceStatuses([]infrav1.ResourceStatus{
							{Ready: false},
						})
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should not remove the finalizer", func(t *testing.T) {
			t.Run("GET", expectSuccess(c.Get(ctx, client.ObjectKeyFromObject(azureManagedCluster), azureManagedCluster)))
			t.Run("finalizers length", checkEqual(len(azureManagedCluster.Finalizers), 1))
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
		azureManagedCluster := &infrav1.AzureManagedCluster{
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
			WithObjects(cluster, azureManagedCluster).
			Build()

		r := &AzureManagedClusterReconciler{
			Client: c,
			newResourceReconciler: func(azureManagedCluster *infrav1.AzureManagedCluster) resourceReconciler {
				return &fakeResourceReconciler{
					deleteFunc: func(_ context.Context, _ resourceStatusObject) error {
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(azureManagedCluster)})

		t.Run("should succeed", expectSuccess(err))
		t.Run("should not requeue", checkEqual(result, ctrl.Result{}))
		t.Run("should delete the AzureManagedCluster", checkEqual(apierrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(azureManagedCluster), azureManagedCluster)), true))
	})
}

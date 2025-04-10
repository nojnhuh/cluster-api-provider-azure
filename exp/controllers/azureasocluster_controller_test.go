/*
Copyright 2025 The Kubernetes Authors.

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

package controllers

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	infrav1alphaexp "sigs.k8s.io/cluster-api-provider-azure/exp/api/v1alpha1"
)

type fakeResourceReconciler struct {
	owner         client.Object
	reconcileFunc func(context.Context, client.Object) error
	pauseFunc     func(context.Context, client.Object) error
	deleteFunc    func(context.Context, client.Object) error
}

func (r *fakeResourceReconciler) Reconcile(ctx context.Context) error {
	if r.reconcileFunc == nil {
		return nil
	}
	return r.reconcileFunc(ctx, r.owner)
}

func (r *fakeResourceReconciler) Pause(ctx context.Context) error {
	if r.pauseFunc == nil {
		return nil
	}
	return r.pauseFunc(ctx, r.owner)
}

func (r *fakeResourceReconciler) Delete(ctx context.Context) error {
	if r.deleteFunc == nil {
		return nil
	}
	return r.deleteFunc(ctx, r.owner)
}

func TestAzureASOClusterReconcile(t *testing.T) {
	ctx := context.Background()

	s := runtime.NewScheme()
	sb := runtime.NewSchemeBuilder(
		infrav1alphaexp.AddToScheme,
		clusterv1.AddToScheme,
	)
	NewGomegaWithT(t).Expect(sb.AddToScheme(s)).To(Succeed())

	fakeClientBuilder := func() *fakeclient.ClientBuilder {
		return fakeclient.NewClientBuilder().
			WithScheme(s).
			WithStatusSubresource(&infrav1alphaexp.AzureASOCluster{})
	}

	t.Run("AzureASOCluster does not exist", func(t *testing.T) {
		g := NewGomegaWithT(t)

		c := fakeClientBuilder().
			Build()
		r := &AzureASOClusterReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "doesn't", Name: "exist"}})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{}))
	})

	t.Run("Cluster does not exist", func(t *testing.T) {
		g := NewGomegaWithT(t)

		asoCluster := &infrav1alphaexp.AzureASOCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aso-cluster",
				Namespace: "ns",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       "Cluster",
						Name:       "cluster",
					},
				},
			},
		}
		c := fakeClientBuilder().
			WithObjects(asoCluster).
			Build()
		r := &AzureASOClusterReconciler{
			Client: c,
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoCluster)})
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("adds a finalizer and block-move annotation", func(t *testing.T) {
		g := NewGomegaWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "ns",
			},
		}
		asoCluster := &infrav1alphaexp.AzureASOCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aso-cluster",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       "Cluster",
						Name:       cluster.Name,
					},
				},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoCluster).
			Build()
		r := &AzureASOClusterReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoCluster)})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))

		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(asoCluster), asoCluster)).To(Succeed())
		g.Expect(asoCluster.GetFinalizers()).To(ContainElement(infrav1alphaexp.AzureASOClusterFinalizer))
		g.Expect(asoCluster.GetAnnotations()).To(HaveKey(clusterctlv1.BlockMoveAnnotation))
	})

	t.Run("reconciles resources that are not ready", func(t *testing.T) {
		g := NewGomegaWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "ns",
			},
		}
		asoCluster := &infrav1alphaexp.AzureASOCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aso-cluster",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       "Cluster",
						Name:       cluster.Name,
					},
				},
				Finalizers: []string{
					infrav1alphaexp.AzureASOClusterFinalizer,
				},
				Annotations: map[string]string{
					clusterctlv1.BlockMoveAnnotation: "true",
				},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoCluster).
			Build()

		var reconciled bool
		r := &AzureASOClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoCluster *infrav1alphaexp.AzureASOCluster, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					owner: asoCluster,
					reconcileFunc: func(ctx context.Context, o client.Object) error {
						asoCluster.SetResourceStatuses([]infrav1.ResourceStatus{
							{Ready: true},
							{Ready: false},
							{Ready: true},
						})
						reconciled = true
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoCluster)})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(reconciled).To(BeTrue())
	})

	t.Run("successfully reconciles normally", func(t *testing.T) {
		g := NewGomegaWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "ns",
			},
		}
		asoCluster := &infrav1alphaexp.AzureASOCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aso-cluster",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       "Cluster",
						Name:       cluster.Name,
					},
				},
				Finalizers: []string{
					infrav1alphaexp.AzureASOClusterFinalizer,
				},
				Annotations: map[string]string{
					clusterctlv1.BlockMoveAnnotation: "true",
				},
			},
			Spec: infrav1alphaexp.AzureASOClusterSpec{
				AzureASOClusterTemplateResourceSpec: infrav1alphaexp.AzureASOClusterTemplateResourceSpec{
					Patches: []infrav1alphaexp.ResourcesPatch{
						{
							Selectors: []infrav1alphaexp.ResourcesPatchSelector{
								{
									Kind:       "ResourceGroup",
									APIVersion: "resources.azure.com/v1api20200601",
								},
								{Kind: "AnotherKind"},
							},
							JSONPatches: []infrav1alphaexp.JSONPatch{
								{
									Op:   infrav1alphaexp.JSONPatchOpAdd,
									Path: "/metadata",
									Value: &apiextensionsv1.JSON{
										Raw: []byte(`{"name": "rg-name"}`),
									},
								},
							},
						},
					},
					Resources: []runtime.RawExtension{
						{Raw: []byte(`{
							"apiVersion": "resources.azure.com/v1api20200601",
							"kind": "ResourceGroup"
						}`)},
					},
				},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoCluster).
			Build()
		expectReconciled := map[string]struct{}{
			"rg-name": {},
		}
		r := &AzureASOClusterReconciler{
			Client: c,
			newResourceReconciler: func(_ *infrav1alphaexp.AzureASOCluster, us []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, _ client.Object) error {
						for _, u := range us {
							g.Expect(expectReconciled).To(HaveKey(u.GetName()), "reconciled unexpected resource")
							delete(expectReconciled, u.GetName())
						}
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoCluster)})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(expectReconciled).To(BeEmpty(), "resources should have been reconciled but were not")

		err = c.Get(ctx, client.ObjectKeyFromObject(asoCluster), asoCluster)
		g.Expect(err).NotTo(HaveOccurred())
	})

	t.Run("successfully reconciles pause", func(t *testing.T) {
		g := NewGomegaWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "ns",
			},
			Spec: clusterv1.ClusterSpec{
				Paused: true,
			},
		}
		asoCluster := &infrav1alphaexp.AzureASOCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aso-cluster",
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       "Cluster",
						Name:       cluster.Name,
					},
				},
				Annotations: map[string]string{
					clusterctlv1.BlockMoveAnnotation: "true",
				},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, asoCluster).
			Build()
		var reconciled bool
		r := &AzureASOClusterReconciler{
			Client: c,
			newResourceReconciler: func(_ *infrav1alphaexp.AzureASOCluster, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					pauseFunc: func(ctx context.Context, o client.Object) error {
						reconciled = true
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoCluster)})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(reconciled).To(BeTrue())
	})

	t.Run("successfully reconciles in-progress delete", func(t *testing.T) {
		g := NewGomegaWithT(t)

		asoCluster := &infrav1alphaexp.AzureASOCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aso-cluster",
				Namespace: "ns",
				Finalizers: []string{
					infrav1alphaexp.AzureASOClusterFinalizer,
				},
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
			},
		}
		c := fakeClientBuilder().
			WithObjects(asoCluster).
			Build()
		var reconciled bool
		r := &AzureASOClusterReconciler{
			Client: c,
			newResourceReconciler: func(asoCluster *infrav1alphaexp.AzureASOCluster, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					owner: asoCluster,
					deleteFunc: func(ctx context.Context, o client.Object) error {
						asoCluster.SetResourceStatuses([]infrav1.ResourceStatus{
							{
								Resource: infrav1.StatusResource{
									Name: "still-deleting",
								},
							},
						})
						reconciled = true
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoCluster)})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{}))

		err = c.Get(ctx, client.ObjectKeyFromObject(asoCluster), asoCluster)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(asoCluster.GetFinalizers()).To(ContainElement(infrav1alphaexp.AzureASOClusterFinalizer))
		g.Expect(reconciled).To(BeTrue())
	})

	t.Run("successfully reconciles finished delete", func(t *testing.T) {
		g := NewGomegaWithT(t)

		asoCluster := &infrav1alphaexp.AzureASOCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aso-cluster",
				Namespace: "ns",
				Finalizers: []string{
					infrav1alphaexp.AzureASOClusterFinalizer,
				},
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
			},
		}
		c := fakeClientBuilder().
			WithObjects(asoCluster).
			Build()
		var reconciled bool
		r := &AzureASOClusterReconciler{
			Client: c,
			newResourceReconciler: func(_ *infrav1alphaexp.AzureASOCluster, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					deleteFunc: func(ctx context.Context, o client.Object) error {
						reconciled = true
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoCluster)})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(reconciled).To(BeTrue())

		err = c.Get(ctx, client.ObjectKeyFromObject(asoCluster), asoCluster)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
}

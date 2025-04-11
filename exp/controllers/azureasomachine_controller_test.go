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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	infrav1alphaexp "sigs.k8s.io/cluster-api-provider-azure/exp/api/v1alpha1"
)

func TestAzureASOMachineReconcile(t *testing.T) {
	ctx := t.Context()

	s := runtime.NewScheme()
	sb := runtime.NewSchemeBuilder(
		infrav1alphaexp.AddToScheme,
		clusterv1.AddToScheme,
		corev1.AddToScheme,
	)
	NewGomegaWithT(t).Expect(sb.AddToScheme(s)).To(Succeed())

	fakeClientBuilder := func() *fakeclient.ClientBuilder {
		return fakeclient.NewClientBuilder().
			WithScheme(s).
			WithStatusSubresource(&infrav1alphaexp.AzureASOMachine{})
	}

	t.Run("AzureASOMachine does not exist", func(t *testing.T) {
		g := NewGomegaWithT(t)

		c := fakeClientBuilder().
			Build()
		r := &AzureASOMachineReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "doesn't", Name: "exist"}})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{}))
	})

	t.Run("Machine does not exist", func(t *testing.T) {
		g := NewGomegaWithT(t)

		asoMachine := &infrav1alphaexp.AzureASOMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aso-machine",
				Namespace: "ns",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       "Machine",
						Name:       "machine",
					},
				},
			},
		}
		c := fakeClientBuilder().
			WithObjects(asoMachine).
			Build()
		r := &AzureASOMachineReconciler{
			Client: c,
		}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoMachine)})
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
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine",
				Namespace: cluster.Namespace,
			},
		}
		asoMachine := &infrav1alphaexp.AzureASOMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aso-machine",
				Namespace: machine.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       "Machine",
						Name:       machine.Name,
					},
				},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, machine, asoMachine).
			Build()
		r := &AzureASOMachineReconciler{
			Client: c,
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoMachine)})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal((ctrl.Result{Requeue: true})))

		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(asoMachine), asoMachine)).To(Succeed())
		g.Expect(asoMachine.GetFinalizers()).To(ContainElement(infrav1alphaexp.AzureASOMachineFinalizer))
		g.Expect(asoMachine.GetAnnotations()).To(HaveKey(clusterctlv1.BlockMoveAnnotation))
	})

	t.Run("successfully reconciles resources that are not ready", func(t *testing.T) {
		g := NewGomegaWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "ns",
			},
			Status: clusterv1.ClusterStatus{
				Initialization: clusterv1.ClusterInitializationStatus{
					InfrastructureProvisioned: ptr.To(true),
				},
			},
		}
		bootstrapData := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bootstrap",
				Namespace: cluster.Namespace,
			},
		}
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine",
				Namespace: cluster.Namespace,
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					DataSecretName: &bootstrapData.Name,
				},
			},
		}
		asoMachine := &infrav1alphaexp.AzureASOMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aso-machine",
				Namespace: machine.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       "Machine",
						Name:       machine.Name,
					},
				},
				Finalizers: []string{
					infrav1alphaexp.AzureASOMachineFinalizer,
				},
				Annotations: map[string]string{
					clusterctlv1.BlockMoveAnnotation: "true",
				},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, machine, asoMachine, bootstrapData).
			Build()
		var reconciled bool
		r := &AzureASOMachineReconciler{
			Client: c,
			newResourceReconciler: func(asoMachine *infrav1alphaexp.AzureASOMachine, us []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					owner: asoMachine,
					reconcileFunc: func(ctx context.Context, _ client.Object) error {
						asoMachine.SetResourceStatuses([]infrav1.ResourceStatus{
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
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoMachine)})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal((ctrl.Result{})))
		g.Expect(reconciled).To(BeTrue())
	})

	t.Run("successfully reconciles normally", func(t *testing.T) {
		g := NewGomegaWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "ns",
			},
			Status: clusterv1.ClusterStatus{
				Initialization: clusterv1.ClusterInitializationStatus{
					InfrastructureProvisioned: ptr.To(true),
				},
			},
		}
		bootstrapData := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bootstrap",
				Namespace: cluster.Namespace,
			},
		}
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine",
				Namespace: cluster.Namespace,
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					DataSecretName: &bootstrapData.Name,
				},
			},
		}
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cluster.Namespace,
				Name:      "my-provider-ids",
			},
			Data: map[string]string{
				"aso-machine": "provider-id",
			},
		}
		asoMachine := &infrav1alphaexp.AzureASOMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aso-machine",
				Namespace: machine.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       "Machine",
						Name:       machine.Name,
					},
				},
				Finalizers: []string{
					infrav1alphaexp.AzureASOMachineFinalizer,
				},
				Annotations: map[string]string{
					clusterctlv1.BlockMoveAnnotation: "true",
				},
			},
			Spec: infrav1alphaexp.AzureASOMachineSpec{
				AzureASOMachineTemplateResourceSpec: infrav1alphaexp.AzureASOMachineTemplateResourceSpec{
					Resources: []runtime.RawExtension{
						{Raw: []byte(`{
							"apiVersion": "v1something",
							"kind": "VirtualMachine",
							"metadata": {"name": "aso-machine"}
						}`)},
					},
				},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, machine, asoMachine, configMap, bootstrapData).
			Build()
		expectReconciled := map[string]struct{}{
			"VirtualMachine/aso-machine": {},
		}
		r := &AzureASOMachineReconciler{
			Client: c,
			newResourceReconciler: func(_ *infrav1alphaexp.AzureASOMachine, us []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					reconcileFunc: func(_ context.Context, _ client.Object) error {
						for _, u := range us {
							key := u.GetKind() + "/" + u.GetName()
							g.Expect(expectReconciled).To(HaveKey(key), "reconciled unexpected resource")
							delete(expectReconciled, key)
						}
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoMachine)})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal((ctrl.Result{})))
		g.Expect(expectReconciled).To(BeEmpty(), "resources should have been reconciled but were not")
	})

	t.Run("successfully reconciles pause", func(t *testing.T) {
		g := NewGomegaWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "ns",
			},
			Spec: clusterv1.ClusterSpec{
				Paused: ptr.To(true),
			},
		}
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine",
				Namespace: cluster.Namespace,
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: cluster.Name,
			},
		}
		asoMachine := &infrav1alphaexp.AzureASOMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aso-machine",
				Namespace: machine.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.Identifier(),
						Kind:       "Machine",
						Name:       machine.Name,
					},
				},
				Annotations: map[string]string{
					clusterctlv1.BlockMoveAnnotation: "true",
				},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, machine, asoMachine).
			Build()
		var reconciled bool
		r := &AzureASOMachineReconciler{
			Client: c,
			newResourceReconciler: func(_ *infrav1alphaexp.AzureASOMachine, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					pauseFunc: func(ctx context.Context, o client.Object) error {
						reconciled = true
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoMachine)})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal((ctrl.Result{})))
		g.Expect(reconciled).To(BeTrue())
	})

	t.Run("successfully reconciles in-progress delete", func(t *testing.T) {
		g := NewGomegaWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "ns",
			},
		}
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine",
				Namespace: cluster.Namespace,
			},
		}
		asoMachine := &infrav1alphaexp.AzureASOMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aso-machine",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
				Finalizers: []string{
					infrav1alphaexp.AzureASOMachineFinalizer,
				},
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, machine, asoMachine).
			Build()
		var reconciled bool
		r := &AzureASOMachineReconciler{
			Client: c,
			newResourceReconciler: func(asoMachine *infrav1alphaexp.AzureASOMachine, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					owner: asoMachine,
					deleteFunc: func(ctx context.Context, o client.Object) error {
						asoMachine.SetResourceStatuses([]infrav1.ResourceStatus{
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
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoMachine)})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal((ctrl.Result{})))
		g.Expect(reconciled).To(BeTrue())

		err = c.Get(ctx, client.ObjectKeyFromObject(asoMachine), asoMachine)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(asoMachine.GetFinalizers()).To(ContainElement(infrav1alphaexp.AzureASOMachineFinalizer))
	})

	t.Run("successfully reconciles finished delete", func(t *testing.T) {
		g := NewGomegaWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster",
				Namespace: "ns",
			},
		}
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine",
				Namespace: cluster.Namespace,
			},
		}
		asoMachine := &infrav1alphaexp.AzureASOMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aso-machine",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
				Finalizers: []string{
					infrav1alphaexp.AzureASOMachineFinalizer,
				},
				DeletionTimestamp: &metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)},
			},
		}
		c := fakeClientBuilder().
			WithObjects(cluster, machine, asoMachine).
			Build()
		var reconciled bool
		r := &AzureASOMachineReconciler{
			Client: c,
			newResourceReconciler: func(asoMachine *infrav1alphaexp.AzureASOMachine, _ []*unstructured.Unstructured) resourceReconciler {
				return &fakeResourceReconciler{
					owner: asoMachine,
					deleteFunc: func(ctx context.Context, o client.Object) error {
						reconciled = true
						return nil
					},
				}
			},
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(asoMachine)})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal((ctrl.Result{})))
		g.Expect(reconciled).To(BeTrue())

		err = c.Get(ctx, client.ObjectKeyFromObject(asoMachine), asoMachine)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
}

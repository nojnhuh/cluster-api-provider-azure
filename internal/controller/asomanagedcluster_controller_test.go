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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1 "github.com/nojnhuh/cluster-api-provider-aso/api/v1alpha1"
)

var _ = Describe("ASOManagedCluster Controller", func() {
	const resourceName = "test-resource"
	var namespace *corev1.Namespace
	var typeNamespacedName types.NamespacedName

	resource := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "configmap",
		},
	}

	ctx := context.Background()

	BeforeEach(func() {
		By("Creating initial state")
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-" + rand.String(5)}}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		resource.SetNamespace(namespace.Name)

		typeNamespacedName = types.NamespacedName{
			Namespace: namespace.Name,
			Name:      resourceName,
		}

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "cluster",
			},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

		asoCluster := &infrav1.ASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace.Name,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       clusterv1.ClusterKind,
						Name:       cluster.Name,
						UID:        cluster.UID,
					},
				},
			},
			Spec: infrav1.ASOManagedClusterSpec{
				Resources: []runtime.RawExtension{
					{
						Object: resource,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, asoCluster)).To(Succeed())
	})

	AfterEach(func() {
		By("Cleanup the test namespace")
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	It("should successfully reconcile the resource", func() {
		By("Reconciling the created resource")
		reconciler := &ASOManagedClusterReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: typeNamespacedName,
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(resource), resource)).To(Succeed())
	})
})

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

	asoresourcesv1 "github.com/Azure/azure-service-operator/v2/api/resources/v1api20200601"
	"github.com/Azure/azure-service-operator/v2/pkg/genruntime/conditions"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
)

type FakeClient struct {
	client.Client
	// Override the Patch method because controller-runtime's doesn't really support
	// server-side apply, so we make our own dollar store version:
	// https://github.com/kubernetes-sigs/controller-runtime/issues/2341
	patchFunc func(context.Context, client.Object, client.Patch, ...client.PatchOption) error
	// Override Delete in order to simulate long-running deletes.
	deleteFunc func(context.Context, client.Object, ...client.DeleteOption) error
}

func (c *FakeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if c.patchFunc == nil {
		return nil
	}
	return c.patchFunc(ctx, obj, patch, opts...)
}

func (c *FakeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.deleteFunc == nil {
		return nil
	}
	return c.deleteFunc(ctx, obj, opts...)
}

type FakeWatcher struct {
	watching map[string]struct{}
}

func (w *FakeWatcher) Watch(_ logr.Logger, obj runtime.Object, _ handler.EventHandler, _ ...predicate.Predicate) error {
	if w.watching == nil {
		w.watching = make(map[string]struct{})
	}
	w.watching[obj.GetObjectKind().GroupVersionKind().GroupKind().String()] = struct{}{}
	return nil
}

func TestInfraReconcilerReconcile(t *testing.T) {
	ctx := context.Background()

	t.Run("empty resources", func(t *testing.T) {
		r := &InfraReconciler{
			resources: []*unstructured.Unstructured{},
			owner:     &infrav1.AzureASOManagedCluster{},
		}

		t.Run("Reconcile", expectSuccess(r.Reconcile(ctx)))
		t.Run("resource statuses length", checkEqual(len(r.owner.GetResourceStatuses()), len(r.resources)))
	})

	t.Run("reconcile several resources", func(t *testing.T) {
		w := &FakeWatcher{}
		s := runtime.NewScheme()
		sb := runtime.NewSchemeBuilder(
			infrav1.AddToScheme,
			asoresourcesv1.AddToScheme,
		)
		t.Run("build scheme", expectSuccess(sb.AddToScheme(s)))
		c := fakeclient.NewClientBuilder().
			WithScheme(s).
			Build()

		r := &InfraReconciler{
			Client: &FakeClient{Client: c},
			resources: []*unstructured.Unstructured{
				rgJSON(t, s, &asoresourcesv1.ResourceGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg1",
					},
					// status is supplied here to simulate the API server response after patching.
					Status: asoresourcesv1.ResourceGroup_STATUS{
						Conditions: conditions.Conditions{
							{
								Type:    conditions.ConditionTypeReady,
								Status:  metav1.ConditionFalse,
								Message: "rg1 message",
							},
						},
					},
				}),
				rgJSON(t, s, &asoresourcesv1.ResourceGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg2",
					},
				}),
			},
			owner: &infrav1.AzureASOManagedCluster{
				Status: infrav1.AzureASOManagedClusterStatus{
					Resources: []infrav1.ResourceStatus{
						{
							Group:   asoresourcesv1.GroupVersion.Group,
							Version: asoresourcesv1.GroupVersion.Version + "this should still match",
							Kind:    "ResourceGroup",
							Name:    "rg1",
							Ready:   true, // this should be overridden
						},
					},
				},
			},
			watcher: w,
		}

		t.Run("Reconcile", expectSuccess(r.Reconcile(ctx)))

		shouldWatch := "ResourceGroup.resources.azure.com"
		if _, watching := w.watching[shouldWatch]; !watching {
			t.Error("watched resources", w.watching, "does not include", shouldWatch)
		}

		t.Run("resource statuses", func(t *testing.T) {
			t.Run("length", checkEqual(len(r.owner.GetResourceStatuses()), len(r.resources)))
			t.Run("0", func(t *testing.T) {
				status := r.owner.GetResourceStatuses()[0]
				t.Run("group", checkEqual(status.Group, asoresourcesv1.GroupVersion.Group))
				t.Run("version", checkEqual(status.Version, asoresourcesv1.GroupVersion.Version))
				t.Run("kind", checkEqual(status.Kind, "ResourceGroup"))
				t.Run("name", checkEqual(status.Name, "rg1"))
				t.Run("ready", checkEqual(status.Ready, false))
				t.Run("message", checkEqual(status.Message, "rg1 message"))
			})
			t.Run("1", func(t *testing.T) {
				status := r.owner.GetResourceStatuses()[1]
				t.Run("group", checkEqual(status.Group, asoresourcesv1.GroupVersion.Group))
				t.Run("version", checkEqual(status.Version, asoresourcesv1.GroupVersion.Version))
				t.Run("kind", checkEqual(status.Kind, "ResourceGroup"))
				t.Run("name", checkEqual(status.Name, "rg2"))
				t.Run("ready", checkEqual(status.Ready, false))
				t.Run("message", checkEqual(status.Message, ""))
			})
		})
	})

	t.Run("reap stale resources", func(t *testing.T) {
		s := runtime.NewScheme()
		sb := runtime.NewSchemeBuilder(
			infrav1.AddToScheme,
			asoresourcesv1.AddToScheme,
		)
		t.Run("build scheme", expectSuccess(sb.AddToScheme(s)))

		owner := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
			},
			Status: infrav1.AzureASOManagedClusterStatus{
				Resources: []infrav1.ResourceStatus{
					{
						Group:   asoresourcesv1.GroupVersion.Group,
						Version: asoresourcesv1.GroupVersion.Version,
						Kind:    "ResourceGroup",
						Name:    "rg1",
					},
					{
						Group:   asoresourcesv1.GroupVersion.Group,
						Version: asoresourcesv1.GroupVersion.Version,
						Kind:    "ResourceGroup",
						Name:    "rg2",
					},
				},
			},
		}

		objs := []client.Object{
			&asoresourcesv1.ResourceGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rg1",
					Namespace: owner.Namespace,
				},
			},
		}

		c := fakeclient.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			Build()

		r := &InfraReconciler{
			Client:    c,
			resources: []*unstructured.Unstructured{},
			owner:     owner,
		}

		t.Run("Reconcile", expectSuccess(r.Reconcile(ctx)))

		t.Run("resource statuses", func(t *testing.T) {
			t.Run("length", checkEqual(len(r.owner.GetResourceStatuses()), 1))
			t.Run("0", func(t *testing.T) {
				status := r.owner.GetResourceStatuses()[0]
				t.Run("group", checkEqual(status.Group, asoresourcesv1.GroupVersion.Group))
				t.Run("version", checkEqual(status.Version, asoresourcesv1.GroupVersion.Version))
				t.Run("kind", checkEqual(status.Kind, "ResourceGroup"))
				t.Run("name", checkEqual(status.Name, "rg1"))
				t.Run("ready", checkEqual(status.Ready, false))
				t.Run("message", checkEqual(status.Message, ""))
			})
		})
	})
}

func TestInfraReconcilerDelete(t *testing.T) {
	ctx := context.Background()

	t.Run("empty resources", func(t *testing.T) {
		r := &InfraReconciler{
			resources: []*unstructured.Unstructured{},
			owner:     &infrav1.AzureASOManagedCluster{},
		}

		t.Run("Delete", expectSuccess(r.Delete(ctx)))
		t.Run("resource statuses length", checkEqual(len(r.owner.GetResourceStatuses()), len(r.resources)))
	})

	t.Run("delete several resources", func(t *testing.T) {
		s := runtime.NewScheme()
		sb := runtime.NewSchemeBuilder(
			infrav1.AddToScheme,
			asoresourcesv1.AddToScheme,
		)
		t.Run("build scheme", expectSuccess(sb.AddToScheme(s)))

		owner := &infrav1.AzureASOManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
			},
			Status: infrav1.AzureASOManagedClusterStatus{
				Resources: []infrav1.ResourceStatus{
					{
						Group:   asoresourcesv1.GroupVersion.Group,
						Version: asoresourcesv1.GroupVersion.Version,
						Kind:    "ResourceGroup",
						Name:    "rg1",
					},
					{
						Group:   asoresourcesv1.GroupVersion.Group,
						Version: asoresourcesv1.GroupVersion.Version,
						Kind:    "ResourceGroup",
						Name:    "rg2",
					},
				},
			},
		}

		objs := []client.Object{
			&asoresourcesv1.ResourceGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rg1",
					Namespace: owner.Namespace,
				},
				Status: asoresourcesv1.ResourceGroup_STATUS{
					Conditions: conditions.Conditions{
						{
							Type:    conditions.ConditionTypeReady,
							Status:  metav1.ConditionFalse,
							Message: "rg1 message",
						},
					},
				},
			},
		}

		c := fakeclient.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			Build()

		r := &InfraReconciler{
			Client: &FakeClient{Client: c},
			resources: []*unstructured.Unstructured{
				rgJSON(t, s, &asoresourcesv1.ResourceGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg1",
					},
				}),
				rgJSON(t, s, &asoresourcesv1.ResourceGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rg2",
					},
				}),
			},
			owner: owner,
		}

		t.Run("Delete", expectSuccess(r.Delete(ctx)))

		t.Run("resource statuses length", func(t *testing.T) {
			t.Run("resource statuses length", checkEqual(len(r.owner.GetResourceStatuses()), 1))
			t.Run("0", func(t *testing.T) {
				status := r.owner.GetResourceStatuses()[0]
				t.Run("group", checkEqual(status.Group, asoresourcesv1.GroupVersion.Group))
				t.Run("version", checkEqual(status.Version, asoresourcesv1.GroupVersion.Version))
				t.Run("kind", checkEqual(status.Kind, "ResourceGroup"))
				t.Run("name", checkEqual(status.Name, "rg1"))
				t.Run("ready", checkEqual(status.Ready, false))
				t.Run("message", checkEqual(status.Message, "rg1 message"))
			})
		})
	})
}

func TestReadyStatus(t *testing.T) {
	t.Run("unstructured", func(t *testing.T) {
		tests := []struct {
			name            string
			object          *unstructured.Unstructured
			expectedReady   bool
			expectedMessage string
		}{
			{
				name:            "empty object",
				object:          &unstructured.Unstructured{Object: make(map[string]interface{})},
				expectedReady:   false,
				expectedMessage: "",
			},
			{
				name: "empty status.conditions",
				object: &unstructured.Unstructured{Object: map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{},
					},
				}},
				expectedReady:   false,
				expectedMessage: "",
			},
			{
				name: "status.conditions wrong type",
				object: &unstructured.Unstructured{Object: map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							int64(0),
						},
					},
				}},
				expectedReady:   false,
				expectedMessage: "",
			},
			{
				name: "non-Ready type status.conditions",
				object: &unstructured.Unstructured{Object: map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type": "not" + conditions.ConditionTypeReady,
							},
						},
					},
				}},
				expectedReady:   false,
				expectedMessage: "",
			},
			{
				name: "observedGeneration not up to date",
				object: &unstructured.Unstructured{Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"generation": int64(1),
					},
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":               conditions.ConditionTypeReady,
								"observedGeneration": int64(0),
							},
						},
					},
				}},
				expectedReady:   false,
				expectedMessage: waitingForASOReconcileMsg,
			},
			{
				name: "status is not defined",
				object: &unstructured.Unstructured{Object: map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":    conditions.ConditionTypeReady,
								"message": "a message",
							},
						},
					},
				}},
				expectedReady:   false,
				expectedMessage: "",
			},
			{
				name: "status is not True",
				object: &unstructured.Unstructured{Object: map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":    conditions.ConditionTypeReady,
								"status":  "not-" + string(metav1.ConditionTrue),
								"message": "a message",
							},
						},
					},
				}},
				expectedReady:   false,
				expectedMessage: "a message",
			},
			{
				name: "message wrong type",
				object: &unstructured.Unstructured{Object: map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":    conditions.ConditionTypeReady,
								"status":  string(metav1.ConditionTrue),
								"message": int64(0),
							},
						},
					},
				}},
				expectedReady:   false,
				expectedMessage: "",
			},
			{
				name: "status is True",
				object: &unstructured.Unstructured{Object: map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":   "not-" + conditions.ConditionTypeReady,
								"status": "not-" + string(metav1.ConditionTrue),
							},
							map[string]interface{}{
								"type":   conditions.ConditionTypeReady,
								"status": string(metav1.ConditionTrue),
							},
							map[string]interface{}{
								"type":   "not-" + conditions.ConditionTypeReady,
								"status": "not-" + string(metav1.ConditionTrue),
							},
						},
					},
				}},
				expectedReady:   true,
				expectedMessage: "",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				ready, message := readyStatus(test.object)
				t.Run("ready", checkEqual(ready, test.expectedReady))
				t.Run("message", checkEqual(message, test.expectedMessage))
			})
		}
	})

	// These tests verify readyStatus() on an actual ASO typed object to ensure the unstructured assertions
	// work on the actual structure of ASO objects.
	t.Run("ResourceGroup", func(t *testing.T) {
		tests := []struct {
			name            string
			conditions      conditions.Conditions
			expectedReady   bool
			expectedMessage string
		}{
			{
				name:            "empty conditions",
				conditions:      nil,
				expectedReady:   false,
				expectedMessage: "",
			},
			{
				name: "not ready conditions",
				conditions: conditions.Conditions{
					{
						Type:    conditions.ConditionTypeReady,
						Status:  metav1.ConditionFalse,
						Message: "a message",
					},
					{
						Type:    "not-" + conditions.ConditionTypeReady,
						Status:  metav1.ConditionTrue,
						Message: "another message",
					},
				},
				expectedReady:   false,
				expectedMessage: "a message",
			},
			{
				name: "ready conditions",
				conditions: conditions.Conditions{
					{
						Type:    "not-" + conditions.ConditionTypeReady,
						Status:  metav1.ConditionTrue,
						Message: "another message",
					},
					{
						Type:    conditions.ConditionTypeReady,
						Status:  metav1.ConditionTrue,
						Message: "a message",
					},
					{
						Type:    "not-" + conditions.ConditionTypeReady,
						Status:  metav1.ConditionTrue,
						Message: "another message",
					},
				},
				expectedReady:   true,
				expectedMessage: "a message",
			},
		}

		s := runtime.NewScheme()
		t.Run("build scheme", expectSuccess(asoresourcesv1.AddToScheme(s)))

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				rg := &asoresourcesv1.ResourceGroup{
					Status: asoresourcesv1.ResourceGroup_STATUS{
						Conditions: test.conditions,
					},
				}
				u := &unstructured.Unstructured{}
				t.Run("convert to unstructured", expectSuccess(s.Convert(rg, u, nil)))

				ready, message := readyStatus(u)
				t.Run("ready", checkEqual(ready, test.expectedReady))
				t.Run("message", checkEqual(message, test.expectedMessage))
			})
		}
	})
}

func rgJSON(t *testing.T, scheme *runtime.Scheme, rg *asoresourcesv1.ResourceGroup) *unstructured.Unstructured {
	t.Helper()
	rg.SetGroupVersionKind(asoresourcesv1.GroupVersion.WithKind("ResourceGroup"))
	u := &unstructured.Unstructured{}
	t.Run("convert to unstructured", expectSuccess(scheme.Convert(rg, u, nil)))
	return u
}

func expectSuccess(err error) func(*testing.T) {
	return func(t *testing.T) {
		t.Helper()
		if err != nil {
			t.Error("expected success, got error", err)
		}
	}
}

func checkEqual[T comparable](actual, expected T) func(*testing.T) {
	return func(t *testing.T) {
		t.Helper()
		if actual != expected {
			t.Errorf("expected %v, got %v", expected, actual)
		}
	}
}

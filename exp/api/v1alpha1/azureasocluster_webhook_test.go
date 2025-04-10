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

package v1alpha1

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestAzureASOClusterWebhookValidateCreate(t *testing.T) {
	tests := []struct {
		name       string
		asoCluster *AzureASOCluster
		matchErrs  types.GomegaMatcher
	}{
		{
			name:       "empty",
			asoCluster: &AzureASOCluster{},
			matchErrs:  Not(HaveOccurred()),
		},
		{
			name: "missing JSON patch fields",
			asoCluster: &AzureASOCluster{
				Spec: AzureASOClusterSpec{
					AzureASOClusterTemplateResourceSpec: AzureASOClusterTemplateResourceSpec{
						Patches: []ResourcesPatch{
							{
								JSONPatches: []JSONPatch{
									{Op: JSONPatchOpCopy, Path: "/", From: "/"},
									{Op: JSONPatchOpCopy, Path: "/"},
								},
							},
						},
					},
				},
			},
			matchErrs: ConsistOf(
				MatchError(field.Required(field.NewPath("spec", "patches").Index(0).Child("jsonPatches").Index(1).Child("from"), "required for \"copy\" operations")),
			),
		},
		{
			name: "spec.controlPlaneEndpoint without spec.controlPlaneEndpointSource",
			asoCluster: &AzureASOCluster{
				Spec: AzureASOClusterSpec{
					ControlPlaneEndpoint: clusterv1.APIEndpoint{
						Host: "127.0.0.1",
						Port: 443,
					},
				},
			},
			matchErrs: Not(HaveOccurred()),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			g := NewGomegaWithT(t)
			a := &azureASOClusterWebhook{}

			if test.matchErrs == nil {
				test.matchErrs = BeEmpty()
			}

			actualWarnings, actualErrs := a.ValidateCreate(ctx, test.asoCluster)
			g.Expect(actualWarnings).To(BeEmpty())
			g.Expect(actualErrs).To(test.matchErrs)
		})
	}
}

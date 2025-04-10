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
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestAzureASOClusterTemplateWebhookValidateCreate(t *testing.T) {
	tests := []struct {
		name               string
		asoClusterTemplate *AzureASOClusterTemplate
		matchErrs          types.GomegaMatcher
	}{
		{
			name:               "empty",
			asoClusterTemplate: &AzureASOClusterTemplate{},
			matchErrs:          Not(HaveOccurred()),
		},
		{
			name: "missing JSON patch fields",
			asoClusterTemplate: &AzureASOClusterTemplate{
				Spec: AzureASOClusterTemplateSpec{
					Template: AzureASOClusterTemplateResource{
						Spec: AzureASOClusterTemplateResourceSpec{
							Patches: []ResourcesPatch{
								{
									JSONPatches: []JSONPatch{
										{Op: JSONPatchOpAdd, Path: "/", Value: &apiextensionsv1.JSON{Raw: []byte(`7`)}},
										{Op: JSONPatchOpAdd, Path: "/"},
									},
								},
								{
									JSONPatches: []JSONPatch{
										{Op: JSONPatchOpReplace, Path: "/", Value: &apiextensionsv1.JSON{Raw: []byte(`7`)}},
										{Op: JSONPatchOpReplace, Path: "/"},
									},
								},
								{
									JSONPatches: []JSONPatch{
										{Op: JSONPatchOpTest, Path: "/", Value: &apiextensionsv1.JSON{Raw: []byte(`7`)}},
										{Op: JSONPatchOpTest, Path: "/"},
									},
								},
								{
									JSONPatches: []JSONPatch{
										{Op: JSONPatchOpMove, Path: "/", From: "/"},
										{Op: JSONPatchOpMove, Path: "/"},
									},
								},
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
			},
			matchErrs: ConsistOf(
				MatchError(field.Required(field.NewPath("spec", "template", "spec", "patches").Index(0).Child("jsonPatches").Index(1).Child("value"), "required for \"add\" operations")),
				MatchError(field.Required(field.NewPath("spec", "template", "spec", "patches").Index(1).Child("jsonPatches").Index(1).Child("value"), "required for \"replace\" operations")),
				MatchError(field.Required(field.NewPath("spec", "template", "spec", "patches").Index(2).Child("jsonPatches").Index(1).Child("value"), "required for \"test\" operations")),
				MatchError(field.Required(field.NewPath("spec", "template", "spec", "patches").Index(3).Child("jsonPatches").Index(1).Child("from"), "required for \"move\" operations")),
				MatchError(field.Required(field.NewPath("spec", "template", "spec", "patches").Index(4).Child("jsonPatches").Index(1).Child("from"), "required for \"copy\" operations")),
			),
		},
	}

	ctx := context.Background()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			a := &azureASOClusterTemplateWebhook{}

			if test.matchErrs == nil {
				test.matchErrs = BeEmpty()
			}

			actualWarnings, actualErrs := a.ValidateCreate(ctx, test.asoClusterTemplate)
			g.Expect(actualWarnings).To(BeEmpty())
			g.Expect(actualErrs).To(test.matchErrs)
		})
	}
}

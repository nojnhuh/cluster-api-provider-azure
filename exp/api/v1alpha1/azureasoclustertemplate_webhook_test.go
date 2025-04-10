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
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
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
										{Op: JSONPatchOpAdd, Path: "/", Value: &apiextensionsv1.JSON{Raw: []byte(`value`)}},
										{Op: JSONPatchOpAdd, Path: "/", ValueFrom: &JSONPatchValueFrom{Template: ptr.To("template")}},
										{Op: JSONPatchOpAdd, Path: "/"},
										{Op: JSONPatchOpAdd, Path: "/", ValueFrom: &JSONPatchValueFrom{Template: ptr.To("{{")}},
										{Op: JSONPatchOpAdd, Path: "/", ValueFrom: &JSONPatchValueFrom{}},
									},
								},
								{
									JSONPatches: []JSONPatch{
										{Op: JSONPatchOpReplace, Path: "/", Value: &apiextensionsv1.JSON{Raw: []byte(`value`)}},
										{Op: JSONPatchOpReplace, Path: "/", ValueFrom: &JSONPatchValueFrom{Template: ptr.To("template")}},
										{Op: JSONPatchOpReplace, Path: "/"},
									},
								},
								{
									JSONPatches: []JSONPatch{
										{Op: JSONPatchOpTest, Path: "/", Value: &apiextensionsv1.JSON{Raw: []byte(`value`)}},
										{Op: JSONPatchOpTest, Path: "/", ValueFrom: &JSONPatchValueFrom{Template: ptr.To("template")}},
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
				MatchError(field.Required(field.NewPath("spec", "template", "spec", "patches").Index(0).Child("jsonPatches").Index(2), "one of \"value\" or \"valueFrom\" is required for \"add\" operations")),
				MatchError(field.Invalid(field.NewPath("spec", "template", "spec", "patches").Index(0).Child("jsonPatches").Index(3).Child("valueFrom", "template"), "{{", "template: tpl:1: unclosed action")),
				MatchError(field.Required(field.NewPath("spec", "template", "spec", "patches").Index(0).Child("jsonPatches").Index(4).Child("valueFrom"), "must set exactly one of `template`")),
				MatchError(field.Required(field.NewPath("spec", "template", "spec", "patches").Index(1).Child("jsonPatches").Index(2), "one of \"value\" or \"valueFrom\" is required for \"replace\" operations")),
				MatchError(field.Required(field.NewPath("spec", "template", "spec", "patches").Index(2).Child("jsonPatches").Index(2), "one of \"value\" or \"valueFrom\" is required for \"test\" operations")),
				MatchError(field.Required(field.NewPath("spec", "template", "spec", "patches").Index(3).Child("jsonPatches").Index(1).Child("from"), "required for \"move\" operations")),
				MatchError(field.Required(field.NewPath("spec", "template", "spec", "patches").Index(4).Child("jsonPatches").Index(1).Child("from"), "required for \"copy\" operations")),
			),
		},
		{
			name: "spec.controlPlaneEndpointSource invalid ConfigMap reference",
			asoClusterTemplate: &AzureASOClusterTemplate{
				Spec: AzureASOClusterTemplateSpec{
					Template: AzureASOClusterTemplateResource{
						Spec: AzureASOClusterTemplateResourceSpec{
							ControlPlaneEndpointSource: &ControlPlaneEndpointSource{
								Host: &StringSource{
									ConfigMap: &ConfigMapReference{
										Name: StringValue{
											Value: ptr.To(strings.Repeat("a", validation.DNS1123SubdomainMaxLength+1)),
										},
										Key: StringValue{
											Value: ptr.To("!"),
										},
									},
								},
							},
						},
					},
				},
			},
			matchErrs: ConsistOf(
				MatchError(field.Invalid(field.NewPath("spec", "template", "spec", "controlPlaneEndpointSource", "host", "configMap", "name", "value"), strings.Repeat("a", validation.DNS1123SubdomainMaxLength+1), "must be no more than 253 characters")),
				MatchError(field.Invalid(field.NewPath("spec", "template", "spec", "controlPlaneEndpointSource", "host", "configMap", "key", "value"), "!", "a valid config key must consist of alphanumeric characters, '-', '_' or '.' (e.g. 'key.name',  or 'KEY_NAME',  or 'key-name', regex used for validation is '[-._a-zA-Z0-9]+')")),
			),
		},
		{
			name: "spec.controlPlaneEndpointSource without spec.controlPlaneEndpoint",
			asoClusterTemplate: &AzureASOClusterTemplate{
				Spec: AzureASOClusterTemplateSpec{
					Template: AzureASOClusterTemplateResource{
						Spec: AzureASOClusterTemplateResourceSpec{
							ControlPlaneEndpointSource: &ControlPlaneEndpointSource{
								Host: &StringSource{
									ConfigMap: &ConfigMapReference{
										Name: StringValue{
											Value: ptr.To("host-configmap"),
										},
										Key: StringValue{
											Value: ptr.To("host"),
										},
									},
								},
								Port: &StringSource{
									ConfigMap: &ConfigMapReference{
										Name: StringValue{
											Value: ptr.To("port-configmap"),
										},
										Key: StringValue{
											Value: ptr.To("port"),
										},
									},
								},
							},
						},
					},
				},
			},
			matchErrs: Not(HaveOccurred()),
		},
		{
			name: "no host set in ControlPlaneEndpointSource",
			asoClusterTemplate: &AzureASOClusterTemplate{
				Spec: AzureASOClusterTemplateSpec{
					Template: AzureASOClusterTemplateResource{
						Spec: AzureASOClusterTemplateResourceSpec{
							ControlPlaneEndpointSource: &ControlPlaneEndpointSource{
								Host: nil, // The user might set host themselves
								Port: &StringSource{
									ConfigMap: &ConfigMapReference{
										Name: StringValue{
											Value: ptr.To("port-configmap"),
										},
										Key: StringValue{
											Value: ptr.To("port"),
										},
									},
								},
							},
						},
					},
				},
			},
			matchErrs: Not(HaveOccurred()),
		},
		{
			name: "no patch set in ControlPlaneEndpointSource",
			asoClusterTemplate: &AzureASOClusterTemplate{
				Spec: AzureASOClusterTemplateSpec{
					Template: AzureASOClusterTemplateResource{
						Spec: AzureASOClusterTemplateResourceSpec{
							ControlPlaneEndpointSource: &ControlPlaneEndpointSource{
								Host: &StringSource{
									ConfigMap: &ConfigMapReference{
										Name: StringValue{
											Value: ptr.To("host-configmap"),
										},
										Key: StringValue{
											Value: ptr.To("host"),
										},
									},
								},
								Port: nil, // The user might set port themselves
							},
						},
					},
				},
			},
			matchErrs: Not(HaveOccurred()),
		},
		{
			name: "spec.controlPlaneEndpointSource ConfigMap references",
			asoClusterTemplate: &AzureASOClusterTemplate{
				Spec: AzureASOClusterTemplateSpec{
					Template: AzureASOClusterTemplateResource{
						Spec: AzureASOClusterTemplateResourceSpec{
							ControlPlaneEndpointSource: &ControlPlaneEndpointSource{
								Host: &StringSource{
									ConfigMap: &ConfigMapReference{
										Name: StringValue{
											Value: ptr.To("host-configmap"),
										},
										Key: StringValue{
											Template: ptr.To(""),
										},
									},
								},
								Port: &StringSource{
									ConfigMap: &ConfigMapReference{
										Name: StringValue{
											Value:    ptr.To("port-configmap"),
											Template: ptr.To("{{"),
										},
										Key: StringValue{},
									},
								},
							},
						},
					},
				},
			},
			matchErrs: ConsistOf(
				MatchError(field.Invalid(field.NewPath("spec", "template", "spec", "controlPlaneEndpointSource", "port", "configMap", "name", "template"), "{{", "template: tpl:1: unclosed action")),
				MatchError(field.Invalid(field.NewPath("spec", "template", "spec", "controlPlaneEndpointSource", "port", "configMap", "name"), "[value, template]", "must set exactly one of `value`, `template`")),
				MatchError(field.Required(field.NewPath("spec", "template", "spec", "controlPlaneEndpointSource", "port", "configMap", "key"), "must set exactly one of `value`, `template`")),
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
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

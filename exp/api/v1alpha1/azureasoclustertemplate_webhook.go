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
	"fmt"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupAzureASOClusterTemplateWebhookWithManager sets up and registers the webhook with the manager.
func SetupAzureASOClusterTemplateWebhookWithManager(mgr ctrl.Manager) error {
	azureASOClusterTemplateWebhook := &azureASOClusterTemplateWebhook{}
	return ctrl.NewWebhookManagedBy(mgr, &AzureASOClusterTemplate{}).
		WithValidator(azureASOClusterTemplateWebhook).
		Complete()
}

// azureASOClusterTemplateWebhook implements a validating and defaulting webhook for AzureASOClusterTemplate.
type azureASOClusterTemplateWebhook struct {
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-azureasoclustertemplate,mutating=false,failurePolicy=fail,groups=infrastructure.cluster.x-k8s.io,resources=azureasoclustertemplates,versions=v1alpha1,name=validation.azureasoclustertemplate.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (a *azureASOClusterTemplateWebhook) ValidateCreate(_ context.Context, asoClusterTemplate *AzureASOClusterTemplate) (admission.Warnings, error) {
	var allErrs field.ErrorList

	if asoClusterTemplate == nil {
		return nil, nil
	}

	allErrs = append(allErrs, validateAzureASOClusterTemplateSpec(field.NewPath("spec"), asoClusterTemplate.Spec)...)

	return nil, allErrs.ToAggregate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (a *azureASOClusterTemplateWebhook) ValidateUpdate(ctx context.Context, _, asoClusterTemplate *AzureASOClusterTemplate) (admission.Warnings, error) {
	return a.ValidateCreate(ctx, asoClusterTemplate)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (*azureASOClusterTemplateWebhook) ValidateDelete(_ context.Context, _ *AzureASOClusterTemplate) (admission.Warnings, error) {
	return nil, nil
}

func validateAzureASOClusterTemplateSpec(path *field.Path, spec AzureASOClusterTemplateSpec) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateAzureASOClusterTemplateResource(path.Child("template"), spec.Template)...)
	return allErrs
}

func validateAzureASOClusterTemplateResource(path *field.Path, template AzureASOClusterTemplateResource) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateAzureASOClusterTemplateResourceSpec(path.Child("spec"), template.Spec)...)
	return allErrs
}

func validateAzureASOClusterTemplateResourceSpec(path *field.Path, spec AzureASOClusterTemplateResourceSpec) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateResourcesPatches(path.Child("patches"), spec.Patches)...)
	if spec.ControlPlaneEndpointSource != nil {
		allErrs = append(allErrs, validateControlPlaneEndpointSource(path.Child("controlPlaneEndpointSource"), *spec.ControlPlaneEndpointSource)...)
	}
	return allErrs
}

func validateResourcesPatches(path *field.Path, patches []ResourcesPatch) field.ErrorList {
	var allErrs field.ErrorList
	for i, patch := range patches {
		allErrs = append(allErrs, validateResourcesPatch(path.Index(i), patch)...)
	}
	return allErrs
}

func validateResourcesPatch(path *field.Path, patch ResourcesPatch) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateJSONPatches(path.Child("jsonPatches"), patch.JSONPatches)...)
	return allErrs
}

func validateJSONPatches(path *field.Path, jsonPatches []JSONPatch) field.ErrorList {
	var allErrs field.ErrorList
	for i, jsonPatch := range jsonPatches {
		allErrs = append(allErrs, validateJSONPatch(path.Index(i), jsonPatch)...)
	}
	return allErrs
}

func validateJSONPatch(path *field.Path, jsonPatch JSONPatch) field.ErrorList {
	var allErrs field.ErrorList
	switch jsonPatch.Op {
	case JSONPatchOpAdd, JSONPatchOpReplace, JSONPatchOpTest:
		if jsonPatch.Value == nil && jsonPatch.ValueFrom == nil {
			allErrs = append(allErrs, field.Required(path, fmt.Sprintf("one of \"value\" or \"valueFrom\" is required for %q operations", jsonPatch.Op)))
		}
	case JSONPatchOpMove, JSONPatchOpCopy:
		if jsonPatch.From == "" {
			allErrs = append(allErrs, field.Required(path.Child("from"), fmt.Sprintf("required for %q operations", jsonPatch.Op)))
		}
	}
	if jsonPatch.ValueFrom != nil {
		allErrs = append(allErrs, validateJSONPatchValueFrom(path.Child("valueFrom"), *jsonPatch.ValueFrom)...)
	}
	return allErrs
}

func validateJSONPatchValueFrom(path *field.Path, valueFrom JSONPatchValueFrom) field.ErrorList {
	var allErrs field.ErrorList

	var fieldsSet []string
	if valueFrom.Template != nil {
		fieldsSet = append(fieldsSet, "template")
		allErrs = append(allErrs, validateStringTemplate(path.Child("template"), *valueFrom.Template)...)
	}

	msg := "must set exactly one of `template`"
	switch len(fieldsSet) {
	case 1:
		// ok
	case 0:
		allErrs = append(allErrs, field.Required(path, msg))
	default:
		// unreachable until at least one other source is added to the API.
		allErrs = append(allErrs, field.Invalid(path, fmt.Sprintf("[%s]", strings.Join(fieldsSet, ", ")), msg))
	}

	return allErrs
}

func validateStringTemplate(path *field.Path, tpl string) field.ErrorList {
	var allErrs field.ErrorList
	if _, err := template.New("tpl").Parse(tpl); err != nil {
		allErrs = append(allErrs, field.Invalid(path, tpl, err.Error()))
	}
	return allErrs
}

func validateControlPlaneEndpointSource(path *field.Path, source ControlPlaneEndpointSource) field.ErrorList {
	var allErrs field.ErrorList

	if source.Host != nil {
		allErrs = append(allErrs, validateStringSource(path.Child("host"), *source.Host)...)
	}
	if source.Port != nil {
		allErrs = append(allErrs, validateStringSource(path.Child("port"), *source.Port)...)
	}

	return allErrs
}

func validateStringSource(path *field.Path, s StringSource) field.ErrorList {
	var allErrs field.ErrorList

	var fieldsSet []string
	if s.ConfigMap != nil {
		fieldsSet = append(fieldsSet, "configMap")
		allErrs = append(allErrs, validateConfigMapReference(path.Child("configMap"), *s.ConfigMap)...)
	}

	// unreachable until at least one other source is added to the API.
	msg := "must set at most one of `configMap`"
	if len(fieldsSet) > 1 {
		allErrs = append(allErrs, field.Invalid(path, fmt.Sprintf("[%s]", strings.Join(fieldsSet, ", ")), msg))
	}

	return allErrs
}

func validateConfigMapReference(path *field.Path, ref ConfigMapReference) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateConfigMapReferenceName(path.Child("name"), ref.Name)...)
	allErrs = append(allErrs, validateConfigMapReferenceKey(path.Child("key"), ref.Key)...)
	return allErrs
}

func validateConfigMapReferenceName(path *field.Path, name StringValue) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateStringValue(path, name)...)
	if name.Value != nil {
		allErrs = append(allErrs, validateConfigMapReferenceNameValue(path.Child("value"), *name.Value)...)
	}
	return allErrs
}

func validateConfigMapReferenceNameValue(path *field.Path, name string) field.ErrorList {
	var allErrs field.ErrorList
	for _, err := range validation.IsDNS1123Subdomain(name) {
		allErrs = append(allErrs, field.Invalid(path, name, err))
	}
	return allErrs
}

func validateConfigMapReferenceKey(path *field.Path, key StringValue) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateStringValue(path, key)...)
	if key.Value != nil {
		allErrs = append(allErrs, validateConfigMapReferenceKeyValue(path.Child("value"), *key.Value)...)
	}
	return allErrs
}

func validateConfigMapReferenceKeyValue(path *field.Path, key string) field.ErrorList {
	var allErrs field.ErrorList
	for _, err := range validation.IsConfigMapKey(key) {
		allErrs = append(allErrs, field.Invalid(path, key, err))
	}
	return allErrs
}

func validateStringValue(path *field.Path, s StringValue) field.ErrorList {
	var allErrs field.ErrorList

	var fieldsSet []string
	if s.Value != nil {
		fieldsSet = append(fieldsSet, "value")
	}
	if s.Template != nil {
		fieldsSet = append(fieldsSet, "template")
		allErrs = append(allErrs, validateStringTemplate(path.Child("template"), *s.Template)...)
	}

	msg := "must set exactly one of `value`, `template`"
	switch len(fieldsSet) {
	case 1:
		// ok
	case 0:
		allErrs = append(allErrs, field.Required(path, msg))
	default:
		allErrs = append(allErrs, field.Invalid(path, fmt.Sprintf("[%s]", strings.Join(fieldsSet, ", ")), msg))
	}

	return allErrs
}

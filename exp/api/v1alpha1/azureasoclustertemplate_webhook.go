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

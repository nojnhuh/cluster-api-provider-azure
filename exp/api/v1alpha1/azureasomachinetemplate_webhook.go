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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupAzureASOMachineTemplateWebhookWithManager sets up and registers the webhook with the manager.
func SetupAzureASOMachineTemplateWebhookWithManager(mgr ctrl.Manager) error {
	azureASOMachineTemplateWebhook := &azureASOMachineTemplateWebhook{}
	return ctrl.NewWebhookManagedBy(mgr).
		For(&AzureASOMachine{}).
		WithValidator(azureASOMachineTemplateWebhook).
		Complete()
}

// azureASOMachineTemplateWebhook implements a validating and defaulting webhook for AzureASOMachineTemplate.
type azureASOMachineTemplateWebhook struct {
}

// +kubebuilder:webhook:verbs=create,path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-azureasoclustertemplate,mutating=false,failurePolicy=fail,groups=infrastructure.cluster.x-k8s.io,resources=azureasoclustertemplates,versions=v1alpha1,name=validation.azureasoclustertemplate.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (a *azureASOMachineTemplateWebhook) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList

	asoMachineTemplate, ok := obj.(*AzureASOMachineTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest("expected an AzureASOMachineTemplate")
	}

	if asoMachineTemplate == nil {
		return nil, nil
	}

	allErrs = append(allErrs, validateAzureASOMachineTemplateSpec(field.NewPath("spec"), asoMachineTemplate.Spec)...)

	return nil, allErrs.ToAggregate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (a *azureASOMachineTemplateWebhook) ValidateUpdate(ctx context.Context, _, obj runtime.Object) (admission.Warnings, error) {
	return a.ValidateCreate(ctx, obj)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (*azureASOMachineTemplateWebhook) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func validateAzureASOMachineTemplateSpec(path *field.Path, spec AzureASOMachineTemplateSpec) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateAzureASOMachineTemplateResource(path.Child("template"), spec.Template)...)
	return allErrs
}

func validateAzureASOMachineTemplateResource(path *field.Path, template AzureASOMachineTemplateResource) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateAzureASOMachineTemplateResourceSpec(path.Child("spec"), template.Spec)...)
	return allErrs
}

func validateAzureASOMachineTemplateResourceSpec(path *field.Path, spec AzureASOMachineTemplateResourceSpec) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateResourcesPatches(path.Child("patches"), spec.Patches)...)
	return allErrs
}

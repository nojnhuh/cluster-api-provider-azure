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

	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupAzureASOMachineWebhookWithManager sets up and registers the webhook with the manager.
func SetupAzureASOMachineWebhookWithManager(mgr ctrl.Manager) error {
	azureASOMachineWebhook := &azureASOMachineWebhook{}
	return ctrl.NewWebhookManagedBy(mgr, &AzureASOMachine{}).
		WithValidator(azureASOMachineWebhook).
		Complete()
}

// azureASOMachineWebhook implements a validating and defaulting webhook for AzureASOMachine.
type azureASOMachineWebhook struct {
}

// +kubebuilder:webhook:verbs=create,path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-azureasomachine,mutating=false,failurePolicy=fail,groups=infrastructure.cluster.x-k8s.io,resources=azureasomachines,versions=v1alpha1,name=validation.azureasomachine.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (a *azureASOMachineWebhook) ValidateCreate(_ context.Context, asoMachine *AzureASOMachine) (admission.Warnings, error) {
	var allErrs field.ErrorList

	if asoMachine == nil {
		return nil, nil
	}

	allErrs = append(allErrs, validateAzureASOMachineSpec(field.NewPath("spec"), asoMachine.Spec)...)

	return nil, allErrs.ToAggregate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (a *azureASOMachineWebhook) ValidateUpdate(ctx context.Context, _, asoMachine *AzureASOMachine) (admission.Warnings, error) {
	return a.ValidateCreate(ctx, asoMachine)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (*azureASOMachineWebhook) ValidateDelete(_ context.Context, _ *AzureASOMachine) (admission.Warnings, error) {
	return nil, nil
}

func validateAzureASOMachineSpec(path *field.Path, spec AzureASOMachineSpec) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateAzureASOMachineTemplateResourceSpec(path, spec.AzureASOMachineTemplateResourceSpec)...)
	return allErrs
}

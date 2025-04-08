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

// SetupAzureASOClusterWebhookWithManager sets up and registers the webhook with the manager.
func SetupAzureASOClusterWebhookWithManager(mgr ctrl.Manager) error {
	azureASOClusterWebhook := &azureASOClusterWebhook{}
	return ctrl.NewWebhookManagedBy(mgr).
		For(&AzureASOCluster{}).
		WithValidator(azureASOClusterWebhook).
		Complete()
}

// azureASOClusterWebhook implements a validating and defaulting webhook for AzureASOCluster.
type azureASOClusterWebhook struct {
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-azureasocluster,mutating=false,failurePolicy=fail,groups=infrastructure.cluster.x-k8s.io,resources=azureasoclusters,versions=v1alpha1,name=validation.azureasocluster.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (a *azureASOClusterWebhook) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList

	asoCluster, ok := obj.(*AzureASOCluster)
	if !ok {
		return nil, apierrors.NewBadRequest("expected an AzureASOCluster")
	}

	if asoCluster == nil {
		return nil, nil
	}

	allErrs = append(allErrs, validateAzureASOClusterSpec(field.NewPath("spec"), asoCluster.Spec)...)

	return nil, allErrs.ToAggregate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (a *azureASOClusterWebhook) ValidateUpdate(ctx context.Context, _, obj runtime.Object) (admission.Warnings, error) {
	return a.ValidateCreate(ctx, obj)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (*azureASOClusterWebhook) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func validateAzureASOClusterSpec(path *field.Path, spec AzureASOClusterSpec) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateAzureASOClusterTemplateResourceSpec(path, spec.AzureASOClusterTemplateResourceSpec)...)
	return allErrs
}

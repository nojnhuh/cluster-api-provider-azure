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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1alpha "sigs.k8s.io/cluster-api-provider-azure/api/v1alpha1"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
)

const (
	// AzureASOMachineKind is the kind for AzureASOMachine.
	AzureASOMachineKind = "AzureASOMachine"

	// AzureASOMachineFinalizer allows CAPZ to clean up Azure resources associated with an
	// AzureASOMachine before removing it from the apiserver.
	AzureASOMachineFinalizer = "azureasomachine.infrastructure.cluster.x-k8s.io"
)

// AzureASOMachineSpec defines the desired state of AzureASOMachine.
type AzureASOMachineSpec struct {
	AzureASOMachineTemplateResourceSpec `json:",inline"`

	// ProviderID must match the provider ID as seen on the node object corresponding to this machine. For
	// Kubernetes Nodes running on Azure, this value is set by the corresponding cloud provider component and
	// it has the format azure://<azure-id>.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	ProviderID string `json:"providerID,omitempty"`
}

// AzureASOMachineStatus defines the observed state of AzureASOMachine.
type AzureASOMachineStatus struct {
	// Initialization provides observations of the AzureASOMachine initialization process.
	// NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial Machine provisioning.
	// +optional
	Initialization AzureASOMachineInitializationStatus `json:"initialization,omitempty,omitzero"`

	//+optional
	Resources []infrav1alpha.ResourceStatus `json:"resources,omitempty"`
}

// AzureASOMachineInitializationStatus provides observations of the FooMachine initialization process.
// +kubebuilder:validation:MinProperties=1
type AzureASOMachineInitializationStatus struct {
	// Provisioned is true when the infrastructure provider reports that the Machine's infrastructure is fully provisioned.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial Machine provisioning.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=azureasomachines,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion

// AzureASOMachine implements the azureasomachine API.
type AzureASOMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AzureASOMachineSpec   `json:"spec,omitempty"`
	Status AzureASOMachineStatus `json:"status,omitempty"`
}

// SetResourceStatuses returns the status of resources.
func (a *AzureASOMachine) SetResourceStatuses(r []infrav1.ResourceStatus) {
	a.Status.Resources = make([]infrav1alpha.ResourceStatus, 0, len(r))
	for _, s := range r {
		a.Status.Resources = append(a.Status.Resources, infrav1alpha.ResourceStatus{
			Resource: infrav1alpha.StatusResource{
				Group:   s.Resource.Group,
				Version: s.Resource.Version,
				Kind:    s.Resource.Kind,
				Name:    s.Resource.Name,
			},
			Ready: s.Ready,
		})
	}
}

// +kubebuilder:object:root=true

// AzureASOMachineList contains a list of AzureASOMachines.
type AzureASOMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AzureASOMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AzureASOMachine{}, &AzureASOMachineList{})
}

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

package v2alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const AzureASOManagedMachinePoolFinalizer = "azureASOManagedMachinePool.infrastructure.cluster.x-k8s.io"

// AzureASOManagedMachinePoolSpec defines the desired state of AzureASOManagedMachinePool
type AzureASOManagedMachinePoolSpec struct {
	AzureASOManagedMachinePoolTemplateResourceSpec `json:",inline"`
}

// AzureASOManagedMachinePoolStatus defines the observed state of AzureASOManagedMachinePool
type AzureASOManagedMachinePoolStatus struct {
	// Ready represents whether or not the infrastructure is ready to be used. It fulfills Cluster API's
	// machine pool infrastructure provider contract.
	//+optional
	Ready bool `json:"ready"`

	// ObservedGeneration is the last generation that was successfully reconciled.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Resources represents the status of the resources defined in the spec.
	Resources []ResourceStatus `json:"resources,omitempty"`

	// Replicas is the current number of provisioned replicas.
	//+optional
	Replicas int32 `json:"replicas"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AzureASOManagedMachinePool is the Schema for the azureasomanagedmachinepools API
type AzureASOManagedMachinePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AzureASOManagedMachinePoolSpec   `json:"spec,omitempty"`
	Status AzureASOManagedMachinePoolStatus `json:"status,omitempty"`
}

func (a *AzureASOManagedMachinePool) GetResourceStatuses() []ResourceStatus {
	return a.Status.Resources
}

func (a *AzureASOManagedMachinePool) SetResourceStatuses(r []ResourceStatus) {
	a.Status.Resources = r
}

//+kubebuilder:object:root=true

// AzureASOManagedMachinePoolList contains a list of AzureASOManagedMachinePool
type AzureASOManagedMachinePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AzureASOManagedMachinePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AzureASOManagedMachinePool{}, &AzureASOManagedMachinePoolList{})
}

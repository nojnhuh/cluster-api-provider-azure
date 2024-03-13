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
	"k8s.io/apimachinery/pkg/runtime"
)

// AzureManagedMachinePoolTemplateSpec defines the desired state of AzureManagedMachinePoolTemplate
type AzureManagedMachinePoolTemplateSpec struct {
	Template AzureManagedControlPlaneResource `json:"template"`
}

type AzureManagedMachinePoolResource struct {
	Spec AzureManagedMachinePoolTemplateResourceSpec `json:"spec,omitempty"`
}

type AzureManagedMachinePoolTemplateResourceSpec struct {
	// ProviderIDList is the list of cloud provider IDs for the instances. It fulfills Cluster API's machine
	// pool infrastructure provider contract.
	ProviderIDList []string `json:"providerIDList,omitempty"`

	// Resources are embedded ASO resources to be managed by this resource.
	Resources []runtime.RawExtension `json:"resources,omitempty"`
}

//+kubebuilder:object:root=true

// AzureManagedMachinePoolTemplate is the Schema for the azuremanagedmachinepooltemplates API
type AzureManagedMachinePoolTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AzureManagedMachinePoolTemplateSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// AzureManagedMachinePoolTemplateList contains a list of AzureManagedMachinePoolTemplate
type AzureManagedMachinePoolTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AzureManagedMachinePoolTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AzureManagedMachinePoolTemplate{}, &AzureManagedMachinePoolTemplateList{})
}

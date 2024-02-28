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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ASOManagedMachinePoolTemplateSpec defines the desired state of ASOManagedMachinePoolTemplate
type ASOManagedMachinePoolTemplateSpec struct {
	Template ASOManagedControlPlaneResource `json:"template"`
}

type ASOManagedMachinePoolResource struct {
	Spec ASOManagedMachinePoolTemplateResourceSpec `json:"spec,omitempty"`
}

type ASOManagedMachinePoolTemplateResourceSpec struct {
	// ProviderIDList is the list of cloud provider IDs for the instances. It fulfills Cluster API's machine
	// pool infrastructure provider contract.
	ProviderIDList []string `json:"providerIDList,omitempty"`

	// Resources are embedded ASO resources to be managed by this resource.
	Resources []runtime.RawExtension `json:"resources,omitempty"`
}

//+kubebuilder:object:root=true

// ASOManagedMachinePoolTemplate is the Schema for the asomanagedmachinepooltemplates API
type ASOManagedMachinePoolTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ASOManagedMachinePoolTemplateSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// ASOManagedMachinePoolTemplateList contains a list of ASOManagedMachinePoolTemplate
type ASOManagedMachinePoolTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ASOManagedMachinePoolTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ASOManagedMachinePoolTemplate{}, &ASOManagedMachinePoolTemplateList{})
}

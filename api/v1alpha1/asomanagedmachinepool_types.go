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
)

// ASOManagedMachinePoolSpec defines the desired state of ASOManagedMachinePool
type ASOManagedMachinePoolSpec struct {
	// ProviderIDList is the list of cloud provider IDs for the instances.
	ProviderIDList string `json:"providerIDList,omitempty"`
}

// ASOManagedMachinePoolStatus defines the observed state of ASOManagedMachinePool
type ASOManagedMachinePoolStatus struct {
	// Ready represents whether or not the infrastructure is ready to be used. It fulfills Cluster API's
	// machine pool infrastructure provider contract.
	Ready bool `json:"ready"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ASOManagedMachinePool is the Schema for the asomanagedmachinepools API
type ASOManagedMachinePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ASOManagedMachinePoolSpec   `json:"spec,omitempty"`
	Status ASOManagedMachinePoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ASOManagedMachinePoolList contains a list of ASOManagedMachinePool
type ASOManagedMachinePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ASOManagedMachinePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ASOManagedMachinePool{}, &ASOManagedMachinePoolList{})
}

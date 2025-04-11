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
}

// AzureASOMachineStatus defines the observed state of AzureASOMachine.
type AzureASOMachineStatus struct {
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

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
	"k8s.io/apimachinery/pkg/runtime"
)

// AzureASOMachineTemplateSpec defines the desired state of AzureASOMachineTemplate.
type AzureASOMachineTemplateSpec struct {
	Template AzureASOMachineTemplateResource `json:"template"`
}

// AzureASOMachineTemplateResource defines the templated resource.
type AzureASOMachineTemplateResource struct {
	Spec AzureASOMachineTemplateResourceSpec `json:"spec,omitempty"`
}

// AzureASOMachineTemplateResourceSpec defines the desired state of the templated resource.
type AzureASOMachineTemplateResourceSpec struct {
	// Resources are embedded ASO resources to be managed by this resource.
	//
	// +optional
	// +kubebuilder:validation:MaxItems:=32
	Resources []runtime.RawExtension `json:"resources,omitempty"`

	// Patches are applied to Resources before they are created or updated.
	//
	// Data passed to JSONPatchValueFrom templates is a map consisting of the following keys:
	//
	// - selfV1alpha1: this AzureASOMachine
	// - machineV1beta2: the owning Machine resource
	// - clusterV1beta2: the associated Cluster resource
	// - bootstrapSecret: the Secret referred to by the owning Machine's spec.bootstrap.dataSecretName
	//
	// e.g. a template could be defined in YAML as
	//
	//     template: '{{ .selfV1alpha1.metadata.name }}'
	//
	// +optional
	// +kubebuilder:validation:MaxItems:=32
	Patches []ResourcesPatch `json:"patches,omitempty"`
}

//+kubebuilder:object:root=true

// AzureASOMachineTemplate is the Schema for the azureasomachinetemplates API.
type AzureASOMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AzureASOMachineTemplateSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// AzureASOMachineTemplateList contains a list of AzureASOMachineTemplate.
type AzureASOMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AzureASOMachineTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AzureASOMachineTemplate{}, &AzureASOMachineTemplateList{})
}

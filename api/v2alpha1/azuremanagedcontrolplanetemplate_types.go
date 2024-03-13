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

// AzureManagedControlPlaneTemplateSpec defines the desired state of AzureManagedControlPlane
type AzureManagedControlPlaneTemplateSpec struct {
	Template AzureManagedControlPlaneResource `json:"template"`
}

type AzureManagedControlPlaneResource struct {
	Spec AzureManagedControlPlaneTemplateResourceSpec `json:"spec,omitempty"`
}

type AzureManagedControlPlaneTemplateResourceSpec struct {
	// Version is the Kubernetes version of the control plane. It fulfills Cluster API's control plane
	// provider contract.
	Version string `json:"version,omitempty"`

	// Resources are embedded ASO resources to be managed by this resource.
	Resources []runtime.RawExtension `json:"resources,omitempty"`
}

//+kubebuilder:object:root=true

// AzureManagedControlPlaneTemplate is the Schema for the azuremanagedcontrolplanetemplates API
type AzureManagedControlPlaneTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AzureManagedControlPlaneTemplateSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// AzureManagedControlPlaneTemplateList contains a list of AzureManagedControlPlaneTemplate
type AzureManagedControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AzureManagedControlPlaneTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AzureManagedControlPlaneTemplate{}, &AzureManagedControlPlaneTemplateList{})
}

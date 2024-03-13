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

// ASOManagedClusterTemplateSpec defines the desired state of ASOManagedClusterTemplate
type ASOManagedClusterTemplateSpec struct {
	Template ASOClusterTemplateResource `json:"template"`
}

type ASOClusterTemplateResource struct {
	Spec ASOManagedClusterTemplateResourceSpec `json:"spec,omitempty"`
}

type ASOManagedClusterTemplateResourceSpec struct {
	// Resources are embedded ASO resources to be managed by this resource.
	Resources []runtime.RawExtension `json:"resources,omitempty"`
}

//+kubebuilder:object:root=true

// ASOManagedClusterTemplate is the Schema for the asomanagedclustertemplates API
type ASOManagedClusterTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ASOManagedClusterTemplateSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// ASOManagedClusterTemplateList contains a list of ASOManagedClusterTemplate
type ASOManagedClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ASOManagedClusterTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ASOManagedClusterTemplate{}, &ASOManagedClusterTemplateList{})
}

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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// AzureASOClusterTemplateSpec defines the desired state of AzureASOClusterTemplate.
type AzureASOClusterTemplateSpec struct {
	Template AzureASOClusterTemplateResource `json:"template"`
}

// AzureASOClusterTemplateResource defines the templated resource.
type AzureASOClusterTemplateResource struct {
	Spec AzureASOClusterTemplateResourceSpec `json:"spec,omitempty"`
}

// AzureASOClusterTemplateResourceSpec defines the desired state of the templated resource.
type AzureASOClusterTemplateResourceSpec struct {
	// Resources are embedded ASO resources to be managed by this resource.
	//
	// +optional
	// +kubebuilder:validation:MaxItems:=32
	Resources []runtime.RawExtension `json:"resources,omitempty"`

	// Patches are applied to Resources before they are created or updated.
	//
	// Data passed to JSONPatchValueFrom templates is a map consisting of the following keys:
	//
	// - self: this AzureASOCluster
	// - cluster: the owning Cluster resource
	//
	// e.g. a template could be defined in YAML as
	//
	//     template: '{{ .self.metadata.name }}'
	//
	// +optional
	// +kubebuilder:validation:MaxItems:=32
	Patches []ResourcesPatch `json:"patches,omitempty"`

	// ControlPlaneEndpointSource defines where the host and port for the ControlPlaneEndpoint can be found.
	// When set, CAPZ takes ownership of setting fields in ControlPlaneEndpoint, even if already set by the
	// user.
	//
	// +optional
	ControlPlaneEndpointSource *ControlPlaneEndpointSource `json:"controlPlaneEndpointSource,omitempty"`
}

// ResourcesPatch defines an ordered list of patches to apply to
// resources matching a set of selection criteria.
type ResourcesPatch struct {
	// Selectors declare conditions to match resources.
	// A resource is matched when at least one selector matches.
	//
	// +optional
	// +kubebuilder:validation:MaxItems:=32
	Selectors []ResourcesPatchSelector `json:"selectors,omitempty"`

	// JSONPatches defines the patches to be applied, in order.
	//
	// +optional
	// +kubebuilder:validation:MaxItems:=32
	JSONPatches []JSONPatch `json:"jsonPatches,omitempty"`
}

// ResourcesPatchSelector filters Resources to which patches should apply.
type ResourcesPatchSelector struct {
	// APIVersion, when defined, matches the apiVersion of the resource exactly.
	// Otherwise matches all APIVersions.
	//
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind, when defined, matches the kind of the resource exactly.
	// Otherwise matches all kinds.
	//
	// +optional
	Kind string `json:"kind,omitempty"`

	// Name, when defined, matches the metadata.name of the resource exactly.
	// Otherwise matches all names.
	//
	// +optional
	Name string `json:"name,omitempty"`
}

// JSONPatchOp is the `op` of a [RFC 6902] JSON patch.
//
// [RFC 6902]: https://tools.ietf.org/html/rfc6902
type JSONPatchOp string

const (
	// JSONPatchOpAdd is "add".
	JSONPatchOpAdd JSONPatchOp = "add"
	// JSONPatchOpRemove is "remove".
	JSONPatchOpRemove JSONPatchOp = "remove"
	// JSONPatchOpReplace is "replace".
	JSONPatchOpReplace JSONPatchOp = "replace"
	// JSONPatchOpMove is "move".
	JSONPatchOpMove JSONPatchOp = "move"
	// JSONPatchOpCopy is "copy".
	JSONPatchOpCopy JSONPatchOp = "copy"
	// JSONPatchOpTest is "test".
	JSONPatchOpTest JSONPatchOp = "test"
)

// JSONPatch represents a JSON Patch defined by [RFC 6902].
//
// [RFC 6902]: https://tools.ietf.org/html/rfc6902
type JSONPatch struct {
	// +required
	// +kubebuilder:validation:Enum=add;remove;replace;move;copy;test
	Op JSONPatchOp `json:"op"`

	// +required
	// +kubebuilder:validation:MaxLength:=256
	Path string `json:"path"`

	// +optional
	// +kubebuilder:validation:MaxLength:=256
	From string `json:"from,omitempty"`

	// Value is a literal value used per RFC 6902.
	// Only one of Value and ValueFrom may be set.
	//
	// +optional
	Value *apiextensionsv1.JSON `json:"value,omitempty"`

	// ValueFrom defines the value of the patch.
	// Only one of Value and ValueFrom may be set.
	//
	// +optional
	ValueFrom *JSONPatchValueFrom `json:"valueFrom,omitempty"`
}

// JSONPatchValueFrom defines a non-literal value to be used in a [JSONPatch].
type JSONPatchValueFrom struct {
	// Template is the Go template to be used to calculate the value.
	// The template must evaluate to a valid YAML or JSON value.
	//
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Template *string `json:"template,omitempty"`
}

// ControlPlaneEndpointSource defines where the host and port for the ControlPlaneEndpoint can be found.
type ControlPlaneEndpointSource struct {
	// Host refers to a value to use for the ControlPlaneEndpoint's host.
	//
	// +optional
	Host *StringSource `json:"host,omitempty"`

	// Port refers to a value to use for the ControlPlaneEndpoint's port.
	//
	// +optional
	Port *StringSource `json:"port,omitempty"`
}

// StringSource defines a location containing a string.
type StringSource struct {
	// +optional
	ConfigMap *ConfigMapReference `json:"configMap,omitempty"`
}

// ConfigMapReference refers to a particular value in a ConfigMap.
type ConfigMapReference struct {
	// Name is the metadata.name of the ConfigMap.
	//
	// +required
	Name StringValue `json:"name"`

	// Key is the key of the ConfigMap's data.
	//
	// +required
	Key StringValue `json:"key"`
}

// StringValue represents a string.
type StringValue struct {
	// Value is a literal string.
	// +optional
	Value *string `json:"value,omitempty"`

	// Template is a Go text/template that evaluates to a string.
	//
	// Data passed to templates is a map consisting of the following keys:
	//
	// - self: this CAPZ resource
	//
	// e.g. a template could be defined in YAML as
	//
	//     template: '{{ .self.metadata.name }}'
	//
	// +optional
	Template *string `json:"template,omitempty"`
}

//+kubebuilder:object:root=true

// AzureASOClusterTemplate is the Schema for the azureasoclustertemplates API.
type AzureASOClusterTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AzureASOClusterTemplateSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// AzureASOClusterTemplateList contains a list of AzureASOClusterTemplate.
type AzureASOClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AzureASOClusterTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AzureASOClusterTemplate{}, &AzureASOClusterTemplateList{})
}

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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// AzureASOManagedControlPlaneSpec defines the desired state of AzureASOManagedControlPlane
type AzureASOManagedControlPlaneSpec struct {
	AzureASOManagedControlPlaneTemplateResourceSpec `json:",inline"`
}

// AzureASOManagedControlPlaneStatus defines the observed state of AzureASOManagedControlPlane
type AzureASOManagedControlPlaneStatus struct {
	// Initialized represents whether or not the API server has been provisioned. It fulfills Cluster API's
	// control plane provider contract.
	//+optional
	Initialized bool `json:"initialized"`

	// Ready represents whether or not the API server is ready to receive requests. It fulfills Cluster API's
	// control plane provider contract.
	//+optional
	Ready bool `json:"ready"`

	// Version is the observed Kubernetes version of the control plane. It fulfills Cluster API's control
	// plane provider contract.
	Version string `json:"version,omitempty"`

	// ExternalManagedControlPlane is always set to true since control plane components for AKS do not exist
	// in Nodes. It fulfills Cluster API's control plane provider contract.
	//+optional
	ExternalManagedControlPlane bool `json:"externalManagedControlPlane"`

	// ControlPlaneEndpoint represents the endpoint for the cluster's API server.
	//+optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint"`

	// ObservedGeneration is the last metadata.generation of the object that has been reconciled.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Resources represents the status of the resources defined in the spec.
	Resources []ResourceStatus `json:"resources,omitempty"`

	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AzureASOManagedControlPlane is the Schema for the azureasomanagedcontrolplanes API
type AzureASOManagedControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AzureASOManagedControlPlaneSpec   `json:"spec,omitempty"`
	Status AzureASOManagedControlPlaneStatus `json:"status,omitempty"`
}

func (a *AzureASOManagedControlPlane) GetResourceStatuses() []ResourceStatus {
	return a.Status.Resources
}

func (a *AzureASOManagedControlPlane) SetResourceStatuses(r []ResourceStatus) {
	a.Status.Resources = r
}

func (a *AzureASOManagedControlPlane) GetConditions() clusterv1.Conditions {
	return a.Status.Conditions
}

func (a *AzureASOManagedControlPlane) SetConditions(conds clusterv1.Conditions) {
	a.Status.Conditions = conds
}

//+kubebuilder:object:root=true

// AzureASOManagedControlPlaneList contains a list of AzureASOManagedControlPlane
type AzureASOManagedControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AzureASOManagedControlPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AzureASOManagedControlPlane{}, &AzureASOManagedControlPlaneList{})
}

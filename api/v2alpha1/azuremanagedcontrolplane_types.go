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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// AzureManagedControlPlaneSpec defines the desired state of AzureManagedControlPlane
type AzureManagedControlPlaneSpec struct {
	AzureManagedControlPlaneTemplateResourceSpec `json:",inline"`
}

// AzureManagedControlPlaneStatus defines the observed state of AzureManagedControlPlane
type AzureManagedControlPlaneStatus struct {
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
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AzureManagedControlPlane is the Schema for the azuremanagedcontrolplanes API
type AzureManagedControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AzureManagedControlPlaneSpec   `json:"spec,omitempty"`
	Status AzureManagedControlPlaneStatus `json:"status,omitempty"`
}

func (a *AzureManagedControlPlane) GetResourceStatuses() []ResourceStatus {
	return a.Status.Resources
}

func (a *AzureManagedControlPlane) SetResourceStatuses(r []ResourceStatus) {
	a.Status.Resources = r
}

//+kubebuilder:object:root=true

// AzureManagedControlPlaneList contains a list of AzureManagedControlPlane
type AzureManagedControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AzureManagedControlPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AzureManagedControlPlane{}, &AzureManagedControlPlaneList{})
}

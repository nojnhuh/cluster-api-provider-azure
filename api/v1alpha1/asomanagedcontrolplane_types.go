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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// ASOManagedControlPlaneSpec defines the desired state of ASOManagedControlPlane
type ASOManagedControlPlaneSpec struct {
	// Version is the Kubernetes version of the control plane. It fulfills Cluster API's control plane
	// provider contract.
	Version string `json:"version,omitempty"`
}

// ASOManagedControlPlaneStatus defines the observed state of ASOManagedControlPlane
type ASOManagedControlPlaneStatus struct {
	// Initialized represents whether or not the API server has been provisioned. It fulfills Cluster API's
	// control plane provider contract.
	Initialized bool `json:"initialized"`

	// Ready represents whether or not the API server is ready to receive requests. It fulfills Cluster API's
	// control plane provider contract.
	Ready bool `json:"ready"`

	// Version is the observed Kubernetes version of the control plane. It fulfills Cluster API's control
	// plane provider contract.
	Version string `json:"version,omitempty"`

	// ExternalManagedControlPlane is always set to true since control plane components for AKS do not exist
	// in Nodes. It fulfills Cluster API's control plane provider contract.
	ExternalManagedControlPlane bool `json:"externalManagedControlPlane"`

	// ControlPlaneEndpoint represents the endpoint for the cluster's API server.
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ASOManagedControlPlane is the Schema for the asomanagedcontrolplanes API
type ASOManagedControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ASOManagedControlPlaneSpec   `json:"spec,omitempty"`
	Status ASOManagedControlPlaneStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ASOManagedControlPlaneList contains a list of ASOManagedControlPlane
type ASOManagedControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ASOManagedControlPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ASOManagedControlPlane{}, &ASOManagedControlPlaneList{})
}

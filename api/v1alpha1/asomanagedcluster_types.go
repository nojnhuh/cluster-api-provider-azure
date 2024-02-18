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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// ASOManagedClusterSpec defines the desired state of ASOManagedCluster
type ASOManagedClusterSpec struct {
	// ControlPlaneEndpoint is the location of the API server within the control plane. It fulfills Cluster
	// API's cluster infrastructure provider contract.
	//+optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint"`

	// Resources are embedded ASO resources to be managed by this resource.
	Resources []runtime.RawExtension `json:"resources,omitempty"`
}

// ASOManagedClusterStatus defines the observed state of ASOManagedCluster
type ASOManagedClusterStatus struct {
	// Ready represents whether or not the cluster has been provisioned and is ready. It fulfills Cluster
	// API's cluster infrastructure provider contract.
	Ready bool `json:"ready"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ASOManagedCluster is the Schema for the asomanagedclusters API
type ASOManagedCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ASOManagedClusterSpec   `json:"spec,omitempty"`
	Status ASOManagedClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ASOManagedClusterList contains a list of ASOManagedCluster
type ASOManagedClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ASOManagedCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ASOManagedCluster{}, &ASOManagedClusterList{})
}

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
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrav1alpha "sigs.k8s.io/cluster-api-provider-azure/api/v1alpha1"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
)

const (
	// AzureASOClusterKind is the kind for AzureASOCluster.
	AzureASOClusterKind = "AzureASOCluster"

	// AzureASOClusterFinalizer is the finalizer added to AzureASOClusters.
	AzureASOClusterFinalizer = "azureasocluster.infrastructure.cluster.x-k8s.io"
)

// AzureASOClusterSpec defines the desired state of AzureASOCluster.
type AzureASOClusterSpec struct {
	AzureASOClusterTemplateResourceSpec `json:",inline"`

	// ControlPlaneEndpoint is the location of the API server within the control plane. CAPZ manages this field
	// and it should not be set by the user. It fulfills Cluster API's cluster infrastructure provider contract.
	// Because this field is programmatically set by CAPZ after resource creation, we define it as +optional
	// in the API schema to permit resource admission.
	//
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint"`
}

// AzureASOClusterStatus defines the observed state of AzureASOCluster.
type AzureASOClusterStatus struct {
	// Initialization provides observations of the AzureASOCluster initialization process.
	// NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial Cluster provisioning.
	// +optional
	Initialization AzureASOClusterInitializationStatus `json:"initialization,omitempty,omitzero"`

	// +optional
	Resources []infrav1alpha.ResourceStatus `json:"resources,omitempty"`
}

// AzureASOClusterInitializationStatus provides observations of the AzureASOCluster initialization process.
// +kubebuilder:validation:MinProperties=1
type AzureASOClusterInitializationStatus struct {
	// Provisioned is true when the infrastructure provider reports that the Cluster's infrastructure is fully provisioned.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial Cluster provisioning.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=azureasoclusters,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion

// AzureASOCluster implements the azureasocluster API.
type AzureASOCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AzureASOClusterSpec   `json:"spec,omitempty"`
	Status AzureASOClusterStatus `json:"status,omitempty"`
}

// SetResourceStatuses returns the status of resources.
func (a *AzureASOCluster) SetResourceStatuses(r []infrav1.ResourceStatus) {
	a.Status.Resources = make([]infrav1alpha.ResourceStatus, 0, len(r))
	for _, s := range r {
		a.Status.Resources = append(a.Status.Resources, infrav1alpha.ResourceStatus{
			Resource: infrav1alpha.StatusResource{
				Group:   s.Resource.Group,
				Version: s.Resource.Version,
				Kind:    s.Resource.Kind,
				Name:    s.Resource.Name,
			},
			Ready: s.Ready,
		})
	}
}

// +kubebuilder:object:root=true

// AzureASOClusterList contains a list of AzureASOClusters.
type AzureASOClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AzureASOCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AzureASOCluster{}, &AzureASOClusterList{})
}

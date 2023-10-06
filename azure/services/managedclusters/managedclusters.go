/*
Copyright 2020 The Kubernetes Authors.

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

package managedclusters

import (
	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20230201"
	"k8s.io/utils/ptr"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/aso"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const serviceName = "managedcluster"

const kubeletIdentityKey = "kubeletidentity"

// ManagedClusterScope defines the scope interface for a managed cluster.
type ManagedClusterScope interface {
	aso.Scope
	ManagedClusterSpec() azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedCluster]
	SetControlPlaneEndpoint(clusterv1.APIEndpoint)
	SetOIDCIssuerProfileStatus(*infrav1.OIDCIssuerProfileStatus)
}

// Service provides operations on azure resources.
type Service struct {
	Scope ManagedClusterScope
	*aso.Service[*asocontainerservicev1.ManagedCluster, ManagedClusterScope]
}

// New creates a new service.
func New(scope ManagedClusterScope) *Service {
	svc := aso.NewService[*asocontainerservicev1.ManagedCluster](serviceName, scope)
	svc.Specs = []azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedCluster]{scope.ManagedClusterSpec()}
	svc.ConditionType = infrav1.ManagedClusterRunningCondition
	svc.PostCreateOrUpdateResourceHook = postCreateOrUpdateResourceHook
	return &Service{
		Scope:   scope,
		Service: svc,
	}
}

// Name returns the service name.
func (s *Service) Name() string {
	return serviceName
}

func postCreateOrUpdateResourceHook(scope ManagedClusterScope, managedCluster *asocontainerservicev1.ManagedCluster, err error) {
	if err != nil {
		return
	}

	// Update control plane endpoint.
	endpoint := clusterv1.APIEndpoint{
		Host: ptr.Deref(managedCluster.Status.Fqdn, ""),
		Port: 443,
	}
	scope.SetControlPlaneEndpoint(endpoint)

	scope.SetOIDCIssuerProfileStatus(nil)
	if managedCluster.Status.OidcIssuerProfile != nil && managedCluster.Status.OidcIssuerProfile.IssuerURL != nil {
		scope.SetOIDCIssuerProfileStatus(&infrav1.OIDCIssuerProfileStatus{
			IssuerURL: managedCluster.Status.OidcIssuerProfile.IssuerURL,
		})
	}
}

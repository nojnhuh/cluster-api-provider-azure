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
	"errors"
	"testing"

	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20230201"
	"go.uber.org/mock/gomock"
	"k8s.io/utils/ptr"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/managedclusters/mock_managedclusters"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestPostCreateOrUpdateResourceHook(t *testing.T) {
	t.Run("error creating or updating", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		scope := mock_managedclusters.NewMockManagedClusterScope(mockCtrl)

		postCreateOrUpdateResourceHook(scope, nil, errors.New("an error"))
	})

	t.Run("successful create or update", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		scope := mock_managedclusters.NewMockManagedClusterScope(mockCtrl)

		scope.EXPECT().SetControlPlaneEndpoint(clusterv1.APIEndpoint{
			Host: "fdqn",
			Port: 443,
		})
		scope.EXPECT().SetKubeletIdentity("kubelet-id")
		scope.EXPECT().SetOIDCIssuerProfileStatus(gomock.Nil())
		scope.EXPECT().SetOIDCIssuerProfileStatus(&infrav1.OIDCIssuerProfileStatus{
			IssuerURL: ptr.To("oidc"),
		})

		managedCluster := &asocontainerservicev1.ManagedCluster{
			Status: asocontainerservicev1.ManagedCluster_STATUS{
				Fqdn: ptr.To("fdqn"),
				IdentityProfile: map[string]asocontainerservicev1.UserAssignedIdentity_STATUS{
					kubeletIdentityKey: {
						ResourceId: ptr.To("kubelet-id"),
					},
				},
				OidcIssuerProfile: &asocontainerservicev1.ManagedClusterOIDCIssuerProfile_STATUS{
					IssuerURL: ptr.To("oidc"),
				},
			},
		}

		postCreateOrUpdateResourceHook(scope, managedCluster, nil)
	})
}

/*
Copyright 2022 The Kubernetes Authors.

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
	"context"
	"encoding/base64"
	"testing"

	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20230201"
	"github.com/Azure/azure-service-operator/v2/pkg/genruntime"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"k8s.io/utils/ptr"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/agentpools"
	gomockinternal "sigs.k8s.io/cluster-api-provider-azure/internal/test/matchers/gomock"
)

func TestParameters(t *testing.T) {
	testcases := []struct {
		name          string
		spec          *ManagedClusterSpec
		existing      *asocontainerservicev1.ManagedCluster
		expectedError string
		expect        func(g *WithT, result *asocontainerservicev1.ManagedCluster)
	}{
		{
			name:     "managedcluster does not exist",
			existing: nil,
			spec: &ManagedClusterSpec{
				Name:              "test-managedcluster",
				ResourceGroup:     "test-rg",
				NodeResourceGroup: "test-node-rg",
				ClusterName:       "test-cluster",
				Location:          "test-location",
				Version:           "v1.22.0",
				LoadBalancerSKU:   "standard",
				SSHPublicKey:      base64.StdEncoding.EncodeToString([]byte("test-ssh-key")),
				NetworkPluginMode: ptr.To(infrav1.NetworkPluginModeOverlay),
				OIDCIssuerProfile: &OIDCIssuerProfile{
					Enabled: ptr.To(true),
				},
				GetAllAgentPools: func() ([]azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedClustersAgentPool], error) {
					return []azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedClustersAgentPool]{
						&agentpools.AgentPoolSpec{
							AzureName:     "test-agentpool-0",
							Mode:          string(infrav1.NodePoolModeSystem),
							ResourceGroup: "test-rg",
							Replicas:      2,
							AdditionalTags: map[string]string{
								"test-tag": "test-value",
							},
						},
						&agentpools.AgentPoolSpec{
							AzureName:         "test-agentpool-1",
							Mode:              string(infrav1.NodePoolModeUser),
							ResourceGroup:     "test-rg",
							Replicas:          4,
							Cluster:           "test-managedcluster",
							SKU:               "test_SKU",
							Version:           ptr.To("v1.22.0"),
							VnetSubnetID:      "fake/subnet/id",
							MaxPods:           ptr.To(32),
							AvailabilityZones: []string{"1", "2"},
							AdditionalTags: map[string]string{
								"test-tag": "test-value",
							},
						},
					}, nil
				},
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				sampleCluster := getSampleManagedCluster()
				g.Expect(gomockinternal.DiffEq(result).Matches(sampleCluster)).To(BeTrue(), cmp.Diff(result, getSampleManagedCluster()))
			},
		},
		{
			name:     "managedcluster exists, no update needed",
			existing: getExistingCluster(),
			spec: &ManagedClusterSpec{
				Name:            "test-managedcluster",
				ResourceGroup:   "test-rg",
				Location:        "test-location",
				Version:         "v1.22.0",
				LoadBalancerSKU: "standard",
				OIDCIssuerProfile: &OIDCIssuerProfile{
					Enabled: ptr.To(true),
				},
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				g.Expect(result).To(Equal(getExistingCluster()))
			},
		},
		{
			name:     "managedcluster exists and an update is needed",
			existing: getExistingCluster(),
			spec: &ManagedClusterSpec{
				Name:            "test-managedcluster",
				ResourceGroup:   "test-rg",
				Location:        "test-location",
				Version:         "v1.22.99",
				LoadBalancerSKU: "standard",
				Identity: &infrav1.Identity{
					Type:                           infrav1.ManagedControlPlaneIdentityTypeUserAssigned,
					UserAssignedIdentityResourceID: "/resource/ID",
				},
				KubeletUserAssignedIdentity: "/resource/ID",
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				g.Expect(result.Spec.KubernetesVersion).To(Equal(ptr.To("v1.22.99")))
				g.Expect(result.Spec.Identity.Type).To(Equal(ptr.To(asocontainerservicev1.ManagedClusterIdentity_Type_UserAssigned)))
				g.Expect(result.Spec.Identity.UserAssignedIdentities).To(Equal([]asocontainerservicev1.UserAssignedIdentityDetails{{Reference: genruntime.ResourceReference{ARMID: "/resource/ID"}}}))
				g.Expect(result.Spec.IdentityProfile).To(Equal(map[string]asocontainerservicev1.UserAssignedIdentity{kubeletIdentityKey: {ResourceReference: &genruntime.ResourceReference{ARMID: "/resource/ID"}}}))
			},
		},
		{
			name:     "set Linux profile if SSH key is set",
			existing: nil,
			spec: &ManagedClusterSpec{
				Name:            "test-managedcluster",
				ResourceGroup:   "test-rg",
				Location:        "test-location",
				Version:         "v1.22.0",
				LoadBalancerSKU: "standard",
				OIDCIssuerProfile: &OIDCIssuerProfile{
					Enabled: ptr.To(true),
				},
				SSHPublicKey: base64.StdEncoding.EncodeToString([]byte("test-ssh-key")),
				GetAllAgentPools: func() ([]azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedClustersAgentPool], error) {
					return []azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedClustersAgentPool]{
						&agentpools.AgentPoolSpec{
							Name:          "test-agentpool-0",
							Mode:          string(infrav1.NodePoolModeSystem),
							ResourceGroup: "test-rg",
							Replicas:      2,
							AdditionalTags: map[string]string{
								"test-tag": "test-value",
							},
						},
					}, nil
				},
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				g.Expect(result.Spec.LinuxProfile).To(Not(BeNil()))
				g.Expect(*(result.Spec.LinuxProfile.Ssh.PublicKeys)[0].KeyData).To(Equal("test-ssh-key"))
			},
		},
		{
			name:     "set HTTPProxyConfig if set",
			existing: nil,
			spec: &ManagedClusterSpec{
				Name:            "test-managedcluster",
				ResourceGroup:   "test-rg",
				Location:        "test-location",
				Version:         "v1.22.0",
				LoadBalancerSKU: "standard",
				OIDCIssuerProfile: &OIDCIssuerProfile{
					Enabled: ptr.To(true),
				},
				HTTPProxyConfig: &HTTPProxyConfig{
					HTTPProxy:  ptr.To("http://proxy.com"),
					HTTPSProxy: ptr.To("https://proxy.com"),
				},
				GetAllAgentPools: func() ([]azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedClustersAgentPool], error) {
					return []azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedClustersAgentPool]{}, nil
				},
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				g.Expect(result.Spec.HttpProxyConfig).To(Not(BeNil()))
				g.Expect(*result.Spec.HttpProxyConfig.HttpProxy).To(Equal("http://proxy.com"))
			},
		},
		{
			name:     "set HTTPProxyConfig if set with no proxy list",
			existing: nil,
			spec: &ManagedClusterSpec{
				Name:            "test-managedcluster",
				ResourceGroup:   "test-rg",
				Location:        "test-location",
				Version:         "v1.22.0",
				LoadBalancerSKU: "standard",
				OIDCIssuerProfile: &OIDCIssuerProfile{
					Enabled: ptr.To(true),
				},
				HTTPProxyConfig: &HTTPProxyConfig{
					NoProxy: []string{"noproxy1", "noproxy2"},
				},
				GetAllAgentPools: func() ([]azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedClustersAgentPool], error) {
					return []azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedClustersAgentPool]{}, nil
				},
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				g.Expect(result.Spec.HttpProxyConfig).To(Not(BeNil()))
				g.Expect((result.Spec.HttpProxyConfig.NoProxy)).To(Equal([]string{"noproxy1", "noproxy2"}))
			},
		},
		{
			name:     "skip Linux profile if SSH key is not set",
			existing: nil,
			spec: &ManagedClusterSpec{
				Name:            "test-managedcluster",
				ResourceGroup:   "test-rg",
				Location:        "test-location",
				Version:         "v1.22.0",
				LoadBalancerSKU: "standard",
				OIDCIssuerProfile: &OIDCIssuerProfile{
					Enabled: ptr.To(true),
				},
				SSHPublicKey: "",
				GetAllAgentPools: func() ([]azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedClustersAgentPool], error) {
					return []azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedClustersAgentPool]{
						&agentpools.AgentPoolSpec{
							Name:          "test-agentpool-0",
							Mode:          string(infrav1.NodePoolModeSystem),
							ResourceGroup: "test-rg",
							Replicas:      2,
							AdditionalTags: map[string]string{
								"test-tag": "test-value",
							},
						},
					}, nil
				},
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				g.Expect(result.Spec.LinuxProfile).To(BeNil())
			},
		},
		{
			name:     "no update needed if both clusters have no authorized IP ranges",
			existing: getExistingClusterWithAPIServerAccessProfile(),
			spec: &ManagedClusterSpec{
				Name:            "test-managedcluster",
				ResourceGroup:   "test-rg",
				Location:        "test-location",
				Version:         "v1.22.0",
				LoadBalancerSKU: "standard",
				OIDCIssuerProfile: &OIDCIssuerProfile{
					Enabled: ptr.To(true),
				},
				APIServerAccessProfile: &APIServerAccessProfile{
					EnablePrivateCluster: ptr.To(false),
				},
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				g.Expect(result).To(Equal(getExistingClusterWithAPIServerAccessProfile()))
			},
		},
		{
			name:     "update authorized IP ranges with empty struct if spec does not have authorized IP ranges but existing cluster has authorized IP ranges",
			existing: getExistingClusterWithAuthorizedIPRanges(),
			spec: &ManagedClusterSpec{
				Name:            "test-managedcluster",
				ResourceGroup:   "test-rg",
				Location:        "test-location",
				Version:         "v1.22.0",
				LoadBalancerSKU: "standard",
				OIDCIssuerProfile: &OIDCIssuerProfile{
					Enabled: ptr.To(true),
				},
				APIServerAccessProfile: &APIServerAccessProfile{
					AuthorizedIPRanges: []string{},
				},
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				g.Expect(result.Spec.ApiServerAccessProfile).To(Not(BeNil()))
				g.Expect(result.Spec.ApiServerAccessProfile.AuthorizedIPRanges).To(Equal([]string{}))
			},
		},
		{
			name:     "update authorized IP ranges with authorized IPs spec has authorized IP ranges but existing cluster does not have authorized IP ranges",
			existing: getExistingCluster(),
			spec: &ManagedClusterSpec{
				Name:            "test-managedcluster",
				ResourceGroup:   "test-rg",
				Location:        "test-location",
				Version:         "v1.22.0",
				LoadBalancerSKU: "standard",
				OIDCIssuerProfile: &OIDCIssuerProfile{
					Enabled: ptr.To(true),
				},
				APIServerAccessProfile: &APIServerAccessProfile{
					AuthorizedIPRanges: []string{"192.168.0.1/32, 192.168.0.2/32, 192.168.0.3/32"},
				},
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				g.Expect(result.Spec.ApiServerAccessProfile).To(Not(BeNil()))
				g.Expect(result.Spec.ApiServerAccessProfile.AuthorizedIPRanges).To(Equal([]string{"192.168.0.1/32, 192.168.0.2/32, 192.168.0.3/32"}))
			},
		},
		{
			name:     "no update needed when authorized IP ranges when both clusters have the same authorized IP ranges",
			existing: getExistingClusterWithAuthorizedIPRanges(),
			spec: &ManagedClusterSpec{
				Name:            "test-managedcluster",
				ResourceGroup:   "test-rg",
				Location:        "test-location",
				Version:         "v1.22.0",
				LoadBalancerSKU: "standard",
				OIDCIssuerProfile: &OIDCIssuerProfile{
					Enabled: ptr.To(true),
				},
				APIServerAccessProfile: &APIServerAccessProfile{
					AuthorizedIPRanges: []string{"192.168.0.1/32, 192.168.0.2/32, 192.168.0.3/32"},
				},
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				g.Expect(result).To(Equal(getExistingClusterWithAuthorizedIPRanges()))
			},
		},
		{
			name:     "managedcluster exists with UserAssigned identity, no update needed",
			existing: getExistingClusterWithUserAssignedIdentity(),
			spec: &ManagedClusterSpec{
				Name:            "test-managedcluster",
				ResourceGroup:   "test-rg",
				Location:        "test-location",
				Version:         "v1.22.0",
				LoadBalancerSKU: "standard",
				OIDCIssuerProfile: &OIDCIssuerProfile{
					Enabled: ptr.To(true),
				},
				Identity: &infrav1.Identity{
					Type:                           infrav1.ManagedControlPlaneIdentityTypeUserAssigned,
					UserAssignedIdentityResourceID: "some id",
				},
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				g.Expect(result).To(Equal(getExistingClusterWithUserAssignedIdentity()))
			},
		},
		{
			name: "setting networkPluginMode from nil to \"overlay\" will update",
			existing: func() *asocontainerservicev1.ManagedCluster {
				c := getExistingCluster()
				c.Spec.NetworkProfile.NetworkPluginMode = nil
				return c
			}(),
			spec: &ManagedClusterSpec{
				Name:            "test-managedcluster",
				ResourceGroup:   "test-rg",
				Location:        "test-location",
				Version:         "v1.22.0",
				LoadBalancerSKU: "standard",
				OIDCIssuerProfile: &OIDCIssuerProfile{
					Enabled: ptr.To(true),
				},
				NetworkPluginMode: ptr.To(infrav1.NetworkPluginModeOverlay),
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				g.Expect(result.Spec.NetworkProfile.NetworkPluginMode).NotTo(BeNil())
				g.Expect(*result.Spec.NetworkProfile.NetworkPluginMode).To(Equal(asocontainerservicev1.ContainerServiceNetworkProfile_NetworkPluginMode_Overlay))
			},
		},
		{
			name:     "setting networkPluginMode from \"overlay\" to nil doesn't require update",
			existing: getExistingCluster(),
			spec: &ManagedClusterSpec{
				Name:            "test-managedcluster",
				ResourceGroup:   "test-rg",
				Location:        "test-location",
				Version:         "v1.22.0",
				LoadBalancerSKU: "standard",
				OIDCIssuerProfile: &OIDCIssuerProfile{
					Enabled: ptr.To(true),
				},
				NetworkPluginMode: nil,
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				g.Expect(result).To(Equal(getExistingCluster()))
			},
		},
		{
			name:     "update needed when oidc issuer profile enabled changes",
			existing: getExistingCluster(),
			spec: &ManagedClusterSpec{
				Name:            "test-managedcluster",
				ResourceGroup:   "test-rg",
				Location:        "test-location",
				Version:         "v1.22.0",
				LoadBalancerSKU: "standard",
				OIDCIssuerProfile: &OIDCIssuerProfile{
					Enabled: ptr.To(false),
				},
			},
			expect: func(g *WithT, result *asocontainerservicev1.ManagedCluster) {
				g.Expect(*result.Spec.OidcIssuerProfile.Enabled).To(BeFalse())
			},
		},
	}
	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			format.MaxLength = 10000
			g := NewWithT(t)
			t.Parallel()

			result, err := tc.spec.Parameters(context.TODO(), tc.existing)
			if tc.expectedError != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(MatchError(tc.expectedError))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			tc.expect(g, result)
		})
	}
}

func TestGetIdentity(t *testing.T) {
	testcases := []struct {
		name         string
		identity     *infrav1.Identity
		expectedType *asocontainerservicev1.ManagedClusterIdentity_Type
	}{
		{
			name:     "default",
			identity: &infrav1.Identity{},
		},
		{
			name: "user-assigned identity",
			identity: &infrav1.Identity{
				Type:                           infrav1.ManagedControlPlaneIdentityTypeUserAssigned,
				UserAssignedIdentityResourceID: "/subscriptions/fae7cc14-bfba-4471-9435-f945b42a16dd/resourcegroups/my-identities/providers/Microsoft.ManagedIdentity/userAssignedIdentities/my-cluster-user-identity",
			},
			expectedType: ptr.To(asocontainerservicev1.ManagedClusterIdentity_Type_UserAssigned),
		},
		{
			name: "system-assigned identity",
			identity: &infrav1.Identity{
				Type: infrav1.ManagedControlPlaneIdentityTypeSystemAssigned,
			},
			expectedType: ptr.To(asocontainerservicev1.ManagedClusterIdentity_Type_SystemAssigned),
		},
	}
	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			t.Parallel()

			result, err := getIdentity(tc.identity)
			g.Expect(err).To(BeNil())
			if tc.identity.Type != "" {
				g.Expect(result.Type).To(Equal(tc.expectedType))
				if tc.identity.Type == infrav1.ManagedControlPlaneIdentityTypeUserAssigned {
					g.Expect(result.UserAssignedIdentities).To(Not(BeEmpty()))
					g.Expect(result.UserAssignedIdentities[0]).To(Equal(asocontainerservicev1.UserAssignedIdentityDetails{
						Reference: genruntime.ResourceReference{
							ARMID: tc.identity.UserAssignedIdentityResourceID,
						},
					}))
				} else {
					g.Expect(result.UserAssignedIdentities).To(BeEmpty())
				}
			} else {
				g.Expect(result).To(BeNil())
			}
		})
	}
}

func getExistingClusterWithAPIServerAccessProfile() *asocontainerservicev1.ManagedCluster {
	mc := getExistingCluster()
	mc.Spec.ApiServerAccessProfile = &asocontainerservicev1.ManagedClusterAPIServerAccessProfile{
		EnablePrivateCluster: ptr.To(false),
	}
	return mc
}

func getExistingCluster() *asocontainerservicev1.ManagedCluster {
	mc := getSampleManagedCluster()
	mc.Status.Id = ptr.To("test-id")

	mc.Spec.LinuxProfile = nil
	mc.Spec.NetworkProfile.NetworkPluginMode = nil
	mc.Spec.NodeResourceGroup = ptr.To("")
	mc.Spec.OperatorSpec.Secrets.AdminCredentials.Name = "-kubeconfig"
	mc.Spec.Tags[infrav1.ClusterTagKey("")] = mc.Spec.Tags[infrav1.ClusterTagKey("test-cluster")]
	delete(mc.Spec.Tags, infrav1.ClusterTagKey("test-cluster"))

	mc.Spec.AgentPoolProfiles = nil
	// only nil vs. non-nil matters here
	mc.Status.AgentPoolProfiles = []asocontainerservicev1.ManagedClusterAgentPoolProfile_STATUS{}

	return mc
}

func getExistingClusterWithUserAssignedIdentity() *asocontainerservicev1.ManagedCluster {
	mc := getExistingCluster()
	mc.Status.Id = ptr.To("test-id")
	mc.Spec.Identity = &asocontainerservicev1.ManagedClusterIdentity{
		Type: ptr.To(asocontainerservicev1.ManagedClusterIdentity_Type_UserAssigned),
		UserAssignedIdentities: []asocontainerservicev1.UserAssignedIdentityDetails{
			{
				Reference: genruntime.ResourceReference{
					ARMID: "some id",
				},
			},
		},
	}
	return mc
}

func getSampleManagedCluster() *asocontainerservicev1.ManagedCluster {
	return &asocontainerservicev1.ManagedCluster{
		Spec: asocontainerservicev1.ManagedCluster_Spec{
			AzureName:         "test-managedcluster",
			Owner:             &genruntime.KnownResourceReference{Name: "test-rg"},
			KubernetesVersion: ptr.To("v1.22.0"),
			AgentPoolProfiles: []asocontainerservicev1.ManagedClusterAgentPoolProfile{
				{
					Name:         ptr.To("test-agentpool-0"),
					Mode:         ptr.To(asocontainerservicev1.AgentPoolMode(infrav1.NodePoolModeSystem)),
					Count:        ptr.To(2),
					Type:         ptr.To(asocontainerservicev1.AgentPoolType_VirtualMachineScaleSets),
					OsDiskSizeGB: ptr.To[asocontainerservicev1.ContainerServiceOSDisk](0),
					Tags: map[string]string{
						"test-tag": "test-value",
					},
					EnableAutoScaling: ptr.To(false),
				},
				{
					Name:                ptr.To("test-agentpool-1"),
					Mode:                ptr.To(asocontainerservicev1.AgentPoolMode(infrav1.NodePoolModeUser)),
					Count:               ptr.To(4),
					Type:                ptr.To(asocontainerservicev1.AgentPoolType_VirtualMachineScaleSets),
					OsDiskSizeGB:        ptr.To[asocontainerservicev1.ContainerServiceOSDisk](0),
					VmSize:              ptr.To("test_SKU"),
					OrchestratorVersion: ptr.To("v1.22.0"),
					VnetSubnetReference: &genruntime.ResourceReference{
						ARMID: "fake/subnet/id",
					},
					MaxPods:           ptr.To(32),
					AvailabilityZones: []string{"1", "2"},
					Tags: map[string]string{
						"test-tag": "test-value",
					},
					EnableAutoScaling: ptr.To(false),
				},
			},
			LinuxProfile: &asocontainerservicev1.ContainerServiceLinuxProfile{
				AdminUsername: ptr.To(azure.DefaultAKSUserName),
				Ssh: &asocontainerservicev1.ContainerServiceSshConfiguration{
					PublicKeys: []asocontainerservicev1.ContainerServiceSshPublicKey{
						{
							KeyData: ptr.To("test-ssh-key"),
						},
					},
				},
			},
			ServicePrincipalProfile: &asocontainerservicev1.ManagedClusterServicePrincipalProfile{ClientId: ptr.To("msi")},
			NodeResourceGroup:       ptr.To("test-node-rg"),
			EnableRBAC:              ptr.To(true),
			NetworkProfile: &asocontainerservicev1.ContainerServiceNetworkProfile{
				LoadBalancerSku:   ptr.To(asocontainerservicev1.ContainerServiceNetworkProfile_LoadBalancerSku_Standard),
				NetworkPluginMode: ptr.To(asocontainerservicev1.ContainerServiceNetworkProfile_NetworkPluginMode_Overlay),
			},
			OidcIssuerProfile: &asocontainerservicev1.ManagedClusterOIDCIssuerProfile{
				Enabled: ptr.To(true),
			},
			OperatorSpec: &asocontainerservicev1.ManagedClusterOperatorSpec{
				Secrets: &asocontainerservicev1.ManagedClusterOperatorSecrets{
					AdminCredentials: &genruntime.SecretDestination{
						Name: "test-cluster-kubeconfig",
						Key:  "value",
					},
				},
			},
			Identity: &asocontainerservicev1.ManagedClusterIdentity{
				Type: ptr.To(asocontainerservicev1.ManagedClusterIdentity_Type_SystemAssigned),
			},
			Location: ptr.To("test-location"),
			Tags: infrav1.Build(infrav1.BuildParams{
				Lifecycle:   infrav1.ResourceLifecycleOwned,
				ClusterName: "test-cluster",
				Name:        ptr.To("test-managedcluster"),
				Role:        ptr.To(infrav1.CommonRole),
			}),
		},
	}
}

func getExistingClusterWithAuthorizedIPRanges() *asocontainerservicev1.ManagedCluster {
	mc := getExistingCluster()
	mc.Spec.ApiServerAccessProfile = &asocontainerservicev1.ManagedClusterAPIServerAccessProfile{
		AuthorizedIPRanges: []string{"192.168.0.1/32, 192.168.0.2/32, 192.168.0.3/32"},
	}
	return mc
}

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
	"fmt"
	"net"
	"reflect"

	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20230201"
	"github.com/Azure/azure-service-operator/v2/pkg/genruntime"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/azure/converters"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/agentpools"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/aso"
	"sigs.k8s.io/cluster-api-provider-azure/util/tele"
	"sigs.k8s.io/cluster-api/util/secret"
)

// ManagedClusterSpec contains properties to create a managed cluster.
type ManagedClusterSpec struct {
	// Name is the name of this AKS Cluster.
	Name string

	// Namespace is the namespace of the ASO ManagedCluster.
	Namespace string

	// ResourceGroup is the name of the Azure resource group for this AKS Cluster.
	ResourceGroup string

	// NodeResourceGroup is the name of the Azure resource group containing IaaS VMs.
	NodeResourceGroup string

	// ClusterName is the name of the owning Cluster API Cluster resource.
	ClusterName string

	// VnetSubnetID is the Azure Resource ID for the subnet which should contain nodes.
	VnetSubnetID string

	// Location is a string matching one of the canonical Azure region names. Examples: "westus2", "eastus".
	Location string

	// Tags is a set of tags to add to this cluster.
	Tags map[string]string

	// Version defines the desired Kubernetes version.
	Version string

	// LoadBalancerSKU for the managed cluster. Possible values include: 'Standard', 'Basic'. Defaults to Standard.
	LoadBalancerSKU string

	// NetworkPlugin used for building Kubernetes network. Possible values include: 'azure', 'kubenet'. Defaults to azure.
	NetworkPlugin string

	// NetworkPluginMode is the mode the network plugin should use.
	NetworkPluginMode *infrav1.NetworkPluginMode

	// NetworkPolicy used for building Kubernetes network. Possible values include: 'calico', 'azure'.
	NetworkPolicy string

	// OutboundType used for building Kubernetes network. Possible values include: 'loadBalancer', 'managedNATGateway', 'userAssignedNATGateway', 'userDefinedRouting'.
	OutboundType *infrav1.ManagedControlPlaneOutboundType

	// SSHPublicKey is a string literal containing an ssh public key. Will autogenerate and discard if not provided.
	SSHPublicKey string

	// GetAllAgentPools is a function that returns the list of agent pool specifications in this cluster.
	GetAllAgentPools func() ([]azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedClustersAgentPool], error)

	// PodCIDR is the CIDR block for IP addresses distributed to pods
	PodCIDR string

	// ServiceCIDR is the CIDR block for IP addresses distributed to services
	ServiceCIDR string

	// DNSServiceIP is an IP address assigned to the Kubernetes DNS service
	DNSServiceIP *string

	// AddonProfiles are the profiles of managed cluster add-on.
	AddonProfiles []AddonProfile

	// AADProfile is Azure Active Directory configuration to integrate with AKS, for aad authentication.
	AADProfile *AADProfile

	// SKU is the SKU of the AKS to be provisioned.
	SKU *SKU

	// LoadBalancerProfile is the profile of the cluster load balancer.
	LoadBalancerProfile *LoadBalancerProfile

	// APIServerAccessProfile is the access profile for AKS API server.
	APIServerAccessProfile *APIServerAccessProfile

	// AutoScalerProfile is the parameters to be applied to the cluster-autoscaler when enabled.
	AutoScalerProfile *AutoScalerProfile

	// Identity is the AKS control plane Identity configuration
	Identity *infrav1.Identity

	// KubeletUserAssignedIdentity is the user-assigned identity for kubelet to authenticate to ACR.
	KubeletUserAssignedIdentity string

	// HTTPProxyConfig is the HTTP proxy configuration for the cluster.
	HTTPProxyConfig *HTTPProxyConfig

	// OIDCIssuerProfile is the OIDC issuer profile of the Managed Cluster.
	OIDCIssuerProfile *OIDCIssuerProfile
}

// HTTPProxyConfig is the HTTP proxy configuration for the cluster.
type HTTPProxyConfig struct {
	// HTTPProxy is the HTTP proxy server endpoint to use.
	HTTPProxy *string `json:"httpProxy,omitempty"`

	// HTTPSProxy is the HTTPS proxy server endpoint to use.
	HTTPSProxy *string `json:"httpsProxy,omitempty"`

	// NoProxy is the endpoints that should not go through proxy.
	NoProxy []string `json:"noProxy,omitempty"`

	// TrustedCA is the Alternative CA cert to use for connecting to proxy servers.
	TrustedCA *string `json:"trustedCa,omitempty"`
}

// AADProfile is Azure Active Directory configuration to integrate with AKS, for aad authentication.
type AADProfile struct {
	// Managed defines whether to enable managed AAD.
	Managed bool

	// EnableAzureRBAC defines whether to enable Azure RBAC for Kubernetes authorization.
	EnableAzureRBAC bool

	// AdminGroupObjectIDs are the AAD group object IDs that will have admin role of the cluster.
	AdminGroupObjectIDs []string
}

// AddonProfile is the profile of a managed cluster add-on.
type AddonProfile struct {
	Name    string
	Config  map[string]string
	Enabled bool
}

// SKU is an AKS SKU.
type SKU struct {
	// Tier is the tier of a managed cluster SKU.
	Tier string
}

// LoadBalancerProfile is the profile of the cluster load balancer.
type LoadBalancerProfile struct {
	// Load balancer profile must specify at most one of ManagedOutboundIPs, OutboundIPPrefixes and OutboundIPs.
	// By default the AKS cluster automatically creates a public IP in the AKS-managed infrastructure resource group and assigns it to the load balancer outbound pool.
	// Alternatively, you can assign your own custom public IP or public IP prefix at cluster creation time.
	// See https://learn.microsoft.com/azure/aks/load-balancer-standard#provide-your-own-outbound-public-ips-or-prefixes

	// ManagedOutboundIPs are the desired managed outbound IPs for the cluster load balancer.
	ManagedOutboundIPs *int

	// OutboundIPPrefixes are the desired outbound IP Prefix resources for the cluster load balancer.
	OutboundIPPrefixes []string

	// OutboundIPs are the desired outbound IP resources for the cluster load balancer.
	OutboundIPs []string

	// AllocatedOutboundPorts are the desired number of allocated SNAT ports per VM. Allowed values must be in the range of 0 to 64000 (inclusive). The default value is 0 which results in Azure dynamically allocating ports.
	AllocatedOutboundPorts *int

	// IdleTimeoutInMinutes  are the desired outbound flow idle timeout in minutes. Allowed values must be in the range of 4 to 120 (inclusive). The default value is 30 minutes.
	IdleTimeoutInMinutes *int
}

// APIServerAccessProfile is the access profile for AKS API server.
type APIServerAccessProfile struct {
	// AuthorizedIPRanges are the authorized IP Ranges to kubernetes API server.
	AuthorizedIPRanges []string
	// EnablePrivateCluster defines hether to create the cluster as a private cluster or not.
	EnablePrivateCluster *bool
	// PrivateDNSZone is the private dns zone for private clusters.
	PrivateDNSZone *string
	// EnablePrivateClusterPublicFQDN defines whether to create additional public FQDN for private cluster or not.
	EnablePrivateClusterPublicFQDN *bool
}

// AutoScalerProfile parameters to be applied to the cluster-autoscaler when enabled.
type AutoScalerProfile struct {
	// BalanceSimilarNodeGroups - Valid values are 'true' and 'false'
	BalanceSimilarNodeGroups *string
	// Expander - If not specified, the default is 'random'. See [expanders](https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md#what-are-expanders) for more information.
	Expander *string
	// MaxEmptyBulkDelete - The default is 10.
	MaxEmptyBulkDelete *string
	// MaxGracefulTerminationSec - The default is 600.
	MaxGracefulTerminationSec *string
	// MaxNodeProvisionTime - The default is '15m'. Values must be an integer followed by an 'm'. No unit of time other than minutes (m) is supported.
	MaxNodeProvisionTime *string
	// MaxTotalUnreadyPercentage - The default is 45. The maximum is 100 and the minimum is 0.
	MaxTotalUnreadyPercentage *string
	// NewPodScaleUpDelay - For scenarios like burst/batch scale where you don't want CA to act before the kubernetes scheduler could schedule all the pods, you can tell CA to ignore unscheduled pods before they're a certain age. The default is '0s'. Values must be an integer followed by a unit ('s' for seconds, 'm' for minutes, 'h' for hours, etc).
	NewPodScaleUpDelay *string
	// OkTotalUnreadyCount - This must be an integer. The default is 3.
	OkTotalUnreadyCount *string
	// ScanInterval - The default is '10s'. Values must be an integer number of seconds.
	ScanInterval *string
	// ScaleDownDelayAfterAdd - The default is '10m'. Values must be an integer followed by an 'm'. No unit of time other than minutes (m) is supported.
	ScaleDownDelayAfterAdd *string
	// ScaleDownDelayAfterDelete - The default is the scan-interval. Values must be an integer followed by an 'm'. No unit of time other than minutes (m) is supported.
	ScaleDownDelayAfterDelete *string
	// ScaleDownDelayAfterFailure - The default is '3m'. Values must be an integer followed by an 'm'. No unit of time other than minutes (m) is supported.
	ScaleDownDelayAfterFailure *string
	// ScaleDownUnneededTime - The default is '10m'. Values must be an integer followed by an 'm'. No unit of time other than minutes (m) is supported.
	ScaleDownUnneededTime *string
	// ScaleDownUnreadyTime - The default is '20m'. Values must be an integer followed by an 'm'. No unit of time other than minutes (m) is supported.
	ScaleDownUnreadyTime *string
	// ScaleDownUtilizationThreshold - The default is '0.5'.
	ScaleDownUtilizationThreshold *string
	// SkipNodesWithLocalStorage - The default is true.
	SkipNodesWithLocalStorage *string
	// SkipNodesWithSystemPods - The default is true.
	SkipNodesWithSystemPods *string
}

// OIDCIssuerProfile is the OIDC issuer profile of the Managed Cluster.
type OIDCIssuerProfile struct {
	// Enabled is whether the OIDC issuer is enabled.
	Enabled *bool
}

// ResourceRef implements azure.ASOResourceSpecGetter.
func (s *ManagedClusterSpec) ResourceRef() *asocontainerservicev1.ManagedCluster {
	return &asocontainerservicev1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Name,
			Namespace: s.Namespace,
		},
	}
}

// buildAutoScalerProfile builds the AutoScalerProfile for the ManagedClusterProperties.
func buildAutoScalerProfile(autoScalerProfile *AutoScalerProfile) *asocontainerservicev1.ManagedClusterProperties_AutoScalerProfile {
	if autoScalerProfile == nil {
		return nil
	}

	mcAutoScalerProfile := &asocontainerservicev1.ManagedClusterProperties_AutoScalerProfile{
		BalanceSimilarNodeGroups:      autoScalerProfile.BalanceSimilarNodeGroups,
		MaxEmptyBulkDelete:            autoScalerProfile.MaxEmptyBulkDelete,
		MaxGracefulTerminationSec:     autoScalerProfile.MaxGracefulTerminationSec,
		MaxNodeProvisionTime:          autoScalerProfile.MaxNodeProvisionTime,
		MaxTotalUnreadyPercentage:     autoScalerProfile.MaxTotalUnreadyPercentage,
		NewPodScaleUpDelay:            autoScalerProfile.NewPodScaleUpDelay,
		OkTotalUnreadyCount:           autoScalerProfile.OkTotalUnreadyCount,
		ScanInterval:                  autoScalerProfile.ScanInterval,
		ScaleDownDelayAfterAdd:        autoScalerProfile.ScaleDownDelayAfterAdd,
		ScaleDownDelayAfterDelete:     autoScalerProfile.ScaleDownDelayAfterDelete,
		ScaleDownDelayAfterFailure:    autoScalerProfile.ScaleDownDelayAfterFailure,
		ScaleDownUnneededTime:         autoScalerProfile.ScaleDownUnneededTime,
		ScaleDownUnreadyTime:          autoScalerProfile.ScaleDownUnreadyTime,
		ScaleDownUtilizationThreshold: autoScalerProfile.ScaleDownUtilizationThreshold,
		SkipNodesWithLocalStorage:     autoScalerProfile.SkipNodesWithLocalStorage,
		SkipNodesWithSystemPods:       autoScalerProfile.SkipNodesWithSystemPods,
	}
	if autoScalerProfile.Expander != nil {
		mcAutoScalerProfile.Expander = ptr.To(asocontainerservicev1.ManagedClusterProperties_AutoScalerProfile_Expander(*autoScalerProfile.Expander))
	}

	return mcAutoScalerProfile
}

// Parameters returns the parameters for the managed clusters.
//
//nolint:gocyclo // Function requires a lot of nil checks that raise complexity.
func (s *ManagedClusterSpec) Parameters(ctx context.Context, existing *asocontainerservicev1.ManagedCluster) (params *asocontainerservicev1.ManagedCluster, err error) {
	ctx, log, done := tele.StartSpanWithLogger(ctx, "managedclusters.Service.Parameters")
	defer done()

	var decodedSSHPublicKey []byte
	if s.SSHPublicKey != "" {
		decodedSSHPublicKey, err = base64.StdEncoding.DecodeString(s.SSHPublicKey)
		if err != nil {
			return nil, errors.Wrap(err, "failed to decode SSHPublicKey")
		}
	}

	managedCluster := asocontainerservicev1.ManagedCluster_Spec{
		AzureName: s.Name,
		Owner: &genruntime.KnownResourceReference{
			Name: s.ResourceGroup,
		},
		Identity: &asocontainerservicev1.ManagedClusterIdentity{
			Type: ptr.To(asocontainerservicev1.ManagedClusterIdentity_Type_SystemAssigned),
		},
		Location: &s.Location,
		Tags: infrav1.Build(infrav1.BuildParams{
			Lifecycle:   infrav1.ResourceLifecycleOwned,
			ClusterName: s.ClusterName,
			Name:        ptr.To(s.Name),
			Role:        ptr.To(infrav1.CommonRole),
			Additional:  s.Tags,
		}),
		NodeResourceGroup: &s.NodeResourceGroup,
		EnableRBAC:        ptr.To(true),
		DnsPrefix:         &s.Name,
		KubernetesVersion: &s.Version,
		ServicePrincipalProfile: &asocontainerservicev1.ManagedClusterServicePrincipalProfile{
			ClientId: ptr.To("msi"),
		},
		AgentPoolProfiles: []asocontainerservicev1.ManagedClusterAgentPoolProfile{},
		NetworkProfile: &asocontainerservicev1.ContainerServiceNetworkProfile{
			NetworkPlugin:   azure.AliasOrNil[asocontainerservicev1.ContainerServiceNetworkProfile_NetworkPlugin](&s.NetworkPlugin),
			LoadBalancerSku: azure.AliasOrNil[asocontainerservicev1.ContainerServiceNetworkProfile_LoadBalancerSku](&s.LoadBalancerSKU),
			NetworkPolicy:   azure.AliasOrNil[asocontainerservicev1.ContainerServiceNetworkProfile_NetworkPolicy](&s.NetworkPolicy),
		},
		OperatorSpec: &asocontainerservicev1.ManagedClusterOperatorSpec{
			Secrets: &asocontainerservicev1.ManagedClusterOperatorSecrets{
				AdminCredentials: &genruntime.SecretDestination{
					Name: secret.Name(s.ClusterName, secret.Kubeconfig),
					Key:  secret.KubeconfigDataName,
				},
			},
		},
	}

	if decodedSSHPublicKey != nil {
		managedCluster.LinuxProfile = &asocontainerservicev1.ContainerServiceLinuxProfile{
			AdminUsername: ptr.To(azure.DefaultAKSUserName),
			Ssh: &asocontainerservicev1.ContainerServiceSshConfiguration{
				PublicKeys: []asocontainerservicev1.ContainerServiceSshPublicKey{
					{
						KeyData: ptr.To(string(decodedSSHPublicKey)),
					},
				},
			},
		}
	}

	if s.NetworkPluginMode != nil {
		managedCluster.NetworkProfile.NetworkPluginMode = ptr.To(asocontainerservicev1.ContainerServiceNetworkProfile_NetworkPluginMode(*s.NetworkPluginMode))
	}

	if s.PodCIDR != "" {
		managedCluster.NetworkProfile.PodCidr = &s.PodCIDR
	}

	if s.ServiceCIDR != "" {
		if s.DNSServiceIP == nil {
			managedCluster.NetworkProfile.ServiceCidr = &s.ServiceCIDR
			ip, _, err := net.ParseCIDR(s.ServiceCIDR)
			if err != nil {
				return nil, fmt.Errorf("failed to parse service cidr: %w", err)
			}
			// HACK: set the last octet of the IP to .10
			// This ensures the dns IP is valid in the service cidr without forcing the user
			// to specify it in both the Capi cluster and the Azure control plane.
			// https://golang.org/src/net/ip.go#L48
			ip[15] = byte(10)
			dnsIP := ip.String()
			managedCluster.NetworkProfile.DnsServiceIP = &dnsIP
		} else {
			managedCluster.NetworkProfile.DnsServiceIP = s.DNSServiceIP
		}
	}

	if s.AADProfile != nil {
		managedCluster.AadProfile = &asocontainerservicev1.ManagedClusterAADProfile{
			Managed:             &s.AADProfile.Managed,
			EnableAzureRBAC:     &s.AADProfile.EnableAzureRBAC,
			AdminGroupObjectIDs: s.AADProfile.AdminGroupObjectIDs,
		}
	}

	for i := range s.AddonProfiles {
		if managedCluster.AddonProfiles == nil {
			managedCluster.AddonProfiles = map[string]asocontainerservicev1.ManagedClusterAddonProfile{}
		}
		item := s.AddonProfiles[i]
		addonProfile := asocontainerservicev1.ManagedClusterAddonProfile{
			Enabled: &item.Enabled,
		}
		if item.Config != nil {
			addonProfile.Config = item.Config
		}
		managedCluster.AddonProfiles[item.Name] = addonProfile
	}

	if s.SKU != nil {
		tierName := asocontainerservicev1.ManagedClusterSKU_Tier(s.SKU.Tier)
		managedCluster.Sku = &asocontainerservicev1.ManagedClusterSKU{
			Name: ptr.To(asocontainerservicev1.ManagedClusterSKU_Name("Base")),
			Tier: ptr.To(tierName),
		}
	}

	if s.LoadBalancerProfile != nil {
		managedCluster.NetworkProfile.LoadBalancerProfile = s.GetLoadBalancerProfile()
	}

	if s.APIServerAccessProfile != nil {
		managedCluster.ApiServerAccessProfile = &asocontainerservicev1.ManagedClusterAPIServerAccessProfile{
			EnablePrivateCluster:           s.APIServerAccessProfile.EnablePrivateCluster,
			PrivateDNSZone:                 s.APIServerAccessProfile.PrivateDNSZone,
			EnablePrivateClusterPublicFQDN: s.APIServerAccessProfile.EnablePrivateClusterPublicFQDN,
		}

		if s.APIServerAccessProfile.AuthorizedIPRanges != nil {
			managedCluster.ApiServerAccessProfile.AuthorizedIPRanges = s.APIServerAccessProfile.AuthorizedIPRanges
		}
	}

	if s.OutboundType != nil {
		managedCluster.NetworkProfile.OutboundType = ptr.To(asocontainerservicev1.ContainerServiceNetworkProfile_OutboundType(*s.OutboundType))
	}

	managedCluster.AutoScalerProfile = buildAutoScalerProfile(s.AutoScalerProfile)

	if s.Identity != nil {
		managedCluster.Identity, err = getIdentity(s.Identity)
		if err != nil {
			return nil, errors.Wrapf(err, "Identity is not valid: %s", err)
		}
	}

	if s.KubeletUserAssignedIdentity != "" {
		managedCluster.IdentityProfile = map[string]asocontainerservicev1.UserAssignedIdentity{
			kubeletIdentityKey: {
				ResourceReference: &genruntime.ResourceReference{
					ARMID: s.KubeletUserAssignedIdentity,
				},
			},
		}
	}

	if s.HTTPProxyConfig != nil {
		managedCluster.HttpProxyConfig = &asocontainerservicev1.ManagedClusterHTTPProxyConfig{
			HttpProxy:  s.HTTPProxyConfig.HTTPProxy,
			HttpsProxy: s.HTTPProxyConfig.HTTPSProxy,
			TrustedCa:  s.HTTPProxyConfig.TrustedCA,
		}

		if s.HTTPProxyConfig.NoProxy != nil {
			managedCluster.HttpProxyConfig.NoProxy = s.HTTPProxyConfig.NoProxy
		}
	}

	if s.OIDCIssuerProfile != nil {
		managedCluster.OidcIssuerProfile = &asocontainerservicev1.ManagedClusterOIDCIssuerProfile{
			Enabled: s.OIDCIssuerProfile.Enabled,
		}
	}

	mc := existing
	if existing != nil {
		// DeepCopy to prevent changes from affecting `existing` which gets returned exactly as passed in when
		// no changes are needed.
		existingMC := existing.DeepCopy().Spec

		// Normalize the LoadBalancerProfile so the diff below doesn't get thrown off by AKS added properties.
		if managedCluster.NetworkProfile.LoadBalancerProfile == nil {
			// If our LoadBalancerProfile generated by the spec is nil, then don't worry about what AKS has added.
			existingMC.NetworkProfile.LoadBalancerProfile = nil
		} else {
			// If our LoadBalancerProfile generated by the spec is not nil, then remove the effective outbound IPs from
			// AKS.
			if existingMC.NetworkProfile == nil {
				existingMC.NetworkProfile = &asocontainerservicev1.ContainerServiceNetworkProfile{}
			}
			if existingMC.NetworkProfile.LoadBalancerProfile == nil {
				existingMC.NetworkProfile.LoadBalancerProfile = &asocontainerservicev1.ManagedClusterLoadBalancerProfile{}
			}
			existingMC.NetworkProfile.LoadBalancerProfile.EffectiveOutboundIPs = nil
		}

		// Avoid changing agent pool profiles through AMCP and just use the existing agent pool profiles
		// AgentPool changes are managed through AMMP.
		managedCluster.AgentPoolProfiles = existing.Spec.AgentPoolProfiles

		// if the AuthorizedIPRanges is nil in the user-updated spec, but not nil in the existing spec, then
		// we need to set the AuthorizedIPRanges to empty array ([]*string{}) once so that the Azure API will
		// update the existing authorized IP ranges to nil.
		if !isAuthIPRangesNilOrEmpty(existingMC) && isAuthIPRangesNilOrEmpty(managedCluster) {
			log.V(4).Info("managed cluster spec has nil AuthorizedIPRanges, updating existing authorized IP ranges to an empty list")
			managedCluster.ApiServerAccessProfile = &asocontainerservicev1.ManagedClusterAPIServerAccessProfile{
				AuthorizedIPRanges: []string{},
			}
		}

		diff := computeDiffOfNormalizedClusters(managedCluster, existingMC)
		if diff == "" {
			log.V(4).Info("no changes found between user-updated spec and existing spec")
			return existing, nil
		}
		log.V(4).Info("found a diff between the desired spec and the existing managed cluster", "difference", diff)

		// Remove agent pool profiles from request once they've been created to let the
		// AzureManagedMachinePool controller handle them. Otherwise changes to the AzureManagedControlPlane
		// would overwrite changes made to the (AzureManaged)MachinePools.
		if existing.Status.AgentPoolProfiles != nil {
			managedCluster.AgentPoolProfiles = nil
		}
	} else {
		mc = &asocontainerservicev1.ManagedCluster{}
		// Add all agent pools to cluster spec that will be submitted to the API
		agentPoolSpecs, err := s.GetAllAgentPools()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get agent pool specs for managed cluster %s", s.Name)
		}

		for _, spec := range agentPoolSpecs {
			agentPool, err := spec.Parameters(ctx, nil)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get agent pool parameters for managed cluster %s", s.Name)
			}
			agentPoolSpec := spec.(*agentpools.AgentPoolSpec)
			agentPool.Spec.AzureName = agentPoolSpec.AzureName
			profile := converters.AgentPoolToManagedClusterAgentPoolProfile(agentPool)
			managedCluster.AgentPoolProfiles = append(managedCluster.AgentPoolProfiles, profile)
		}
	}
	mc.Spec = managedCluster

	return mc, nil
}

// GetLoadBalancerProfile returns an asocontainerservicev1.ManagedClusterLoadBalancerProfile from the
// information present in ManagedClusterSpec.LoadBalancerProfile.
func (s *ManagedClusterSpec) GetLoadBalancerProfile() (loadBalancerProfile *asocontainerservicev1.ManagedClusterLoadBalancerProfile) {
	loadBalancerProfile = &asocontainerservicev1.ManagedClusterLoadBalancerProfile{
		AllocatedOutboundPorts: s.LoadBalancerProfile.AllocatedOutboundPorts,
		IdleTimeoutInMinutes:   s.LoadBalancerProfile.IdleTimeoutInMinutes,
	}
	if s.LoadBalancerProfile.ManagedOutboundIPs != nil {
		loadBalancerProfile.ManagedOutboundIPs = &asocontainerservicev1.ManagedClusterLoadBalancerProfile_ManagedOutboundIPs{Count: s.LoadBalancerProfile.ManagedOutboundIPs}
	}
	if len(s.LoadBalancerProfile.OutboundIPPrefixes) > 0 {
		loadBalancerProfile.OutboundIPPrefixes = &asocontainerservicev1.ManagedClusterLoadBalancerProfile_OutboundIPPrefixes{
			PublicIPPrefixes: convertToResourceReferences(s.LoadBalancerProfile.OutboundIPPrefixes),
		}
	}
	if len(s.LoadBalancerProfile.OutboundIPs) > 0 {
		loadBalancerProfile.OutboundIPs = &asocontainerservicev1.ManagedClusterLoadBalancerProfile_OutboundIPs{
			PublicIPs: convertToResourceReferences(s.LoadBalancerProfile.OutboundIPs),
		}
	}
	return
}

func convertToResourceReferences(resources []string) []asocontainerservicev1.ResourceReference {
	resourceReferences := make([]asocontainerservicev1.ResourceReference, len(resources))
	for i := range resources {
		resourceReferences[i] = asocontainerservicev1.ResourceReference{
			Reference: &genruntime.ResourceReference{
				ARMID: resources[i],
			},
		}
	}
	return resourceReferences
}

func computeDiffOfNormalizedClusters(managedCluster asocontainerservicev1.ManagedCluster_Spec, existingMC asocontainerservicev1.ManagedCluster_Spec) string {
	// Normalize properties for the desired (CR spec) and existing managed
	// cluster, so that we check only those fields that were specified in
	// the initial CreateOrUpdate request and that can be modified.
	// Without comparing to normalized properties, we would always get a
	// difference in desired and existing, which would result in sending
	// unnecessary Azure API requests.
	propertiesNormalized := asocontainerservicev1.ManagedCluster_Spec{
		KubernetesVersion: managedCluster.KubernetesVersion,
		NetworkProfile:    &asocontainerservicev1.ContainerServiceNetworkProfile{},
		AutoScalerProfile: &asocontainerservicev1.ManagedClusterProperties_AutoScalerProfile{},
	}

	existingMCPropertiesNormalized := asocontainerservicev1.ManagedCluster_Spec{
		KubernetesVersion: existingMC.KubernetesVersion,
		NetworkProfile:    &asocontainerservicev1.ContainerServiceNetworkProfile{},
		AutoScalerProfile: &asocontainerservicev1.ManagedClusterProperties_AutoScalerProfile{},
	}

	if managedCluster.AadProfile != nil {
		propertiesNormalized.AadProfile = &asocontainerservicev1.ManagedClusterAADProfile{
			Managed:             managedCluster.AadProfile.Managed,
			EnableAzureRBAC:     managedCluster.AadProfile.EnableAzureRBAC,
			AdminGroupObjectIDs: managedCluster.AadProfile.AdminGroupObjectIDs,
		}
	}

	if existingMC.AadProfile != nil {
		existingMCPropertiesNormalized.AadProfile = &asocontainerservicev1.ManagedClusterAADProfile{
			Managed:             existingMC.AadProfile.Managed,
			EnableAzureRBAC:     existingMC.AadProfile.EnableAzureRBAC,
			AdminGroupObjectIDs: existingMC.AadProfile.AdminGroupObjectIDs,
		}
	}

	if existingMC.NetworkProfile != nil {
		existingMCPropertiesNormalized.NetworkProfile.LoadBalancerProfile = existingMC.NetworkProfile.LoadBalancerProfile

		existingMCPropertiesNormalized.NetworkProfile.NetworkPluginMode = existingMC.NetworkProfile.NetworkPluginMode
	}
	if managedCluster.NetworkProfile != nil {
		propertiesNormalized.NetworkProfile.LoadBalancerProfile = managedCluster.NetworkProfile.LoadBalancerProfile

		propertiesNormalized.NetworkProfile.NetworkPluginMode = managedCluster.NetworkProfile.NetworkPluginMode
		if propertiesNormalized.NetworkProfile.NetworkPluginMode == nil {
			propertiesNormalized.NetworkProfile.NetworkPluginMode = existingMCPropertiesNormalized.NetworkProfile.NetworkPluginMode
		}
	}

	if managedCluster.ApiServerAccessProfile != nil {
		propertiesNormalized.ApiServerAccessProfile = &asocontainerservicev1.ManagedClusterAPIServerAccessProfile{
			AuthorizedIPRanges: managedCluster.ApiServerAccessProfile.AuthorizedIPRanges,
		}
	}

	if existingMC.ApiServerAccessProfile != nil {
		existingMCPropertiesNormalized.ApiServerAccessProfile = &asocontainerservicev1.ManagedClusterAPIServerAccessProfile{
			AuthorizedIPRanges: existingMC.ApiServerAccessProfile.AuthorizedIPRanges,
		}
	}

	if managedCluster.AutoScalerProfile != nil {
		propertiesNormalized.AutoScalerProfile = &asocontainerservicev1.ManagedClusterProperties_AutoScalerProfile{
			BalanceSimilarNodeGroups:      managedCluster.AutoScalerProfile.BalanceSimilarNodeGroups,
			Expander:                      managedCluster.AutoScalerProfile.Expander,
			MaxEmptyBulkDelete:            managedCluster.AutoScalerProfile.MaxEmptyBulkDelete,
			MaxGracefulTerminationSec:     managedCluster.AutoScalerProfile.MaxGracefulTerminationSec,
			MaxNodeProvisionTime:          managedCluster.AutoScalerProfile.MaxNodeProvisionTime,
			MaxTotalUnreadyPercentage:     managedCluster.AutoScalerProfile.MaxTotalUnreadyPercentage,
			NewPodScaleUpDelay:            managedCluster.AutoScalerProfile.NewPodScaleUpDelay,
			OkTotalUnreadyCount:           managedCluster.AutoScalerProfile.OkTotalUnreadyCount,
			ScanInterval:                  managedCluster.AutoScalerProfile.ScanInterval,
			ScaleDownDelayAfterAdd:        managedCluster.AutoScalerProfile.ScaleDownDelayAfterAdd,
			ScaleDownDelayAfterDelete:     managedCluster.AutoScalerProfile.ScaleDownDelayAfterDelete,
			ScaleDownDelayAfterFailure:    managedCluster.AutoScalerProfile.ScaleDownDelayAfterFailure,
			ScaleDownUnneededTime:         managedCluster.AutoScalerProfile.ScaleDownUnneededTime,
			ScaleDownUnreadyTime:          managedCluster.AutoScalerProfile.ScaleDownUnreadyTime,
			ScaleDownUtilizationThreshold: managedCluster.AutoScalerProfile.ScaleDownUtilizationThreshold,
			SkipNodesWithLocalStorage:     managedCluster.AutoScalerProfile.SkipNodesWithLocalStorage,
			SkipNodesWithSystemPods:       managedCluster.AutoScalerProfile.SkipNodesWithSystemPods,
		}
	}

	if existingMC.AutoScalerProfile != nil {
		existingMCPropertiesNormalized.AutoScalerProfile = &asocontainerservicev1.ManagedClusterProperties_AutoScalerProfile{
			BalanceSimilarNodeGroups:      existingMC.AutoScalerProfile.BalanceSimilarNodeGroups,
			Expander:                      existingMC.AutoScalerProfile.Expander,
			MaxEmptyBulkDelete:            existingMC.AutoScalerProfile.MaxEmptyBulkDelete,
			MaxGracefulTerminationSec:     existingMC.AutoScalerProfile.MaxGracefulTerminationSec,
			MaxNodeProvisionTime:          existingMC.AutoScalerProfile.MaxNodeProvisionTime,
			MaxTotalUnreadyPercentage:     existingMC.AutoScalerProfile.MaxTotalUnreadyPercentage,
			NewPodScaleUpDelay:            existingMC.AutoScalerProfile.NewPodScaleUpDelay,
			OkTotalUnreadyCount:           existingMC.AutoScalerProfile.OkTotalUnreadyCount,
			ScanInterval:                  existingMC.AutoScalerProfile.ScanInterval,
			ScaleDownDelayAfterAdd:        existingMC.AutoScalerProfile.ScaleDownDelayAfterAdd,
			ScaleDownDelayAfterDelete:     existingMC.AutoScalerProfile.ScaleDownDelayAfterDelete,
			ScaleDownDelayAfterFailure:    existingMC.AutoScalerProfile.ScaleDownDelayAfterFailure,
			ScaleDownUnneededTime:         existingMC.AutoScalerProfile.ScaleDownUnneededTime,
			ScaleDownUnreadyTime:          existingMC.AutoScalerProfile.ScaleDownUnreadyTime,
			ScaleDownUtilizationThreshold: existingMC.AutoScalerProfile.ScaleDownUtilizationThreshold,
			SkipNodesWithLocalStorage:     existingMC.AutoScalerProfile.SkipNodesWithLocalStorage,
			SkipNodesWithSystemPods:       existingMC.AutoScalerProfile.SkipNodesWithSystemPods,
		}
	}

	if managedCluster.IdentityProfile != nil {
		propertiesNormalized.IdentityProfile = map[string]asocontainerservicev1.UserAssignedIdentity{
			kubeletIdentityKey: {
				ResourceReference: &genruntime.ResourceReference{
					ARMID: managedCluster.IdentityProfile[kubeletIdentityKey].ResourceReference.ARMID,
				},
			},
		}
	}

	if existingMC.IdentityProfile != nil {
		existingMCPropertiesNormalized.IdentityProfile = map[string]asocontainerservicev1.UserAssignedIdentity{
			kubeletIdentityKey: {
				ResourceReference: &genruntime.ResourceReference{
					ARMID: existingMC.IdentityProfile[kubeletIdentityKey].ResourceReference.ARMID,
				},
			},
		}
	}

	// Once the AKS autoscaler has been updated it will always return values so we need to
	// respect those values even though the settings are now not being explicitly set by CAPZ.
	if existingMC.AutoScalerProfile != nil && managedCluster.AutoScalerProfile == nil {
		existingMCPropertiesNormalized.AutoScalerProfile = nil
		propertiesNormalized.AutoScalerProfile = nil
	}

	clusterNormalized := propertiesNormalized
	existingMCClusterNormalized := existingMCPropertiesNormalized

	if managedCluster.Identity != nil {
		clusterNormalized.Identity = &asocontainerservicev1.ManagedClusterIdentity{
			Type:                   managedCluster.Identity.Type,
			UserAssignedIdentities: managedCluster.Identity.UserAssignedIdentities,
		}
	}

	if existingMC.Identity != nil {
		existingMCClusterNormalized.Identity = &asocontainerservicev1.ManagedClusterIdentity{
			Type:                   existingMC.Identity.Type,
			UserAssignedIdentities: existingMC.Identity.UserAssignedIdentities,
		}
	}

	if managedCluster.Sku != nil {
		clusterNormalized.Sku = managedCluster.Sku
	}
	if existingMC.Sku != nil {
		existingMCClusterNormalized.Sku = existingMC.Sku
	}

	if managedCluster.OidcIssuerProfile != nil {
		clusterNormalized.OidcIssuerProfile = &asocontainerservicev1.ManagedClusterOIDCIssuerProfile{
			Enabled: managedCluster.OidcIssuerProfile.Enabled,
		}
	}
	if existingMC.OidcIssuerProfile != nil {
		existingMCClusterNormalized.OidcIssuerProfile = &asocontainerservicev1.ManagedClusterOIDCIssuerProfile{
			Enabled: existingMC.OidcIssuerProfile.Enabled,
		}
	}

	diff := cmp.Diff(clusterNormalized, existingMCClusterNormalized)
	return diff
}

func getIdentity(identity *infrav1.Identity) (managedClusterIdentity *asocontainerservicev1.ManagedClusterIdentity, err error) {
	if identity.Type == "" {
		return
	}

	managedClusterIdentity = &asocontainerservicev1.ManagedClusterIdentity{
		Type: ptr.To(asocontainerservicev1.ManagedClusterIdentity_Type(identity.Type)),
	}
	if ptr.Deref(managedClusterIdentity.Type, "") == asocontainerservicev1.ManagedClusterIdentity_Type_UserAssigned {
		if identity.UserAssignedIdentityResourceID == "" {
			err = errors.Errorf("Identity is set to \"UserAssigned\" but no UserAssignedIdentityResourceID is present")
			return
		}
		managedClusterIdentity.UserAssignedIdentities = []asocontainerservicev1.UserAssignedIdentityDetails{
			{
				Reference: genruntime.ResourceReference{
					ARMID: identity.UserAssignedIdentityResourceID,
				},
			},
		}
	}
	return
}

// isAuthIPRangesNilOrEmpty returns true if the managed cluster's APIServerAccessProfile or AuthorizedIPRanges is nil or if AuthorizedIPRanges is empty.
func isAuthIPRangesNilOrEmpty(managedCluster asocontainerservicev1.ManagedCluster_Spec) bool {
	return managedCluster.ApiServerAccessProfile == nil ||
		managedCluster.ApiServerAccessProfile.AuthorizedIPRanges == nil ||
		reflect.DeepEqual(managedCluster.ApiServerAccessProfile.AuthorizedIPRanges, []string{})
}

// WasManaged implements azure.ASOResourceSpecGetter.
func (s *ManagedClusterSpec) WasManaged(resource *asocontainerservicev1.ManagedCluster) bool {
	return true
}

var _ aso.TagsGetterSetter[*asocontainerservicev1.ManagedCluster] = (*ManagedClusterSpec)(nil)

// GetAdditionalTags implements aso.TagsGetterSetter.
func (s *ManagedClusterSpec) GetAdditionalTags() infrav1.Tags {
	return s.Tags
}

// GetDesiredTags implements aso.TagsGetterSetter.
func (*ManagedClusterSpec) GetDesiredTags(resource *asocontainerservicev1.ManagedCluster) infrav1.Tags {
	return resource.Spec.Tags
}

// GetActualTags implements aso.TagsGetterSetter.
func (*ManagedClusterSpec) GetActualTags(resource *asocontainerservicev1.ManagedCluster) infrav1.Tags {
	return resource.Status.Tags
}

// SetTags implements aso.TagsGetterSetter.
func (*ManagedClusterSpec) SetTags(resource *asocontainerservicev1.ManagedCluster, tags infrav1.Tags) {
	resource.Spec.Tags = tags
}

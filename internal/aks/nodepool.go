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
package aks

import (
	asocontainerservicev1hub "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20231001/storage"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func SetAgentPoolProfilesFromAgentPools[T conversion.Convertible](managedCluster conversion.Convertible, agentPools []T) error {
	hubMC := &asocontainerservicev1hub.ManagedCluster{}
	err := managedCluster.ConvertTo(hubMC)
	if err != nil {
		return err
	}
	hubMC.Spec.AgentPoolProfiles = nil

	for _, agentPool := range agentPools {
		hubPool := &asocontainerservicev1hub.ManagedClustersAgentPool{}
		err := agentPool.ConvertTo(hubPool)
		if err != nil {
			return err
		}

		profile := asocontainerservicev1hub.ManagedClusterAgentPoolProfile{
			AvailabilityZones:                 hubPool.Spec.AvailabilityZones,
			CapacityReservationGroupReference: hubPool.Spec.CapacityReservationGroupReference,
			Count:                             hubPool.Spec.Count,
			CreationData:                      hubPool.Spec.CreationData,
			EnableAutoScaling:                 hubPool.Spec.EnableAutoScaling,
			EnableEncryptionAtHost:            hubPool.Spec.EnableEncryptionAtHost,
			EnableFIPS:                        hubPool.Spec.EnableFIPS,
			EnableNodePublicIP:                hubPool.Spec.EnableNodePublicIP,
			EnableUltraSSD:                    hubPool.Spec.EnableUltraSSD,
			GpuInstanceProfile:                hubPool.Spec.GpuInstanceProfile,
			HostGroupReference:                hubPool.Spec.HostGroupReference,
			KubeletConfig:                     hubPool.Spec.KubeletConfig,
			KubeletDiskType:                   hubPool.Spec.KubeletDiskType,
			LinuxOSConfig:                     hubPool.Spec.LinuxOSConfig,
			MaxCount:                          hubPool.Spec.MaxCount,
			MaxPods:                           hubPool.Spec.MaxPods,
			MinCount:                          hubPool.Spec.MinCount,
			Mode:                              hubPool.Spec.Mode,
			Name:                              ptr.To(hubPool.Spec.AzureName),
			NetworkProfile:                    hubPool.Spec.NetworkProfile,
			NodeLabels:                        hubPool.Spec.NodeLabels,
			NodePublicIPPrefixReference:       hubPool.Spec.NodePublicIPPrefixReference,
			NodeTaints:                        hubPool.Spec.NodeTaints,
			OrchestratorVersion:               hubPool.Spec.OrchestratorVersion,
			OsDiskSizeGB:                      hubPool.Spec.OsDiskSizeGB,
			OsDiskType:                        hubPool.Spec.OsDiskType,
			OsSKU:                             hubPool.Spec.OsSKU,
			OsType:                            hubPool.Spec.OsType,
			PodSubnetReference:                hubPool.Spec.PodSubnetReference,
			PowerState:                        hubPool.Spec.PowerState,
			PropertyBag:                       hubPool.Spec.PropertyBag,
			ProximityPlacementGroupReference:  hubPool.Spec.ProximityPlacementGroupReference,
			ScaleDownMode:                     hubPool.Spec.ScaleDownMode,
			ScaleSetEvictionPolicy:            hubPool.Spec.ScaleSetEvictionPolicy,
			ScaleSetPriority:                  hubPool.Spec.ScaleSetPriority,
			SpotMaxPrice:                      hubPool.Spec.SpotMaxPrice,
			Tags:                              hubPool.Spec.Tags,
			Type:                              hubPool.Spec.Type,
			UpgradeSettings:                   hubPool.Spec.UpgradeSettings,
			VmSize:                            hubPool.Spec.VmSize,
			VnetSubnetReference:               hubPool.Spec.VnetSubnetReference,
			WorkloadRuntime:                   hubPool.Spec.WorkloadRuntime,
		}

		hubMC.Spec.AgentPoolProfiles = append(hubMC.Spec.AgentPoolProfiles, profile)
	}

	return managedCluster.ConvertFrom(hubMC)
}

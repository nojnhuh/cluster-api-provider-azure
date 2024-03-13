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
	"strings"

	asocontainerservicev1hub "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20231001/storage"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
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

func SetAgentPoolDefaults(u *unstructured.Unstructured, machinePool *expv1.MachinePool) error {
	// TODO: do this in a webhook. Or not? maybe never let users set this in the ASO resource and silently
	// propagate it here so the CAPZ manifest doesn't have two fields that mean the same thing where it's
	// not obvious which one is authoritative?
	err := unstructured.SetNestedField(u.UnstructuredContent(), strings.TrimPrefix(ptr.Deref(machinePool.Spec.Template.Spec.Version, ""), "v"), "spec", "orchestratorVersion")
	if err != nil {
		return err
	}

	var count any
	autoscaling, _, err := unstructured.NestedBool(u.UnstructuredContent(), "spec", "enableAutoScaling")
	if err != nil {
		return err
	}
	if autoscaling {
		if machinePool.Annotations == nil {
			machinePool.Annotations = make(map[string]string)
		}
		// TODO: do we need to patch the MachinePool in the AzureManagedControlPlane reconciliation? Or is the
		// first AzureManagedMachinePool reconciliation doing it enough?
		machinePool.Annotations[clusterv1.ReplicasManagedByAnnotation] = "aks"
	} else {
		count = int64(ptr.Deref(machinePool.Spec.Replicas, 1))
		delete(machinePool.Annotations, clusterv1.ReplicasManagedByAnnotation)
	}
	err = unstructured.SetNestedField(u.UnstructuredContent(), count, "spec", "count")
	if err != nil {
		return err
	}

	return nil
}

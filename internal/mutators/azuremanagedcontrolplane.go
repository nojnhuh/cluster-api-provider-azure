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

package mutators

import (
	"context"
	"errors"
	"fmt"
	"strings"

	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20231001"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1 "sigs.k8s.io/cluster-api-provider-azure/v2/api/v2alpha1"
	"sigs.k8s.io/cluster-api-provider-azure/v2/internal/aks"
)

var (
	NoManagedClusterDefinedErr    = fmt.Errorf("no %s ManagedCluster defined in AzureManagedControlPlane spec.resources", asocontainerservicev1.GroupVersion.Group)
	NoAzureManagedMachinePoolsErr = errors.New("no AzureManagedMachinePools found for AzureManagedControlPlane")
)

func SetManagedClusterDefaults(ctrlClient client.Client, azureManagedControlPlane *infrav1.AzureManagedControlPlane, cluster *clusterv1.Cluster, machinePools []*expv1.MachinePool) ResourcesMutator {
	return func(ctx context.Context, us []*unstructured.Unstructured) error {
		log := ctrl.LoggerFrom(ctx)

		var managedCluster *unstructured.Unstructured
		var managedClusterPath string
		for i, u := range us {
			if u.GroupVersionKind().Group == asocontainerservicev1.GroupVersion.Group &&
				u.GroupVersionKind().Kind == "ManagedCluster" {
				managedCluster = u
				managedClusterPath = fmt.Sprintf("spec.resources[%d]", i)
				break
			}
		}
		if managedCluster == nil {
			return reconcile.TerminalError(NoManagedClusterDefinedErr)
		}

		// TODO: default/validate this isn't set in a webhook. Check to make sure ClusterClass controller doesn't fight with
		// a webhook-defaulted value
		// TODO: maybe conversion can help us work with a typed object here instead? Would require always
		// keeping an up-to-date scheme populated with all versions.

		// TODO: auto k8s version upgrades?
		k8sVersionPath := []string{"spec", "kubernetesVersion"}
		userK8sVersion, k8sVersionFound, err := unstructured.NestedString(managedCluster.UnstructuredContent(), k8sVersionPath...)
		if err != nil {
			return err
		}
		capzK8sVersion := strings.TrimPrefix(azureManagedControlPlane.Spec.Version, "v")
		setK8sVersion := mutation{
			location: managedClusterPath + "." + strings.Join(k8sVersionPath, "."),
			val:      capzK8sVersion,
			reason:   "because spec.version is set to " + azureManagedControlPlane.Spec.Version,
		}
		if k8sVersionFound && userK8sVersion != capzK8sVersion {
			return Incompatible{
				mutation: setK8sVersion,
				userVal:  userK8sVersion,
			}
		}
		logMutation(log, setK8sVersion)
		err = unstructured.SetNestedField(managedCluster.UnstructuredContent(), capzK8sVersion, k8sVersionPath...)
		if err != nil {
			return err
		}

		agentPoolProfilesPath := []string{"spec", "agentPoolProfiles"}
		userAgentPoolProfiles, agentPoolProfilesFound, err := unstructured.NestedSlice(managedCluster.UnstructuredContent(), agentPoolProfilesPath...)
		if err != nil {
			return err
		}
		setAgentPoolProfiles := mutation{
			location: managedClusterPath + "." + strings.Join(agentPoolProfilesPath, "."),
			val:      "nil",
			reason:   "because agent pool definitions must be inherited from AzureManagedMachinePools",
		}
		if agentPoolProfilesFound {
			return Incompatible{
				mutation: setAgentPoolProfiles,
				userVal:  fmt.Sprintf("<slice of length %d>", len(userAgentPoolProfiles)),
			}
		}

		// AKS requires ManagedClusters to be created with agent pools: https://github.com/Azure/azure-service-operator/issues/2791
		getMC := &asocontainerservicev1.ManagedCluster{}
		err = ctrlClient.Get(ctx, client.ObjectKey{Namespace: azureManagedControlPlane.Namespace, Name: managedCluster.GetName()}, getMC)
		if client.IgnoreNotFound(err) != nil {
			return err
		}
		if len(getMC.Status.AgentPoolProfiles) == 0 {
			log.Info("gathering agent pool profiles to include in ManagedCluster create")
			agentPools, err := agentPoolsFromManagedMachinePools(ctx, ctrlClient, cluster.Name, azureManagedControlPlane, machinePools)
			if err != nil {
				return err
			}
			mc, err := ctrlClient.Scheme().New(managedCluster.GroupVersionKind())
			if err != nil {
				return err
			}
			err = ctrlClient.Scheme().Convert(managedCluster, mc, nil)
			if err != nil {
				return err
			}
			setAgentPoolProfiles.val = fmt.Sprintf("<slice of length %d>", len(agentPools))
			logMutation(log, setAgentPoolProfiles)
			err = aks.SetAgentPoolProfilesFromAgentPools(mc.(conversion.Convertible), agentPools)
			err = ctrlClient.Scheme().Convert(mc, managedCluster, nil)
			if err != nil {
				return err
			}
		}

		return nil
	}
}

func agentPoolsFromManagedMachinePools(ctx context.Context, ctrlClient client.Client, clusterName string, azureManagedControlPlane *infrav1.AzureManagedControlPlane, machinePools []*expv1.MachinePool) ([]conversion.Convertible, error) {
	log := ctrl.LoggerFrom(ctx)

	azureManagedMachinePools := &infrav1.AzureManagedMachinePoolList{}
	err := ctrlClient.List(ctx, azureManagedMachinePools,
		client.InNamespace(azureManagedControlPlane.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterNameLabel: clusterName,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list AzureManagedMachinePools: %w", err)
	}
	if len(azureManagedMachinePools.Items) == 0 {
		// TODO: let this fail so we don't have to check for it?
		return nil, NoAzureManagedMachinePoolsErr
	}

	var agentPools []conversion.Convertible
	for _, azureManagedMachinePool := range azureManagedMachinePools.Items {
		var machinePoolName string
		// This logic to get the associated machine pool is essentially utilexp.GetOwnerMachinePool but
		// operating against a plain list of objects vs. fetching them with a client.
		for _, ref := range azureManagedMachinePool.OwnerReferences {
			if ref.Kind != "MachinePool" {
				continue
			}
			gv, err := schema.ParseGroupVersion(ref.APIVersion)
			if err != nil {
				return nil, err
			}
			if gv.Group == expv1.GroupVersion.Group {
				machinePoolName = ref.Name
				break
			}
		}
		var machinePool *expv1.MachinePool
		for _, mp := range machinePools {
			if mp.Name == machinePoolName {
				machinePool = mp
				break
			}
		}
		if machinePool == nil {
			log.Info("Waiting for MachinePool Controller to set OwnerRef on AzureManagedMachinePool")
			// TODO: this error isn't very accurate, but it has some bearing on control flow
			// that matches what we want here, which is to exit reconciliation early and requeue.
			return nil, NoAzureManagedMachinePoolsErr
		}

		resources, err := ApplyMutators(ctx, azureManagedMachinePool.Spec.Resources,
			SetAgentPoolDefaults(ctrlClient, ptr.To(azureManagedMachinePool), machinePool),
		)
		if err != nil {
			return nil, err
		}

		for _, u := range resources {
			if u.GroupVersionKind().Group != asocontainerservicev1.GroupVersion.Group ||
				u.GroupVersionKind().Kind != "ManagedClustersAgentPool" {
				continue
			}

			agentPool, err := ctrlClient.Scheme().New(u.GroupVersionKind())
			if err != nil {
				return nil, fmt.Errorf("error creating new %v: %w", u.GroupVersionKind(), err)
			}
			err = ctrlClient.Scheme().Convert(u, agentPool, nil)
			if err != nil {
				return nil, err
			}

			agentPools = append(agentPools, agentPool.(conversion.Convertible))
			break
		}
	}

	return agentPools, nil
}

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
	"fmt"
	"strings"

	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20231001"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "sigs.k8s.io/cluster-api-provider-azure/v2/api/v2alpha1"
)

const ReplicasManagedByValue = "aks"

var NoManagedClustersAgentPoolDefined = fmt.Errorf("no %s ManagedClustersAgentPools defined in AzureManagedMachinePool spec.resources", asocontainerservicev1.GroupVersion.Group)

func SetAgentPoolDefaults(ctrlClient client.Client, azureManagedMachinePool *infrav1.AzureManagedMachinePool, machinePool *expv1.MachinePool) ResourcesMutator {
	return func(ctx context.Context, us []*unstructured.Unstructured) error {
		log := ctrl.LoggerFrom(ctx)

		var agentPool *unstructured.Unstructured
		var agentPoolPath string
		for i, u := range us {
			if u.GroupVersionKind().Group == asocontainerservicev1.GroupVersion.Group &&
				u.GroupVersionKind().Kind == "ManagedClustersAgentPool" {
				agentPool = u
				agentPoolPath = fmt.Sprintf("spec.resources[%d]", i) // TODO: in clusterclass this should be spec.template...
				break
			}
		}

		// Users may choose not to set the version in the MachinePool.
		if machinePool.Spec.Template.Spec.Version != nil {
			k8sVersionPath := []string{"spec", "orchestratorVersion"}
			capzK8sVersion := strings.TrimPrefix(*machinePool.Spec.Template.Spec.Version, "v")
			userK8sVersion, k8sVersionFound, err := unstructured.NestedString(agentPool.UnstructuredContent(), k8sVersionPath...)
			if err != nil {
				return err
			}

			setK8sVersion := mutation{
				location: agentPoolPath + "." + strings.Join(k8sVersionPath, "."),
				val:      capzK8sVersion,
				reason:   fmt.Sprintf("because MachinePool %s's spec.template.spec.version is %s", machinePool.Name, *machinePool.Spec.Template.Spec.Version),
			}

			if k8sVersionFound && userK8sVersion != capzK8sVersion {
				return Incompatible{
					mutation: setK8sVersion,
					userVal:  userK8sVersion,
				}
			}

			logMutation(log, setK8sVersion)
			err = unstructured.SetNestedField(agentPool.UnstructuredContent(), capzK8sVersion, k8sVersionPath...)
			if err != nil {
				return err
			}
		}

		autoscaling, _, err := unstructured.NestedBool(agentPool.UnstructuredContent(), "spec", "enableAutoScaling")
		if err != nil {
			return err
		}

		if !autoscaling {
			countPath := []string{"spec", "count"}
			capzCount := int64(ptr.Deref(machinePool.Spec.Replicas, 1))
			userCount, countFound, err := unstructured.NestedInt64(agentPool.UnstructuredContent(), countPath...)
			if err != nil {
				return err
			}

			setCount := mutation{
				location: agentPoolPath + "." + strings.Join(countPath, "."),
				val:      capzCount,
				reason:   fmt.Sprintf("MachinePool %s's spec.replicas takes precedence", machinePool.Name),
			}

			if countFound && userCount != capzCount {
				return Incompatible{
					mutation: setCount,
					userVal:  userCount,
				}
			}

			logMutation(log, setCount)
			err = unstructured.SetNestedField(agentPool.UnstructuredContent(), capzCount, countPath...)
			if err != nil {
				return err
			}
			// TODO: double-check if if we need to explicitly set count to null when autoscaling is enabled.
		}

		// Update the MachinePool replica manager annotation. This isn't wrapped in a mutation object because
		// it's not modifying an ASO resource and users are not expected to set this manually. This behavior
		// is documented by CAPI as expected of a provider.
		replicaManager, hasReplicaManager := machinePool.Annotations[clusterv1.ReplicasManagedByAnnotation]
		if autoscaling && !hasReplicaManager {
			if machinePool.Annotations == nil {
				machinePool.Annotations = make(map[string]string)
			}
			// TODO: do we need to patch the MachinePool in the AzureManagedControlPlane reconciliation? Or is the
			// first AzureManagedMachinePool reconciliation doing it enough?

			// TODO: should we give some feedback if the MachinePool replicas are already being managed, but
			// by something else?

			machinePool.Annotations[clusterv1.ReplicasManagedByAnnotation] = ReplicasManagedByValue
		} else if !autoscaling && replicaManager == ReplicasManagedByValue {
			delete(machinePool.Annotations, clusterv1.ReplicasManagedByAnnotation)
		}

		return nil
	}
}

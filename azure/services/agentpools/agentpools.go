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

package agentpools

import (
	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20230201"
	"k8s.io/utils/ptr"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/aso"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const serviceName = "agentpools"

// AgentPoolScope defines the scope interface for an agent pool.
type AgentPoolScope interface {
	azure.ClusterDescriber
	aso.Scope

	Name() string
	NodeResourceGroup() string
	AgentPoolSpec() azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedClustersAgentPool]
	SetAgentPoolProviderIDList([]string)
	SetAgentPoolReplicas(int32)
	SetAgentPoolReady(bool)
	SetCAPIMachinePoolReplicas(replicas *int)
	SetCAPIMachinePoolAnnotation(key, value string)
	RemoveCAPIMachinePoolAnnotation(key string)
	SetSubnetName()
}

// Service provides operations on Azure resources.
type Service struct {
	scope AgentPoolScope
	*aso.Service[*asocontainerservicev1.ManagedClustersAgentPool, AgentPoolScope]
}

// New creates a new service.
func New(scope AgentPoolScope) *Service {
	svc := aso.NewService[*asocontainerservicev1.ManagedClustersAgentPool](serviceName, scope)
	svc.Specs = []azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedClustersAgentPool]{scope.AgentPoolSpec()}
	svc.ConditionType = infrav1.AgentPoolsReadyCondition
	svc.PostCreateOrUpdateResourceHook = postCreateOrUpdateResourceHook
	return &Service{
		scope:   scope,
		Service: svc,
	}
}

// Name returns the service name.
func (s *Service) Name() string {
	return serviceName
}

func postCreateOrUpdateResourceHook(scope AgentPoolScope, agentPool *asocontainerservicev1.ManagedClustersAgentPool, err error) {
	if err != nil {
		return
	}
	// When autoscaling is set, add the annotation to the machine pool and update the replica count.
	if ptr.Deref(agentPool.Status.EnableAutoScaling, false) {
		scope.SetCAPIMachinePoolAnnotation(clusterv1.ReplicasManagedByAnnotation, "true")
		scope.SetCAPIMachinePoolReplicas(agentPool.Status.Count)
	} else { // Otherwise, remove the annotation.
		scope.RemoveCAPIMachinePoolAnnotation(clusterv1.ReplicasManagedByAnnotation)
	}
}

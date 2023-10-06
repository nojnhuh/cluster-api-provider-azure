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

package agentpools

import (
	"context"
	"reflect"
	"testing"

	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20230201"
	"github.com/Azure/azure-service-operator/v2/pkg/genruntime"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	azureutil "sigs.k8s.io/cluster-api-provider-azure/util/azure"
)

func fakeAgentPool(changes ...func(*AgentPoolSpec)) AgentPoolSpec {
	pool := AgentPoolSpec{
		Name:              "fake-agent-pool-name",
		AzureName:         "fakename",
		ResourceGroup:     "fake-rg",
		Cluster:           "fake-cluster",
		AvailabilityZones: []string{"fake-zone"},
		EnableAutoScaling: true,
		EnableUltraSSD:    ptr.To(true),
		KubeletDiskType:   (*infrav1.KubeletDiskType)(ptr.To("fake-kubelet-disk-type")),
		MaxCount:          ptr.To(5),
		MaxPods:           ptr.To(10),
		MinCount:          ptr.To(1),
		Mode:              "fake-mode",
		NodeLabels:        map[string]string{"fake-label": "fake-value"},
		NodeTaints:        []string{"fake-taint"},
		OSDiskSizeGB:      2,
		OsDiskType:        ptr.To("fake-os-disk-type"),
		OSType:            ptr.To("fake-os-type"),
		Replicas:          1,
		SKU:               "fake-sku",
		Version:           ptr.To("fake-version"),
		VnetSubnetID:      "fake-vnet-subnet-id",
		AdditionalTags:    infrav1.Tags{"fake": "tag"},
	}

	for _, change := range changes {
		change(&pool)
	}

	return pool
}

func withReplicas(replicas int) func(*AgentPoolSpec) {
	return func(pool *AgentPoolSpec) {
		pool.Replicas = replicas
	}
}

func withAutoscaling(enabled bool) func(*AgentPoolSpec) {
	return func(pool *AgentPoolSpec) {
		pool.EnableAutoScaling = enabled
	}
}

func withSpotMaxPrice(spotMaxPrice string) func(*AgentPoolSpec) {
	quantity := resource.MustParse(spotMaxPrice)
	return func(pool *AgentPoolSpec) {
		pool.SpotMaxPrice = &quantity
	}
}
func sdkFakeAgentPool(changes ...func(*asocontainerservicev1.ManagedClustersAgentPool)) *asocontainerservicev1.ManagedClustersAgentPool {
	pool := &asocontainerservicev1.ManagedClustersAgentPool{
		Spec: asocontainerservicev1.ManagedClusters_AgentPool_Spec{
			AvailabilityZones:   []string{"fake-zone"},
			AzureName:           "fakename",
			Count:               ptr.To(1),
			EnableAutoScaling:   ptr.To(true),
			EnableUltraSSD:      ptr.To(true),
			KubeletDiskType:     ptr.To(asocontainerservicev1.KubeletDiskType("fake-kubelet-disk-type")),
			MaxCount:            ptr.To(5),
			MaxPods:             ptr.To(10),
			MinCount:            ptr.To(1),
			Mode:                ptr.To(asocontainerservicev1.AgentPoolMode("fake-mode")),
			NodeLabels:          map[string]string{"fake-label": "fake-value"},
			NodeTaints:          []string{"fake-taint"},
			OrchestratorVersion: ptr.To("fake-version"),
			OsDiskSizeGB:        ptr.To[asocontainerservicev1.ContainerServiceOSDisk](2),
			OsDiskType:          ptr.To(asocontainerservicev1.OSDiskType("fake-os-disk-type")),
			OsType:              ptr.To(asocontainerservicev1.OSType("fake-os-type")),
			Owner:               &genruntime.KnownResourceReference{Name: "fake-cluster"},
			Tags:                map[string]string{"fake": "tag"},
			Type:                ptr.To(asocontainerservicev1.AgentPoolType_VirtualMachineScaleSets),
			VmSize:              ptr.To("fake-sku"),
			VnetSubnetReference: &genruntime.ResourceReference{
				ARMID: "fake-vnet-subnet-id",
			},
		},
	}

	for _, change := range changes {
		change(pool)
	}

	return pool
}

func sdkWithAutoscaling(enableAutoscaling bool) func(*asocontainerservicev1.ManagedClustersAgentPool) {
	return func(pool *asocontainerservicev1.ManagedClustersAgentPool) {
		pool.Spec.EnableAutoScaling = ptr.To(enableAutoscaling)
	}
}

func sdkWithCount(count int) func(*asocontainerservicev1.ManagedClustersAgentPool) {
	return func(pool *asocontainerservicev1.ManagedClustersAgentPool) {
		pool.Spec.Count = ptr.To(count)
	}
}

func sdkWithScaleDownMode(scaleDownMode asocontainerservicev1.ScaleDownMode) func(*asocontainerservicev1.ManagedClustersAgentPool) {
	return func(pool *asocontainerservicev1.ManagedClustersAgentPool) {
		if scaleDownMode == "" {
			pool.Spec.ScaleDownMode = nil
		} else {
			pool.Spec.ScaleDownMode = ptr.To(scaleDownMode)
		}
	}
}

func sdkWithSpotMaxPrice(spotMaxPrice float64) func(*asocontainerservicev1.ManagedClustersAgentPool) {
	return func(pool *asocontainerservicev1.ManagedClustersAgentPool) {
		pool.Spec.SpotMaxPrice = &spotMaxPrice
	}
}

func sdkWithNodeTaints(nodeTaints []string) func(*asocontainerservicev1.ManagedClustersAgentPool) {
	return func(pool *asocontainerservicev1.ManagedClustersAgentPool) {
		pool.Spec.NodeTaints = nodeTaints
	}
}

func TestParameters(t *testing.T) {
	testcases := []struct {
		name           string
		spec           AgentPoolSpec
		existing       *asocontainerservicev1.ManagedClustersAgentPool
		expected       *asocontainerservicev1.ManagedClustersAgentPool
		expectNoChange bool
		expectedError  error
	}{
		{
			name:          "parameters without an existing agent pool",
			spec:          fakeAgentPool(),
			existing:      nil,
			expected:      sdkFakeAgentPool(),
			expectedError: nil,
		},
		{
			name: "parameters with an existing agent pool, update when count is out of date when enableAutoScaling is false",
			spec: fakeAgentPool(),
			existing: sdkFakeAgentPool(
				sdkWithAutoscaling(false),
				sdkWithCount(5),
			),
			expected:      sdkFakeAgentPool(),
			expectedError: nil,
		},
		{
			name: "parameters with an existing agent pool, do not update when count is out of date and enableAutoScaling is true",
			spec: fakeAgentPool(),
			existing: sdkFakeAgentPool(
				sdkWithAutoscaling(true),
				sdkWithCount(5),
			),
			expectNoChange: true,
			expectedError:  nil,
		},
		{
			name: "parameters with an existing agent pool and update needed on autoscaling",
			spec: fakeAgentPool(),
			existing: sdkFakeAgentPool(
				sdkWithAutoscaling(false),
			),
			expected:      sdkFakeAgentPool(),
			expectedError: nil,
		},
		{
			name: "parameters with an existing agent pool and update needed on scale down mode",
			spec: fakeAgentPool(),
			existing: sdkFakeAgentPool(
				sdkWithScaleDownMode(asocontainerservicev1.ScaleDownMode_Deallocate),
			),
			expected:      sdkFakeAgentPool(),
			expectedError: nil,
		},
		{
			name: "parameters with an existing agent pool and update needed on spot max price",
			spec: fakeAgentPool(),
			existing: sdkFakeAgentPool(
				sdkWithSpotMaxPrice(123.456),
			),
			expected:      sdkFakeAgentPool(),
			expectedError: nil,
		},
		{
			name: "parameters with an existing agent pool and update needed on spot max price",
			spec: fakeAgentPool(
				withSpotMaxPrice("789.12345"),
			),
			existing: sdkFakeAgentPool(),
			expected: sdkFakeAgentPool(
				sdkWithSpotMaxPrice(789.12345),
			),
			expectedError: nil,
		},
		{
			name: "parameters with an existing agent pool and update needed on max count",
			spec: fakeAgentPool(),
			existing: sdkFakeAgentPool(
				func(pool *asocontainerservicev1.ManagedClustersAgentPool) {
					pool.Spec.MaxCount = ptr.To(3)
				},
			),
			expected:      sdkFakeAgentPool(),
			expectedError: nil,
		},
		{
			name: "parameters with an existing agent pool and update needed on min count",
			spec: fakeAgentPool(),
			existing: sdkFakeAgentPool(
				func(pool *asocontainerservicev1.ManagedClustersAgentPool) {
					pool.Spec.MinCount = ptr.To(3)
				},
			),
			expected:      sdkFakeAgentPool(),
			expectedError: nil,
		},
		{
			name: "parameters with an existing agent pool and update needed on mode",
			spec: fakeAgentPool(),
			existing: sdkFakeAgentPool(
				func(pool *asocontainerservicev1.ManagedClustersAgentPool) {
					pool.Spec.Mode = ptr.To(asocontainerservicev1.AgentPoolMode("fake-old-mode"))
				},
			),
			expected:      sdkFakeAgentPool(),
			expectedError: nil,
		},
		{
			name: "parameters with an existing agent pool and update needed on node labels",
			spec: fakeAgentPool(),
			existing: sdkFakeAgentPool(
				func(pool *asocontainerservicev1.ManagedClustersAgentPool) {
					pool.Spec.NodeLabels = map[string]string{
						"fake-label":     "fake-value",
						"fake-old-label": "fake-old-value",
					}
				},
			),
			expected:      sdkFakeAgentPool(),
			expectedError: nil,
		},
		{
			name: "difference in system node labels shouldn't trigger update",
			spec: fakeAgentPool(
				func(pool *AgentPoolSpec) {
					pool.NodeLabels = map[string]string{
						"fake-label": "fake-value",
					}
				},
			),
			existing: sdkFakeAgentPool(
				func(pool *asocontainerservicev1.ManagedClustersAgentPool) {
					pool.Spec.NodeLabels = map[string]string{
						"fake-label":                            "fake-value",
						"kubernetes.azure.com/scalesetpriority": "spot",
					}
				},
			),
			expectNoChange: true,
			expectedError:  nil,
		},
		{
			name: "difference in system node labels with empty labels shouldn't trigger update",
			spec: fakeAgentPool(
				func(pool *AgentPoolSpec) {
					pool.NodeLabels = nil
				},
			),
			existing: sdkFakeAgentPool(
				func(pool *asocontainerservicev1.ManagedClustersAgentPool) {
					pool.Spec.NodeLabels = map[string]string{
						"kubernetes.azure.com/scalesetpriority": "spot",
					}
				},
			),
			expectNoChange: true,
			expectedError:  nil,
		},
		{
			name: "difference in non-system node taints with empty taints should trigger update",
			spec: fakeAgentPool(
				func(pool *AgentPoolSpec) {
					pool.NodeTaints = nil
				},
			),
			existing: sdkFakeAgentPool(
				sdkWithNodeTaints([]string{"fake-taint"}),
			),
			expected:      sdkFakeAgentPool(sdkWithNodeTaints(nil)),
			expectedError: nil,
		},
		{
			name: "difference in system node taints with empty taints shouldn't trigger update",
			spec: fakeAgentPool(
				func(pool *AgentPoolSpec) {
					pool.NodeTaints = nil
				},
			),
			existing: sdkFakeAgentPool(
				sdkWithNodeTaints([]string{azureutil.AzureSystemNodeLabelPrefix + "-fake-taint"}),
			),
			expectNoChange: true,
			expectedError:  nil,
		},
		{
			name: "parameters with an existing agent pool and update needed on node taints",
			spec: fakeAgentPool(),
			existing: sdkFakeAgentPool(
				func(pool *asocontainerservicev1.ManagedClustersAgentPool) {
					pool.Spec.NodeTaints = []string{"fake-old-taint"}
				},
			),
			expected:      sdkFakeAgentPool(),
			expectedError: nil,
		},
		{
			name: "scale to zero",
			spec: fakeAgentPool(
				withReplicas(0),
				withAutoscaling(false),
			),
			existing: sdkFakeAgentPool(
				sdkWithAutoscaling(false),
				sdkWithCount(1),
			),
			expected: sdkFakeAgentPool(
				sdkWithAutoscaling(false),
				sdkWithCount(0),
			),
			expectedError: nil,
		},
		{
			name: "empty node taints should not trigger an update",
			spec: fakeAgentPool(
				func(pool *AgentPoolSpec) { pool.NodeTaints = nil },
			),
			existing: sdkFakeAgentPool(
				func(pool *asocontainerservicev1.ManagedClustersAgentPool) { pool.Spec.NodeTaints = nil },
			),
			expectNoChange: true,
			expectedError:  nil,
		},
	}
	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			t.Parallel()

			result, err := tc.spec.Parameters(context.TODO(), tc.existing)
			if tc.expectedError != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(MatchError(tc.expectedError))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			if tc.expectNoChange {
				tc.expected = tc.existing
			}
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Got difference between expected result and computed result:\n%s", cmp.Diff(tc.expected, result))
			}
		})
	}
}

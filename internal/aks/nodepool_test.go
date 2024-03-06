package aks

import (
	"testing"

	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20231001"
	asocontainerservicev1preview "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20231102preview"
	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func TestSetAgentPoolProfilesFromAgentPools(t *testing.T) {
	t.Run("stable with no pools succeeds", func(t *testing.T) {
		mc := &asocontainerservicev1.ManagedCluster{}
		var pools []*asocontainerservicev1.ManagedClustersAgentPool
		var expected []asocontainerservicev1.ManagedClusterAgentPoolProfile

		err := SetAgentPoolProfilesFromAgentPools(mc, pools)
		if err != nil {
			t.Fatal("expected success, got err", err)
		}
		if diff := cmp.Diff(expected, mc.Spec.AgentPoolProfiles); diff != "" {
			t.Error(diff)
		}
	})

	t.Run("stable with pools", func(t *testing.T) {
		mc := &asocontainerservicev1.ManagedCluster{}
		pools := []conversion.Convertible{
			&asocontainerservicev1.ManagedClustersAgentPool{
				Spec: asocontainerservicev1.ManagedClusters_AgentPool_Spec{
					AzureName: "pool0",
					MaxCount:  ptr.To(1),
				},
			},
			// Not all pools have to be the same version, or the same version as the cluster.
			&asocontainerservicev1preview.ManagedClustersAgentPool{
				Spec: asocontainerservicev1preview.ManagedClusters_AgentPool_Spec{
					AzureName:           "pool1",
					MinCount:            ptr.To(2),
					EnableCustomCATrust: ptr.To(true),
				},
			},
		}
		expected := []asocontainerservicev1.ManagedClusterAgentPoolProfile{
			{
				Name:     ptr.To("pool0"),
				MaxCount: ptr.To(1),
			},
			{
				Name:     ptr.To("pool1"),
				MinCount: ptr.To(2),
				// EnableCustomCATrust is a preview-only feature that can't be represented here, so it should
				// be lost.
				// TODO: validate this up front?
			},
		}

		err := SetAgentPoolProfilesFromAgentPools(mc, pools)
		if err != nil {
			t.Fatal("expected success, got err", err)
		}
		if diff := cmp.Diff(expected, mc.Spec.AgentPoolProfiles); diff != "" {
			t.Error(diff)
		}
	})

	t.Run("preview with pools", func(t *testing.T) {
		mc := &asocontainerservicev1preview.ManagedCluster{}
		pools := []conversion.Convertible{
			&asocontainerservicev1.ManagedClustersAgentPool{
				Spec: asocontainerservicev1.ManagedClusters_AgentPool_Spec{
					AzureName: "pool0",
					MaxCount:  ptr.To(1),
				},
			},
			&asocontainerservicev1preview.ManagedClustersAgentPool{
				Spec: asocontainerservicev1preview.ManagedClusters_AgentPool_Spec{
					AzureName:           "pool1",
					MinCount:            ptr.To(2),
					EnableCustomCATrust: ptr.To(true),
				},
			},
		}
		expected := []asocontainerservicev1preview.ManagedClusterAgentPoolProfile{
			{
				Name:     ptr.To("pool0"),
				MaxCount: ptr.To(1),
			},
			{
				Name:                ptr.To("pool1"),
				MinCount:            ptr.To(2),
				EnableCustomCATrust: ptr.To(true),
			},
		}

		err := SetAgentPoolProfilesFromAgentPools(mc, pools)
		if err != nil {
			t.Fatal("expected success, got err", err)
		}
		if diff := cmp.Diff(expected, mc.Spec.AgentPoolProfiles); diff != "" {
			t.Error(diff)
		}
	})
}

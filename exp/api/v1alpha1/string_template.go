/*
Copyright 2025 The Kubernetes Authors.

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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// AzureASOClusterJSONPatchValueFromTemplateData returns the data passed to [JSONPatchValueFrom] templates for AzureASOClusters.
func AzureASOClusterJSONPatchValueFromTemplateData(self *AzureASOCluster, cluster *clusterv1.Cluster) (any, error) {
	objs := make(map[string]any)
	if self != nil {
		objs["selfV1alpha1"] = self
	}
	if cluster != nil {
		objs["clusterV1beta2"] = cluster
	}
	return buildTemplateData(objs)
}

func buildTemplateData(objs map[string]any) (any, error) {
	data := map[string]any{}

	for name, obj := range objs {
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return nil, err
		}
		data[name] = u
	}

	return data, nil
}

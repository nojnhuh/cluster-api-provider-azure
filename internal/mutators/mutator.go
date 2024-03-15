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

	asoannotations "github.com/Azure/azure-service-operator/v2/pkg/common/annotations"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// ResourcesMutator mutates in-place a slice of ASO resources to be reconciled. These mutations make only the
// changes strictly necessary for CAPZ resources to play nice with Cluster API. Any mutations should be logged
// and mutations that conflict with user-defined values should be rejected by returning Incompatible.
type ResourcesMutator func(context.Context, []*unstructured.Unstructured) error

type mutation struct {
	location string
	val      any
	reason   string
}

func logMutation(log logr.Logger, mutation mutation) {
	log.Info(fmt.Sprintf("setting %s to %v %s", mutation.location, mutation.val, mutation.reason))
}

// Incompatible describes an error where a piece of user-defined configuration does not match what CAPZ
// requires.
type Incompatible struct {
	mutation
	userVal any
}

func (e Incompatible) Error() string {
	return fmt.Sprintf("incompatible value: value at %s set by user to %v but CAPZ must set it to %v %s. The user-defined value must not be defined, or must match CAPZ's desired value.", e.location, e.userVal, e.val, e.reason)
}

func ApplyMutators(ctx context.Context, resources []runtime.RawExtension, mutators ...ResourcesMutator) ([]*unstructured.Unstructured, error) {
	us := []*unstructured.Unstructured{}
	for _, resource := range resources {
		u := &unstructured.Unstructured{}
		if err := u.UnmarshalJSON(resource.Raw); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resource JSON: %w", err)
		}
		us = append(us, u)
	}
	for _, mutator := range mutators {
		if err := mutator(ctx, us); err != nil {
			return nil, fmt.Errorf("failed to run mutator: %w", err)
		}
	}
	return us, nil
}

// SetASOReconciliationPolicy returns a mutator that sets the ASO reconcile policy to "skip" on all
// resources if the cluster is paused.
func SetASOReconciliationPolicy(cluster *clusterv1.Cluster) ResourcesMutator {
	return func(ctx context.Context, us []*unstructured.Unstructured) error {
		log := ctrl.LoggerFrom(ctx)

		if !cluster.Spec.Paused {
			return nil
		}

		for _, u := range us {
			annotations := u.GetAnnotations()
			if annotations == nil {
				annotations = make(map[string]string)
			}
			userReconcilePolicy, exists := annotations[asoannotations.ReconcilePolicy]
			capzReconcilePolicy := string(asoannotations.ReconcilePolicySkip)

			setSkip := mutation{
				location: fmt.Sprintf("metadata.annotations[%q]", asoannotations.ReconcilePolicy),
				val:      capzReconcilePolicy,
				reason:   "because Cluster " + cluster.Name + " is paused",
			}

			if exists && userReconcilePolicy != capzReconcilePolicy {
				return Incompatible{
					mutation: setSkip,
					userVal:  userReconcilePolicy,
				}
			}

			annotations[asoannotations.ReconcilePolicy] = capzReconcilePolicy
			logMutation(log, setSkip)
			u.SetAnnotations(annotations)
		}
		return nil
	}
}

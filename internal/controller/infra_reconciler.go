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

package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-service-operator/v2/pkg/genruntime/conditions"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
)

var waitingForASOReconcileMsg = "Waiting for ASO to reconcile the resource"

type InfraReconciler struct {
	client.Client
	resources []*unstructured.Unstructured
	owner     resourceStatusObject
	watcher   watcher
}

type watcher interface {
	Watch(log logr.Logger, obj runtime.Object, handler handler.EventHandler, p ...predicate.Predicate) error
}

type resourceStatusObject interface {
	client.Object
	GetResourceStatuses() []infrav1.ResourceStatus
	SetResourceStatuses([]infrav1.ResourceStatus)
}

// Reconcile creates or updates the specified resources.
func (r *InfraReconciler) Reconcile(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("reconciling resources")

	newStatuses := r.owner.GetResourceStatuses()

	type reconcileKey struct {
		group string
		kind  string
		// version may change throughout the lifetime of a resource
		name string
	}
	reconciled := map[reconcileKey]struct{}{}

	for _, spec := range r.resources {
		spec.SetNamespace(r.owner.GetNamespace())
		gvk := spec.GroupVersionKind()

		log := log.WithValues("resource", klog.KObj(spec), "resourceVersion", gvk.GroupVersion(), "resourceKind", gvk.Kind)

		if err := controllerutil.SetControllerReference(r.owner, spec, r.Scheme()); err != nil {
			return fmt.Errorf("failed to set owner reference: %w", err)
		}

		if err := r.watcher.Watch(log, spec, handler.EnqueueRequestForOwner(r.Client.Scheme(), r.Client.RESTMapper(), r.owner)); err != nil {
			return fmt.Errorf("failed to watch resource: %w", err)
		}

		log.Info("applying resource")
		err := r.Patch(ctx, spec, client.Apply, client.FieldOwner("capz-manager"), client.ForceOwnership)
		if err != nil {
			return fmt.Errorf("failed to apply resource: %w", err)
		}

		ready, err := readyStatus(spec)
		if err != nil {
			return err
		}
		newStatus := infrav1.ResourceStatus{
			Resource: infrav1.StatusResource{
				Group:   gvk.Group,
				Version: gvk.Version,
				Kind:    gvk.Kind,
				Name:    spec.GetName(),
			},
			Condition: ready,
		}
		foundStatus := false
		for i, status := range r.owner.GetResourceStatuses() {
			if newStatus.Resource.Group == status.Resource.Group &&
				newStatus.Resource.Kind == status.Resource.Kind &&
				newStatus.Resource.Name == status.Resource.Name {
				newStatuses[i] = newStatus
				foundStatus = true
				break
			}
		}
		if !foundStatus {
			newStatuses = append(newStatuses, newStatus)
		}

		reconciled[reconcileKey{gvk.Group, gvk.Kind, spec.GetName()}] = struct{}{}
	}

	// filteredStatuses does not contain status for resources that have been deleted and are gone.
	var filteredStatuses []infrav1.ResourceStatus
	for _, status := range newStatuses {
		if _, reconciled := reconciled[reconcileKey{
			status.Resource.Group,
			status.Resource.Kind,
			status.Resource.Name,
		}]; reconciled {
			filteredStatuses = append(filteredStatuses, status)
			continue
		}
		d := &unstructured.Unstructured{}
		d.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   status.Resource.Group,
			Version: status.Resource.Version,
			Kind:    status.Resource.Kind,
		})
		d.SetName(status.Resource.Name)
		d.SetNamespace(r.owner.GetNamespace())
		log.Info("deleting resource")
		err := r.Client.Delete(ctx, d)
		if !apierrors.IsNotFound(err) {
			filteredStatuses = append(filteredStatuses, status)
			if err != nil {
				return fmt.Errorf("failed to delete resource: %w", err)
			}
		}
	}

	r.owner.SetResourceStatuses(filteredStatuses)

	return nil
}

func readyStatus(u *unstructured.Unstructured) (conditions.Condition, error) {
	statusConditions, found, err := unstructured.NestedSlice(u.Object, "status", "conditions")
	if !found || err != nil {
		return conditions.Condition{}, err
	}

	for _, el := range statusConditions {
		condition, ok := el.(map[string]interface{})
		if !ok {
			continue
		}
		condType, found, err := unstructured.NestedString(condition, "type")
		if !found || err != nil || condType != conditions.ConditionTypeReady {
			continue
		}

		observedGen, _, err := unstructured.NestedInt64(condition, "observedGeneration")
		if observedGen < u.GetGeneration() {
			return conditions.Condition{
				Type:               conditions.ConditionTypeReady,
				Status:             metav1.ConditionFalse,
				Severity:           conditions.ConditionSeverityInfo,
				LastTransitionTime: metav1.Time{},
				ObservedGeneration: observedGen,
				Reason:             "WaitingForASOReconcile",
				Message:            waitingForASOReconcileMsg,
			}, nil
		}

		condJSON, err := json.Marshal(condition)
		if err != nil {
			return conditions.Condition{}, err
		}
		asoCond := conditions.Condition{}
		err = json.Unmarshal(condJSON, &asoCond)
		if err != nil {
			return conditions.Condition{}, err
		}
		return asoCond, nil
	}

	return conditions.Condition{}, nil
}

// Delete deletes the specified resources.
func (r *InfraReconciler) Delete(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("deleting resources")

	var statuses []infrav1.ResourceStatus

	for _, spec := range r.resources {
		spec.SetNamespace(r.owner.GetNamespace())
		gvk := spec.GroupVersionKind()

		log := log.WithValues("resource", klog.KObj(spec), "resourceVersion", gvk.GroupVersion(), "resourceKind", gvk.Kind)

		log.Info("deleting resource")
		err := r.Client.Delete(ctx, spec)
		if apierrors.IsNotFound(err) {
			log.Info("resource has been deleted")
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to delete resource: %w", err)
		}
		err = r.Get(ctx, client.ObjectKeyFromObject(spec), spec)
		if apierrors.IsNotFound(err) {
			log.Info("resource has been deleted")
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to get status for resource being deleted: %w", err)
		}
		ready, err := readyStatus(spec)
		if err != nil {
			return err
		}
		statuses = append(statuses, infrav1.ResourceStatus{
			Resource: infrav1.StatusResource{
				Group:   gvk.Group,
				Version: gvk.Version,
				Kind:    gvk.Kind,
				Name:    spec.GetName(),
			},
			Condition: ready,
		})
	}

	r.owner.SetResourceStatuses(statuses)

	return nil
}

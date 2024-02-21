package controller

import (
	"context"
	"fmt"

	"github.com/Azure/azure-service-operator/v2/pkg/genruntime/conditions"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1 "github.com/nojnhuh/cluster-api-provider-aso/api/v1alpha1"
)

type InfraReconciler struct {
	client.Client
	resources       []runtime.RawExtension
	owner           resourceStatusObject
	externalTracker *external.ObjectTracker
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

	for _, resource := range r.resources {
		spec := &unstructured.Unstructured{}
		err := spec.UnmarshalJSON(resource.Raw)
		if err != nil {
			return err
		}
		spec.SetNamespace(r.owner.GetNamespace())
		gvk := spec.GroupVersionKind()

		log := log.WithValues("resource", klog.KObj(spec), "resourceVersion", gvk.GroupVersion(), "resourceKind", gvk.Kind)

		if err := controllerutil.SetControllerReference(r.owner, spec, r.Scheme()); err != nil {
			return fmt.Errorf("failed to set owner reference: %w", err)
		}

		if err := r.externalTracker.Watch(log, spec, handler.EnqueueRequestForOwner(r.Client.Scheme(), r.Client.RESTMapper(), r.owner)); err != nil {
			return fmt.Errorf("failed to watch resource: %w", err)
		}

		log.Info("applying resource")
		err = r.Patch(ctx, spec, client.Apply, client.FieldOwner("capaso"), client.ForceOwnership)
		if err != nil {
			return fmt.Errorf("failed to apply resource: %w", err)
		}

		ready, message := readyStatus(spec)
		newStatus := infrav1.ResourceStatus{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind,
			Name:    spec.GetName(),
			Ready:   ready,
			Message: message,
		}
		foundStatus := false
		for i, status := range r.owner.GetResourceStatuses() {
			if newStatus.Group == status.Group &&
				newStatus.Kind == status.Kind &&
				newStatus.Name == status.Name {
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
		if _, reconciled := reconciled[reconcileKey{status.Group, status.Kind, status.Name}]; reconciled {
			filteredStatuses = append(filteredStatuses, status)
			continue
		}
		d := &unstructured.Unstructured{}
		d.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   status.Group,
			Version: status.Version,
			Kind:    status.Kind,
		})
		d.SetName(status.Name)
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

func readyStatus(u *unstructured.Unstructured) (bool, string) {
	statusConditions, found, err := unstructured.NestedSlice(u.Object, "status", "conditions")
	if !found || err != nil {
		return false, ""
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
			return false, "the resource is being reconciled"
		}

		status, found, err := unstructured.NestedString(condition, "status")
		if !found || err != nil {
			continue
		}
		ready := false
		if status == string(metav1.ConditionTrue) {
			ready = true
		}

		// message might not always be defined
		message, _, err := unstructured.NestedString(condition, "message")
		if err != nil {
			continue
		}

		return ready, message
	}

	return false, ""
}

// Delete deletes the specified resources.
func (r *InfraReconciler) Delete(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("deleting resources")

	var statuses []infrav1.ResourceStatus

	for _, resource := range r.resources {
		spec := &unstructured.Unstructured{}
		err := spec.UnmarshalJSON(resource.Raw)
		if err != nil {
			return err
		}
		spec.SetNamespace(r.owner.GetNamespace())
		gvk := spec.GroupVersionKind()

		err = r.Client.Delete(ctx, spec)
		if !apierrors.IsNotFound(err) {
			if err != nil {
				return fmt.Errorf("failed to delete resource: %w", err)
			}
			err := r.Get(ctx, client.ObjectKeyFromObject(spec), spec)
			if apierrors.IsNotFound(err) {
				// object has been deleted
				continue
			}
			if err != nil {
				return fmt.Errorf("failed to get status for resource being deleted: %w", err)
			}
			ready, message := readyStatus(spec)
			statuses = append(statuses, infrav1.ResourceStatus{
				Group:   gvk.Group,
				Version: gvk.Version,
				Kind:    gvk.Kind,
				Name:    spec.GetName(),
				Ready:   ready,
				Message: message,
			})
		}
	}

	r.owner.SetResourceStatuses(statuses)

	return nil
}

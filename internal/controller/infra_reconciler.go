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
	"k8s.io/utils/ptr"
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
			return fmt.Errorf("failed to apply owner reference: %w", err)
		}

		if err := r.externalTracker.Watch(log, spec, handler.EnqueueRequestForOwner(r.Client.Scheme(), r.Client.RESTMapper(), r.owner)); err != nil {
			return fmt.Errorf("failed to watch resource: %w", err)
		}

		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(spec.GetObjectKind().GroupVersionKind())
		err = r.Get(ctx, client.ObjectKeyFromObject(spec), u)
		if apierrors.IsNotFound(err) {
			u = spec
			log.Info("creating resource")
			err = r.Create(ctx, u)
		} else if err == nil {
			// TODO: also update labels and annotations, but make sure not to delete ones that already exist
			// since they might be managed by something else like ASO.
			u.Object["spec"] = spec.Object["spec"]
			log.Info("updating resource")
			err = r.Update(ctx, u)
		}
		if err != nil {
			return fmt.Errorf("failed to create or update object: %w", err)
		}

		ready, message := readyStatus(u)
		newStatus := infrav1.ResourceStatus{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind,
			Name:    u.GetName(),
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

		reconciled[reconcileKey{gvk.Group, gvk.Kind, u.GetName()}] = struct{}{}
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

func readyStatus(u *unstructured.Unstructured) (*bool, string) {
	statusConditions, found, err := unstructured.NestedSlice(u.Object, "status", "conditions")
	if !found || err != nil {
		return nil, ""
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
		status, found, err := unstructured.NestedString(condition, "status")
		if !found || err != nil {
			continue
		}
		var ready *bool
		if status == string(metav1.ConditionTrue) {
			ready = ptr.To(true)
		}
		if status == string(metav1.ConditionFalse) {
			ready = ptr.To(false)
		}
		// message might not always be defined
		message, _, err := unstructured.NestedString(condition, "message")
		if err != nil {
			continue
		}

		return ready, message
	}

	return nil, ""
}

// Delete deletes the specified resources.
func (r *InfraReconciler) Delete(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("deleting resources")

	var statuses []infrav1.ResourceStatus

	for _, resource := range r.resources {
		u := &unstructured.Unstructured{}
		err := u.UnmarshalJSON(resource.Raw)
		if err != nil {
			return err
		}
		u.SetNamespace(r.owner.GetNamespace())
		gvk := u.GroupVersionKind()

		err = r.Client.Delete(ctx, u)
		if !apierrors.IsNotFound(err) {
			if err != nil {
				return fmt.Errorf("failed to delete resource: %w", err)
			}
			err := r.Get(ctx, client.ObjectKeyFromObject(u), u)
			if apierrors.IsNotFound(err) {
				// object has been deleted
				continue
			}
			if err != nil {
				return fmt.Errorf("failed to get status for deleted resource: %w", err)
			}
			ready, message := readyStatus(u)
			statuses = append(statuses, infrav1.ResourceStatus{
				Group:   gvk.Group,
				Version: gvk.Version,
				Kind:    gvk.Kind,
				Name:    u.GetName(),
				Ready:   ready,
				Message: message,
			})
		}
	}

	r.owner.SetResourceStatuses(statuses)

	return nil
}

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/Azure/azure-service-operator/v2/pkg/genruntime/conditions"
	infrastructurev1alpha1 "github.com/nojnhuh/cluster-api-provider-aso/api/v1alpha1"
)

type InfraReconciler struct {
	client.Client
	resources []runtime.RawExtension
	owner     resourceStatusObject
	// TODO: some status interface.
	// - applied resources (with ready conditions)
	// TODO: remote object tracker to watch applied resources.
}

type resourceStatusObject interface {
	client.Object
	GetResourceStatuses() []infrastructurev1alpha1.ResourceStatus
	SetResourceStatuses([]infrastructurev1alpha1.ResourceStatus)
}

// Reconcile creates or updates the specified resources.
func (r *InfraReconciler) Reconcile(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("reconciling resources")

	newStatuses := r.owner.GetResourceStatuses()

	for _, resource := range r.resources {
		spec := &unstructured.Unstructured{}
		err := spec.UnmarshalJSON(resource.Raw)
		if err != nil {
			return err
		}
		spec.SetNamespace(r.owner.GetNamespace())

		if err := controllerutil.SetControllerReference(r.owner, spec, r.Scheme()); err != nil {
			return fmt.Errorf("failed to apply owner reference: %w", err)
		}

		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(spec.GetObjectKind().GroupVersionKind())
		err = r.Get(ctx, client.ObjectKeyFromObject(spec), u)
		if apierrors.IsNotFound(err) {
			u = spec
			err = r.Create(ctx, u)
		} else if err == nil {
			// TODO: also update labels and annotations, but make sure not to delete ones that already exist
			// since they might be managed by something else like ASO.
			u.Object["spec"] = spec.Object["spec"]
			err = r.Update(ctx, u)
		}
		if err != nil {
			return fmt.Errorf("failed to create or update object: %w", err)
		}

		gvk := u.GroupVersionKind()
		newStatus := infrastructurev1alpha1.ResourceStatus{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind,
			Name:    u.GetName(),
		}
		ready, message := readyStatus(u)
		newStatus.Ready = ready
		newStatus.Message = message
		foundStatus := false
		for i, status := range r.owner.GetResourceStatuses() {
			if newStatus.Group == status.Group &&
				newStatus.Version == status.Version &&
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
	}

	r.owner.SetResourceStatuses(newStatuses)

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
	for _, resource := range r.resources {
		u := &unstructured.Unstructured{}
		err := u.UnmarshalJSON(resource.Raw)
		if err != nil {
			return err
		}
		u.SetNamespace(r.owner.GetNamespace())

		err = r.Client.Delete(ctx, u)
		if client.IgnoreNotFound(err) != nil {
			return err
		}
		// TODO: wait for objects to be deleted
	}

	return nil
}

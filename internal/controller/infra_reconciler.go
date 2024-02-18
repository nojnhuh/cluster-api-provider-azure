package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type InfraReconciler struct {
	client.Client
	resources []runtime.RawExtension
	owner     client.Object
	// TODO: some status interface.
	// - applied resources (with ready conditions)
	// TODO: remote object tracker to watch applied resources.
}

// Reconcile creates or updates the specified resources.
func (r *InfraReconciler) Reconcile(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("reconciling resources")

	for _, resource := range r.resources {
		u := &unstructured.Unstructured{}
		err := u.UnmarshalJSON(resource.Raw)
		if err != nil {
			return err
		}
		u.SetNamespace(r.owner.GetNamespace())

		existing := &unstructured.Unstructured{}
		existing.SetGroupVersionKind(u.GetObjectKind().GroupVersionKind())

		err = r.Get(ctx, client.ObjectKeyFromObject(u), existing)
		if apierrors.IsNotFound(err) {
			err = r.Create(ctx, u)
		} else if err == nil {
			// TODO: also update labels and annotations, but make sure not to delete ones that already exist
			// since they might be managed by something else like ASO.
			existing.Object["spec"] = u.Object["spec"]
			err = r.Update(ctx, existing)
		}
		if err != nil {
			return fmt.Errorf("failed to create or update object: %w", err)
		}
	}

	return nil
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

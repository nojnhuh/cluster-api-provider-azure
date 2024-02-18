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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1 "github.com/nojnhuh/cluster-api-provider-aso/api/v1alpha1"
)

// ASOManagedControlPlaneReconciler reconciles a ASOManagedControlPlane object
type ASOManagedControlPlaneReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	infraReconciler *InfraReconciler
	externalTracker *external.ObjectTracker
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=asomanagedcontrolplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=asomanagedcontrolplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=asomanagedcontrolplanes/finalizers,verbs=update

// Reconcile reconciles an ASOManagedControlPlane
func (r *ASOManagedControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, resultErr error) {
	asoControlPlane := &infrav1.ASOManagedControlPlane{}
	err := r.Get(ctx, req.NamespacedName, asoControlPlane)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(asoControlPlane, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper: %w", err)
	}
	defer func() {
		err := patchHelper.Patch(ctx, asoControlPlane)
		if !asoControlPlane.GetDeletionTimestamp().IsZero() {
			err = ignorePatchErrNotFound(err)
		}
		if err != nil && resultErr == nil {
			resultErr = err
			result = ctrl.Result{}
		}
	}()

	asoControlPlane.Status.ExternalManagedControlPlane = true

	cluster, err := util.GetOwnerCluster(ctx, r.Client, asoControlPlane.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.infraReconciler = &InfraReconciler{
		Client:          r.Client,
		resources:       asoControlPlane.Spec.Resources,
		owner:           asoControlPlane,
		externalTracker: r.externalTracker,
	}

	if !asoControlPlane.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, asoControlPlane, cluster)
	}

	return r.reconcileNormal(ctx, asoControlPlane, cluster)
}

func (r *ASOManagedControlPlaneReconciler) reconcileNormal(ctx context.Context, asoControlPlane *infrav1.ASOManagedControlPlane, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	if controllerutil.AddFinalizer(asoControlPlane, clusterv1.ClusterFinalizer) {
		return ctrl.Result{Requeue: true}, nil
	}

	if !cluster.Status.InfrastructureReady {
		log.Info("Cluster infrastructure is not ready")
		return ctrl.Result{}, nil
	}

	err := r.infraReconciler.Reconcile(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	asoControlPlane.Status.Ready = true
	for _, status := range asoControlPlane.GetResourceStatuses() {
		if !ptr.Deref(status.Ready, true) {
			asoControlPlane.Status.Ready = false
			break
		}
	}
	asoControlPlane.Status.Initialized = asoControlPlane.Status.Ready

	return ctrl.Result{}, nil
}

func (r *ASOManagedControlPlaneReconciler) reconcileDelete(ctx context.Context, asoControlPlane *infrav1.ASOManagedControlPlane, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	if cluster == nil {
		// Cluster owner ref not set
		controllerutil.RemoveFinalizer(asoControlPlane, clusterv1.ClusterFinalizer)
		return ctrl.Result{}, nil
	}

	err := r.infraReconciler.Delete(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(asoControlPlane.GetResourceStatuses()) > 0 {
		// waiting for resources to be deleted
		return ctrl.Result{}, nil
	}

	controllerutil.RemoveFinalizer(asoControlPlane, clusterv1.ClusterFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ASOManagedControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.ASOManagedControlPlane{}).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.ClusterToKubeadmControlPlane),
			builder.WithPredicates(
				// TODO: watch for changes in pause to pause ASO resources.
				predicates.ClusterUnpausedAndInfrastructureReady(log.FromContext(ctx)),
			),
		).
		Build(r)
	if err != nil {
		return err
	}
	r.externalTracker = &external.ObjectTracker{
		Cache:      mgr.GetCache(),
		Controller: c,
	}
	return nil
}

// ClusterToASOManagedControlPlane is a handler.ToRequestsFunc to be used to enqueue requests for
// reconciliation for ASOManagedControlPlane based on updates to a Cluster.
func (r *ASOManagedControlPlaneReconciler) ClusterToKubeadmControlPlane(_ context.Context, o client.Object) []ctrl.Request {
	c, ok := o.(*clusterv1.Cluster)
	if !ok {
		panic(fmt.Sprintf("Expected a Cluster but got a %T", o))
	}

	controlPlaneRef := c.Spec.ControlPlaneRef
	if controlPlaneRef != nil && controlPlaneRef.Kind == "ASOManagedControlPlane" {
		return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: controlPlaneRef.Namespace, Name: controlPlaneRef.Name}}}
	}

	return nil
}

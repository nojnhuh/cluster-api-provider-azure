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

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1 "github.com/nojnhuh/cluster-api-provider-aso/api/v1alpha1"
)

// ASOManagedClusterReconciler reconciles a ASOManagedCluster object
type ASOManagedClusterReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	controller      controller.Controller
	infraReconciler *InfraReconciler
	externalTracker *external.ObjectTracker
}

//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=resources.azure.com;containerservice.azure.com,resources=*,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=asomanagedclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=asomanagedclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=asomanagedclusters/finalizers,verbs=update

// Reconcile reconciles an ASOManagedCluster.
func (r *ASOManagedClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, resultErr error) {
	asoCluster := &infrav1.ASOManagedCluster{}
	err := r.Get(ctx, req.NamespacedName, asoCluster)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(asoCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper: %w", err)
	}
	defer func() {
		err := patchHelper.Patch(ctx, asoCluster)
		if !asoCluster.GetDeletionTimestamp().IsZero() {
			err = ignorePatchErrNotFound(err)
		}
		if err != nil && resultErr == nil {
			resultErr = err
			result = ctrl.Result{}
		}
	}()

	cluster, err := util.GetOwnerCluster(ctx, r.Client, asoCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.infraReconciler = &InfraReconciler{
		Client:          r.Client,
		resources:       asoCluster.Spec.Resources,
		owner:           asoCluster,
		externalTracker: r.externalTracker,
	}

	if !asoCluster.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, asoCluster, cluster)
	}

	return r.reconcileNormal(ctx, asoCluster, cluster)
}

func (r *ASOManagedClusterReconciler) reconcileNormal(ctx context.Context, asoCluster *infrav1.ASOManagedCluster, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	if controllerutil.AddFinalizer(asoCluster, clusterv1.ClusterFinalizer) {
		return ctrl.Result{Requeue: true}, nil
	}

	err := r.infraReconciler.Reconcile(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	asoControlPlane := &infrav1.ASOManagedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Spec.ControlPlaneRef.Namespace,
			Name:      cluster.Spec.ControlPlaneRef.Name,
		},
	}
	err = r.Get(ctx, client.ObjectKeyFromObject(asoControlPlane), asoControlPlane)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get ASOManagedControlPlane %s/%s: %w", asoControlPlane.Namespace, asoControlPlane.Name, err)
	}

	asoCluster.Spec.ControlPlaneEndpoint = asoControlPlane.Status.ControlPlaneEndpoint
	asoCluster.Status.Ready = true
	for _, status := range asoCluster.GetResourceStatuses() {
		if !ptr.Deref(status.Ready, true) {
			asoCluster.Status.Ready = false
			break
		}
	}

	return ctrl.Result{}, nil
}

func (r *ASOManagedClusterReconciler) reconcileDelete(ctx context.Context, asoCluster *infrav1.ASOManagedCluster, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	if cluster == nil {
		controllerutil.RemoveFinalizer(asoCluster, clusterv1.ClusterFinalizer)
		return ctrl.Result{}, nil
	}

	err := r.infraReconciler.Delete(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(asoCluster.GetResourceStatuses()) > 0 {
		// waiting for resources to be deleted
		return ctrl.Result{}, nil
	}

	controllerutil.RemoveFinalizer(asoCluster, clusterv1.ClusterFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ASOManagedClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, log logr.Logger) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.ASOManagedCluster{}).
		WithEventFilter(predicates.ResourceIsNotExternallyManaged(log)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(
				util.ClusterToInfrastructureMapFunc(ctx, infrav1.GroupVersion.WithKind("ASOManagedCluster"), mgr.GetClient(), &infrav1.ASOManagedCluster{}),
			),
		).
		Watches(
			&infrav1.ASOManagedControlPlane{},
			handler.EnqueueRequestsFromMapFunc(r.asoManagedControlPlaneToManagedClusterMap()),
			builder.WithPredicates(
				asoManagedControlPlaneEndpointUpdated(),
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

func (r *ASOManagedClusterReconciler) asoManagedControlPlaneToManagedClusterMap() handler.MapFunc {
	return func(ctx context.Context, o client.Object) []reconcile.Request {
		asoControlPlane := o.(*infrav1.ASOManagedControlPlane)

		cluster, err := util.GetOwnerCluster(ctx, r.Client, asoControlPlane.ObjectMeta)
		if err != nil {
			return nil
		}

		if cluster == nil ||
			cluster.Spec.InfrastructureRef == nil ||
			cluster.Spec.InfrastructureRef.Kind != "ASOManagedCluster" {
			return nil
		}

		return []reconcile.Request{
			{
				NamespacedName: client.ObjectKey{
					Namespace: cluster.Spec.InfrastructureRef.Namespace,
					Name:      cluster.Spec.InfrastructureRef.Name,
				},
			},
		}
	}
}

func asoManagedControlPlaneEndpointUpdated() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool { return true },
		UpdateFunc: func(ev event.UpdateEvent) bool {
			oldControlPlane := ev.ObjectOld.(*infrav1.ASOManagedControlPlane)
			newControlPlane := ev.ObjectNew.(*infrav1.ASOManagedControlPlane)
			return oldControlPlane.Status.ControlPlaneEndpoint !=
				newControlPlane.Status.ControlPlaneEndpoint
		},
	}
}

func ignorePatchErrNotFound(err error) error {
	notFound := apierrors.IsNotFound(err)
	if !notFound {
		if agg, ok := err.(kerrors.Aggregate); ok {
			for _, aggErr := range kerrors.Flatten(agg).Errors() {
				if apierrors.IsNotFound(aggErr) {
					notFound = true
					break
				}
			}
		}
	}
	if notFound {
		return nil
	}
	return err
}

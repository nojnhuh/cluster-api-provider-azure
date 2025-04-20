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

package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	infracontroller "sigs.k8s.io/cluster-api-provider-azure/controllers"
	infrav1alphaexp "sigs.k8s.io/cluster-api-provider-azure/exp/api/v1alpha1"
	"sigs.k8s.io/cluster-api-provider-azure/pkg/mutators"
	"sigs.k8s.io/cluster-api-provider-azure/util/tele"
)

// AzureASOClusterReconciler reconciles a AzureASOCluster object.
type AzureASOClusterReconciler struct {
	client.Client
	WatchFilterValue string

	newResourceReconciler func(*infrav1alphaexp.AzureASOCluster, []*unstructured.Unstructured) resourceReconciler
}

type resourceReconciler interface {
	// Reconcile reconciles resources defined by this object and updates this object's status to reflect the
	// state of the specified resources.
	Reconcile(context.Context) error

	// Pause stops ASO from continuously reconciling the specified resources.
	Pause(context.Context) error

	// Delete begins deleting the specified resources and updates the object's status to reflect the state of
	// the specified resources.
	Delete(context.Context) error
}

// SetupWithManager sets up the controller with the Manager.
func (r *AzureASOClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	ctx, log, done := tele.StartSpanWithLogger(ctx,
		"controllers.AzureASOClusterReconciler.SetupWithManager",
		tele.KVP("controller", infrav1alphaexp.AzureASOClusterKind),
	)
	defer done()

	c, err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&infrav1alphaexp.AzureASOCluster{}).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), log, r.WatchFilterValue)).
		WithEventFilter(predicates.ResourceIsNotExternallyManaged(mgr.GetScheme(), log)).
		// Watch clusters for pause/unpause notifications
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(
				util.ClusterToInfrastructureMapFunc(ctx, infrav1alphaexp.GroupVersion.WithKind(infrav1alphaexp.AzureASOClusterKind), mgr.GetClient(), &infrav1alphaexp.AzureASOCluster{}),
			),
			builder.WithPredicates(
				predicates.ResourceHasFilterLabel(mgr.GetScheme(), log, r.WatchFilterValue),
				infracontroller.ClusterUpdatePauseChange(log),
			),
		).
		Build(r)
	if err != nil {
		return err
	}

	externalTracker := &external.ObjectTracker{
		Cache:           mgr.GetCache(),
		Controller:      c,
		Scheme:          mgr.GetScheme(),
		PredicateLogger: &log,
	}

	r.newResourceReconciler = func(asoCluster *infrav1alphaexp.AzureASOCluster, resources []*unstructured.Unstructured) resourceReconciler {
		return &infracontroller.ResourceReconciler{
			Client:    r.Client,
			Resources: resources,
			Owner:     asoCluster,
			Watcher:   externalTracker,
		}
	}

	return nil
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasoclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasoclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasoclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=network.azure.com,resources=publicipaddresses;loadbalancers;networksecuritygroups;networksecuritygroupssecurityrules;routetables,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=network.azure.com,resources=publicipaddresses/status;loadbalancers/status;networksecuritygroups/status;networksecuritygroupssecurityrules/status;routetables/status,verbs=get;list;watch

// Reconcile reconciles an AzureASOCluster.
func (r *AzureASOClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, resultErr error) {
	ctx, _, done := tele.StartSpanWithLogger(ctx,
		"controllers.AzureASOClusterReconciler.Reconcile",
		tele.KVP("namespace", req.Namespace),
		tele.KVP("name", req.Name),
		tele.KVP("kind", infrav1alphaexp.AzureASOClusterKind),
	)
	defer done()

	asoCluster := &infrav1alphaexp.AzureASOCluster{}
	err := r.Get(ctx, req.NamespacedName, asoCluster)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(asoCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create patch helper: %w", err)
	}
	defer func() {
		err := patchHelper.Patch(ctx, asoCluster)
		if err != nil && resultErr == nil {
			resultErr = err
			result = ctrl.Result{}
		}
	}()

	cluster, err := util.GetOwnerCluster(ctx, r.Client, asoCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}

	if cluster != nil && cluster.Spec.Paused ||
		annotations.HasPaused(asoCluster) {
		return r.reconcilePaused(ctx, asoCluster)
	}

	if !asoCluster.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, asoCluster)
	}

	return r.reconcileNormal(ctx, asoCluster, cluster)
}

func (r *AzureASOClusterReconciler) reconcileNormal(ctx context.Context, asoCluster *infrav1alphaexp.AzureASOCluster, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	ctx, log, done := tele.StartSpanWithLogger(ctx,
		"controllers.AzureASOClusterReconciler.reconcileNormal",
	)
	defer done()
	log.V(4).Info("reconciling normally")

	if cluster == nil {
		log.V(4).Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	needsPatch := controllerutil.AddFinalizer(asoCluster, infrav1alphaexp.AzureASOClusterFinalizer)
	needsPatch = infracontroller.AddBlockMoveAnnotation(asoCluster) || needsPatch
	if needsPatch {
		return ctrl.Result{Requeue: true}, nil
	}

	us, err := mutators.ToUnstructured(ctx, asoCluster.Spec.Resources)
	if err != nil {
		return ctrl.Result{}, err
	}
	resourceReconciler := r.newResourceReconciler(asoCluster, us)
	err = resourceReconciler.Reconcile(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile resources: %w", err)
	}
	for _, status := range asoCluster.Status.Resources {
		if !status.Ready {
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *AzureASOClusterReconciler) reconcilePaused(ctx context.Context, asoCluster *infrav1alphaexp.AzureASOCluster) (ctrl.Result, error) {
	ctx, log, done := tele.StartSpanWithLogger(ctx,
		"controllers.AzureASOClusterReconciler.reconcilePaused",
	)
	defer done()
	log.V(4).Info("reconciling pause")

	resources, err := mutators.ToUnstructured(ctx, asoCluster.Spec.Resources)
	if err != nil {
		return ctrl.Result{}, err
	}
	resourceReconciler := r.newResourceReconciler(asoCluster, resources)
	err = resourceReconciler.Pause(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to pause resources: %w", err)
	}

	infracontroller.RemoveBlockMoveAnnotation(asoCluster)

	return ctrl.Result{}, nil
}

func (r *AzureASOClusterReconciler) reconcileDelete(ctx context.Context, asoCluster *infrav1alphaexp.AzureASOCluster) (ctrl.Result, error) {
	ctx, log, done := tele.StartSpanWithLogger(ctx,
		"controllers.AzureASOClusterReconciler.reconcileDelete",
	)
	defer done()
	log.V(4).Info("reconciling delete")

	resources, err := mutators.ToUnstructured(ctx, asoCluster.Spec.Resources)
	if err != nil {
		return ctrl.Result{}, err
	}
	resourceReconciler := r.newResourceReconciler(asoCluster, resources)
	err = resourceReconciler.Delete(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile resources: %w", err)
	}
	if len(asoCluster.Status.Resources) > 0 {
		return ctrl.Result{}, nil
	}

	controllerutil.RemoveFinalizer(asoCluster, infrav1alphaexp.AzureASOClusterFinalizer)

	return ctrl.Result{}, nil
}

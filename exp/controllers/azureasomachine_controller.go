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

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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
	"sigs.k8s.io/cluster-api-provider-azure/util/tele"
)

// AzureASOMachineReconciler reconciles a AzureASOMachine object.
type AzureASOMachineReconciler struct {
	client.Client
	WatchFilterValue string
}

// SetupWithManager sets up the controller with the Manager.
func (r *AzureASOMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	_, log, done := tele.StartSpanWithLogger(ctx,
		"controllers.AzureASOMachineReconciler.SetupWithManager",
		tele.KVP("controller", infrav1alphaexp.AzureASOMachineKind),
	)
	defer done()

	clusterMapper, err := util.ClusterToTypedObjectsMapper(r.Client, &infrav1alphaexp.AzureASOMachineList{}, mgr.GetScheme())
	if err != nil {
		return fmt.Errorf("failed to create mapper for Cluster to AzureASOMachines: %w", err)
	}

	_, err = ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&infrav1alphaexp.AzureASOMachine{}).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), log, r.WatchFilterValue)).
		WithEventFilter(predicates.ResourceIsNotExternallyManaged(mgr.GetScheme(), log)).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(
				util.MachineToInfrastructureMapFunc(infrav1alphaexp.GroupVersion.WithKind(infrav1alphaexp.AzureASOMachineKind)),
			),
			builder.WithPredicates(
				predicates.ResourceHasFilterLabel(mgr.GetScheme(), log, r.WatchFilterValue),
			),
		).
		// Watch clusters for pause/unpause notifications
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterMapper),
			builder.WithPredicates(
				predicates.ResourceHasFilterLabel(mgr.GetScheme(), log, r.WatchFilterValue),
				infracontroller.ClusterPauseChangeAndInfrastructureReady(mgr.GetScheme(), log),
			),
		).
		Build(r)
	if err != nil {
		return err
	}

	return nil
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasomachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasomachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasomachines/finalizers,verbs=update

// Reconcile reconciles an AzureASOMachine.
func (r *AzureASOMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, resultErr error) {
	ctx, _, done := tele.StartSpanWithLogger(ctx,
		"controllers.AzureASOMachineReconciler.Reconcile",
		tele.KVP("namespace", req.Namespace),
		tele.KVP("name", req.Name),
		tele.KVP("kind", infrav1alphaexp.AzureASOMachineKind),
	)
	defer done()

	asoMachine := &infrav1alphaexp.AzureASOMachine{}
	err := r.Get(ctx, req.NamespacedName, asoMachine)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(asoMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create patch helper: %w", err)
	}
	defer func() {
		err := patchHelper.Patch(ctx, asoMachine)
		if err != nil && resultErr == nil {
			resultErr = err
			result = ctrl.Result{}
		}
	}()

	machine, err := util.GetOwnerMachine(ctx, r.Client, asoMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}

	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, asoMachine.ObjectMeta)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	if cluster != nil && cluster.Spec.Paused ||
		annotations.HasPaused(asoMachine) {
		return r.reconcilePaused(ctx, asoMachine)
	}

	if !asoMachine.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, asoMachine)
	}

	return r.reconcileNormal(ctx, asoMachine, machine)
}

func (r *AzureASOMachineReconciler) reconcileNormal(ctx context.Context, asoMachine *infrav1alphaexp.AzureASOMachine, machine *clusterv1.Machine) (ctrl.Result, error) {
	ctx, log, done := tele.StartSpanWithLogger(ctx,
		"controllers.AzureASOMachineReconciler.reconcileNormal",
	)
	defer done()
	log.V(4).Info("reconciling normally")

	// this will be used soon
	_ = ctx

	if machine == nil {
		log.V(4).Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	needsPatch := controllerutil.AddFinalizer(asoMachine, infrav1alphaexp.AzureASOMachineFinalizer)
	if needsPatch {
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

func (r *AzureASOMachineReconciler) reconcilePaused(ctx context.Context, _ *infrav1alphaexp.AzureASOMachine) (ctrl.Result, error) {
	ctx, log, done := tele.StartSpanWithLogger(ctx,
		"controllers.AzureASOMachineReconciler.reconcilePaused",
	)
	defer done()
	log.V(4).Info("reconciling pause")

	// this will be used soon
	_ = ctx

	return ctrl.Result{}, nil
}

func (r *AzureASOMachineReconciler) reconcileDelete(ctx context.Context, asoMachine *infrav1alphaexp.AzureASOMachine) (ctrl.Result, error) {
	ctx, log, done := tele.StartSpanWithLogger(ctx,
		"controllers.AzureASOMachineReconciler.reconcileDelete",
	)
	defer done()
	log.V(4).Info("reconciling delete")

	// this will be used soon
	_ = ctx

	controllerutil.RemoveFinalizer(asoMachine, infrav1alphaexp.AzureASOMachineFinalizer)

	return ctrl.Result{}, nil
}

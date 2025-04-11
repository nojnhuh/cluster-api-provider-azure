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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
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

// AzureASOMachineReconciler reconciles a AzureASOMachine object.
type AzureASOMachineReconciler struct {
	client.Client
	WatchFilterValue string

	newResourceReconciler func(*infrav1alphaexp.AzureASOMachine, []*unstructured.Unstructured) resourceReconciler
	watcher               watcher
}

// SetupWithManager sets up the controller with the Manager.
func (r *AzureASOMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	ctx, log, done := tele.StartSpanWithLogger(ctx,
		"controllers.AzureASOMachineReconciler.SetupWithManager",
		tele.KVP("controller", infrav1alphaexp.AzureASOMachineKind),
	)
	defer done()

	clusterMapper, err := util.ClusterToTypedObjectsMapper(r.Client, &infrav1alphaexp.AzureASOMachineList{}, mgr.GetScheme())
	if err != nil {
		return fmt.Errorf("failed to create mapper for Cluster to AzureASOMachines: %w", err)
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
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
				predicates.ClusterPausedTransitionsOrInfrastructureProvisioned(mgr.GetScheme(), log),
			),
		).
		Build(r)
	if err != nil {
		return err
	}

	r.watcher = &external.ObjectTracker{
		Cache:           mgr.GetCache(),
		Controller:      c,
		Scheme:          mgr.GetScheme(),
		PredicateLogger: &log,
	}

	r.newResourceReconciler = func(asoMachine *infrav1alphaexp.AzureASOMachine, resources []*unstructured.Unstructured) resourceReconciler {
		return &infracontroller.ResourceReconciler{
			Client:    r.Client,
			Resources: resources,
			Owner:     asoMachine,
			Watcher:   r.watcher,
		}
	}

	// Allow for efficient lookups of AzureASOMachines that are informed by a particular ConfigMap.
	err = mgr.GetCache().IndexField(ctx, &infrav1alphaexp.AzureASOMachine{}, configMapIndexedField, func(o client.Object) (keys []string) {
		asoMachine, ok := o.(*infrav1alphaexp.AzureASOMachine)
		if !ok ||
			asoMachine == nil ||
			asoMachine.Spec.ProviderIDSource == nil {
			return
		}
		log := log.WithValues("kind", infrav1alphaexp.AzureASOMachineKind, "namespace", asoMachine.Namespace, "name", asoMachine.Name)
		if ref := asoMachine.Spec.ProviderIDSource.ConfigMap; ref != nil {
			name, err := evalStringValue(ref.Name, asoMachine)
			if err != nil {
				log.Error(err, "failed to evaluate ConfigMap name", "field", "spec.providerIDSource.configMap.name")
			} else {
				keys = append(keys, name)
			}
		}
		return
	})
	if err != nil {
		return err
	}

	return nil
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasomachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasomachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasomachines/finalizers,verbs=update
//+kubebuilder:rbac:groups=compute.azure.com,resources=virtualmachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=compute.azure.com,resources=virtualmachines/status,verbs=get;list;watch
//+kubebuilder:rbac:groups=network.azure.com,resources=networkinterfaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=network.azure.com,resources=networkinterfaces/status,verbs=get;list;watch
//+kubebuilder:rbac:groups=authorization.azure.com,resources=roleassignments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=authorization.azure.com,resources=roleassignments/status,verbs=get;list;watch

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

	if cluster != nil && ptr.Deref(cluster.Spec.Paused, false) ||
		annotations.HasPaused(asoMachine) {
		return r.reconcilePaused(ctx, asoMachine, machine, cluster)
	}

	if !asoMachine.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, asoMachine, machine, cluster)
	}

	return r.reconcileNormal(ctx, asoMachine, machine, cluster)
}

func (r *AzureASOMachineReconciler) resourceReconciler(ctx context.Context, asoMachine *infrav1alphaexp.AzureASOMachine, machine *clusterv1.Machine, cluster *clusterv1.Cluster) (resourceReconciler, error) {
	var bootstrapSecret *corev1.Secret
	if machine != nil && machine.Spec.Bootstrap.DataSecretName != nil {
		secret := &corev1.Secret{}
		err := r.Get(ctx, client.ObjectKey{Namespace: asoMachine.Namespace, Name: *machine.Spec.Bootstrap.DataSecretName}, secret)
		if err != nil {
			return nil, err
		}
		bootstrapSecret = secret
	}

	templateData, err := infrav1alphaexp.AzureASOMachineJSONPatchValueFromTemplateData(asoMachine, machine, cluster, bootstrapSecret)
	if err != nil {
		return nil, err
	}

	resources, err := applyPatches(ctx, asoMachine.Spec.Resources, asoMachine.Spec.Patches, templateData)
	if err != nil {
		return nil, err
	}
	us, err := mutators.ToUnstructured(ctx, resources)
	if err != nil {
		return nil, err
	}
	return r.newResourceReconciler(asoMachine, us), nil
}

func (r *AzureASOMachineReconciler) reconcileNormal(ctx context.Context, asoMachine *infrav1alphaexp.AzureASOMachine, machine *clusterv1.Machine, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	ctx, log, done := tele.StartSpanWithLogger(ctx,
		"controllers.AzureASOMachineReconciler.reconcileNormal",
	)
	defer done()
	log.V(4).Info("reconciling normally")

	if machine == nil {
		log.V(4).Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	needsPatch := controllerutil.AddFinalizer(asoMachine, infrav1alphaexp.AzureASOMachineFinalizer)
	needsPatch = infracontroller.AddBlockMoveAnnotation(asoMachine) || needsPatch
	if needsPatch {
		return ctrl.Result{Requeue: true}, nil
	}

	if !ptr.Deref(cluster.Status.Initialization.InfrastructureProvisioned, false) {
		log.V(4).Info("Waiting for cluster infrastructure")
		return ctrl.Result{}, nil
	}

	if machine.Spec.Bootstrap.DataSecretName == nil {
		log.V(4).Info("Waiting for bootstrap data")
		return ctrl.Result{}, nil
	}

	bootstrapSecret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Namespace: asoMachine.Namespace, Name: *machine.Spec.Bootstrap.DataSecretName}, bootstrapSecret)
	if err != nil {
		return ctrl.Result{}, err
	}

	resourceReconciler, err := r.resourceReconciler(ctx, asoMachine, machine, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	err = resourceReconciler.Reconcile(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile resources: %w", err)
	}
	for _, status := range asoMachine.Status.Resources {
		if !status.Ready {
			return ctrl.Result{}, nil
		}
	}

	if asoMachine.Spec.ProviderIDSource != nil {
		providerID, err := reconcileStringSource(ctx, r.Client, r.watcher, *asoMachine.Spec.ProviderIDSource, asoMachine, &infrav1alphaexp.AzureASOMachineList{})
		if err != nil {
			return ctrl.Result{}, err
		}
		asoMachine.Spec.ProviderID = providerID
	}

	asoMachine.Status.Initialization.Provisioned = ptr.To(true)

	return ctrl.Result{}, nil
}

func (r *AzureASOMachineReconciler) reconcilePaused(ctx context.Context, asoMachine *infrav1alphaexp.AzureASOMachine, machine *clusterv1.Machine, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	ctx, log, done := tele.StartSpanWithLogger(ctx,
		"controllers.AzureASOMachineReconciler.reconcilePaused",
	)
	defer done()
	log.V(4).Info("reconciling pause")

	resourceReconciler, err := r.resourceReconciler(ctx, asoMachine, machine, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	err = resourceReconciler.Pause(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to pause resources: %w", err)
	}

	infracontroller.RemoveBlockMoveAnnotation(asoMachine)

	return ctrl.Result{}, nil
}

func (r *AzureASOMachineReconciler) reconcileDelete(ctx context.Context, asoMachine *infrav1alphaexp.AzureASOMachine, machine *clusterv1.Machine, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	ctx, log, done := tele.StartSpanWithLogger(ctx,
		"controllers.AzureASOMachineReconciler.reconcileDelete",
	)
	defer done()
	log.V(4).Info("reconciling delete")

	resourceReconciler, err := r.resourceReconciler(ctx, asoMachine, machine, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	err = resourceReconciler.Delete(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile resources: %w", err)
	}
	if len(asoMachine.Status.Resources) > 0 {
		return ctrl.Result{}, nil
	}

	controllerutil.RemoveFinalizer(asoMachine, infrav1alphaexp.AzureASOMachineFinalizer)

	return ctrl.Result{}, nil
}

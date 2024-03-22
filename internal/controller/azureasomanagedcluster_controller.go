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
	"errors"
	"fmt"
	"strings"

	asoconditions "github.com/Azure/azure-service-operator/v2/pkg/genruntime/conditions"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-azure/internal/mutators"
)

var invalidControlPlaneKindErr = errors.New("AzureASOManagedCluster cannot be used without AzureASOManagedControlPlane")

// AzureASOManagedClusterReconciler reconciles a AzureASOManagedCluster object
type AzureASOManagedClusterReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	externalTracker       *external.ObjectTracker
	newResourceReconciler func(*infrav1.AzureASOManagedCluster, []*unstructured.Unstructured) resourceReconciler
}

type resourceReconciler interface {
	// Reconcile reconciles resources defined by this object and updates this object's status to reflect the
	// state of the specified resources.
	Reconcile(context.Context) error

	// Delete begins deleting the specified resources and updates the object's status to reflect the state of
	// the specified resources.
	Delete(context.Context) error
}

//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasomanagedclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasomanagedclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasomanagedclusters/finalizers,verbs=update

// Reconcile reconciles an AzureASOManagedCluster.
func (r *AzureASOManagedClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, resultErr error) {
	asoManagedCluster := &infrav1.AzureASOManagedCluster{}
	err := r.Get(ctx, req.NamespacedName, asoManagedCluster)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(asoManagedCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper: %w", err)
	}
	defer func() {
		r.reconcileConditions(asoManagedCluster, resultErr)

		opts := []patch.Option{
			patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				clusterv1.ReadyCondition,
				infrav1.Reconciled, // TODO: make sure this works
				infrav1.ResourcesReady,
				infrav1.ControlPlaneEndpointReady,
			}},
		}
		if resultErr == nil {
			opts = append(opts, patch.WithStatusObservedGeneration{})
		}

		err := patchHelper.Patch(ctx, asoManagedCluster, opts...)
		if !asoManagedCluster.GetDeletionTimestamp().IsZero() {
			err = ignorePatchErrNotFound(err)
		}
		if err != nil && resultErr == nil {
			resultErr = err
			result = ctrl.Result{}
		}
	}()

	// TODO: don't reconcile when cluster is paused. Ensure no other changes get made to ASO resources other
	// than setting the reconciliation policy.

	cluster, err := util.GetOwnerCluster(ctx, r.Client, asoManagedCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !asoManagedCluster.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, asoManagedCluster, cluster)
	}

	return r.reconcileNormal(ctx, asoManagedCluster, cluster)
}

func (r *AzureASOManagedClusterReconciler) reconcileNormal(ctx context.Context, asoManagedCluster *infrav1.AzureASOManagedCluster, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}
	if cluster.Spec.ControlPlaneRef == nil || cluster.Spec.ControlPlaneRef.Kind != "AzureASOManagedControlPlane" {
		return ctrl.Result{}, reconcile.TerminalError(invalidControlPlaneKindErr)
	}

	needsPatch := controllerutil.AddFinalizer(asoManagedCluster, clusterv1.ClusterFinalizer)
	if !cluster.Spec.Paused {
		needsPatch = addBlockMoveAnnotation(asoManagedCluster) || needsPatch
	}
	if needsPatch {
		return ctrl.Result{Requeue: true}, nil
	}

	resources, err := mutators.ApplyMutators(ctx, asoManagedCluster.Spec.Resources, mutators.SetASOReconciliationPolicy(cluster))
	if err != nil {
		return ctrl.Result{}, err
	}
	resourceReconciler := r.newResourceReconciler(asoManagedCluster, resources)

	err = resourceReconciler.Reconcile(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, status := range asoManagedCluster.GetResourceStatuses() {
		if status.Condition.Status != metav1.ConditionTrue {
			return ctrl.Result{}, nil
		}
	}

	if cluster.Spec.Paused {
		removeBlockMoveAnnotation(asoManagedCluster)
	}

	asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Spec.ControlPlaneRef.Namespace,
			Name:      cluster.Spec.ControlPlaneRef.Name,
		},
	}
	err = r.Get(ctx, client.ObjectKeyFromObject(asoManagedControlPlane), asoManagedControlPlane)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get AzureASOManagedControlPlane %s/%s: %w", asoManagedControlPlane.Namespace, asoManagedControlPlane.Name, err)
	}

	asoManagedCluster.Spec.ControlPlaneEndpoint = asoManagedControlPlane.Status.ControlPlaneEndpoint

	return ctrl.Result{}, nil
}

func (r *AzureASOManagedClusterReconciler) reconcileDelete(ctx context.Context, asoManagedCluster *infrav1.AzureASOManagedCluster, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	if cluster == nil {
		controllerutil.RemoveFinalizer(asoManagedCluster, clusterv1.ClusterFinalizer)
		return ctrl.Result{}, nil
	}

	resources, err := mutators.ApplyMutators(ctx, asoManagedCluster.Spec.Resources)
	if err != nil {
		return ctrl.Result{}, err
	}
	resourceReconciler := r.newResourceReconciler(asoManagedCluster, resources)

	err = resourceReconciler.Delete(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(asoManagedCluster.GetResourceStatuses()) > 0 {
		// waiting for resources to be deleted
		return ctrl.Result{}, nil
	}

	controllerutil.RemoveFinalizer(asoManagedCluster, clusterv1.ClusterFinalizer)
	return ctrl.Result{}, nil
}

func (r *AzureASOManagedClusterReconciler) reconcileConditions(asoManagedCluster *infrav1.AzureASOManagedCluster, resultErr error) {
	reconcileResourcesReadyCondition(asoManagedCluster, asoManagedCluster.Spec.Resources)
	reconcileResultErr(asoManagedCluster, resultErr)

	conditions.MarkTrue(asoManagedCluster, infrav1.ControlPlaneEndpointReady)
	if asoManagedCluster.Spec.ControlPlaneEndpoint.IsZero() {
		conditions.MarkFalse(asoManagedCluster, infrav1.ControlPlaneEndpointReady, "WaitingForControlPlaneEndpoint", clusterv1.ConditionSeverityInfo, fmt.Sprintf("Waiting for spec.controlPlaneEndpoint"))
	}

	conditions.SetSummary(asoManagedCluster)
	asoManagedCluster.Status.Ready = conditions.IsTrue(asoManagedCluster, clusterv1.ReadyCondition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AzureASOManagedClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, log logr.Logger) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.AzureASOManagedCluster{}).
		WithEventFilter(predicates.ResourceIsNotExternallyManaged(log)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(
				util.ClusterToInfrastructureMapFunc(ctx, infrav1.GroupVersion.WithKind("AzureASOManagedCluster"), mgr.GetClient(), &infrav1.AzureASOManagedCluster{}),
			),
			builder.WithPredicates(
				ClusterUpdatePauseChange(log),
			),
		).
		Watches(
			&infrav1.AzureASOManagedControlPlane{},
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

	r.newResourceReconciler = func(asoManagedCluster *infrav1.AzureASOManagedCluster, us []*unstructured.Unstructured) resourceReconciler {
		return &InfraReconciler{
			Client:    r.Client,
			resources: us,
			owner:     asoManagedCluster,
			watcher:   r.externalTracker,
		}
	}

	return nil
}

func (r *AzureASOManagedClusterReconciler) asoManagedControlPlaneToManagedClusterMap() handler.MapFunc {
	return func(ctx context.Context, o client.Object) []reconcile.Request {
		asoManagedControlPlane := o.(*infrav1.AzureASOManagedControlPlane)

		cluster, err := util.GetOwnerCluster(ctx, r.Client, asoManagedControlPlane.ObjectMeta)
		if err != nil {
			return nil
		}

		if cluster == nil ||
			cluster.Spec.InfrastructureRef == nil ||
			cluster.Spec.InfrastructureRef.Kind != "AzureASOManagedCluster" {
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
			oldControlPlane := ev.ObjectOld.(*infrav1.AzureASOManagedControlPlane)
			newControlPlane := ev.ObjectNew.(*infrav1.AzureASOManagedControlPlane)
			return oldControlPlane.Status.ControlPlaneEndpoint !=
				newControlPlane.Status.ControlPlaneEndpoint
		},
	}
}

func ignorePatchErrNotFound(err error) error {
	if apierrors.IsNotFound(err) {
		return nil
	}
	if agg, ok := err.(kerrors.Aggregate); ok {
		for _, aggErr := range kerrors.Flatten(agg).Errors() {
			if apierrors.IsNotFound(aggErr) {
				return nil
			}
		}
	}
	return err
}

func reconcileResourcesReadyCondition(obj interface {
	conditions.Setter
	resourceStatusObject
}, specResources []runtime.RawExtension) {
	type severity int
	const (
		none severity = iota
		info
		warning
		error
	)
	severities := map[asoconditions.ConditionSeverity]severity{
		asoconditions.ConditionSeverityNone:    none,
		asoconditions.ConditionSeverityInfo:    info,
		asoconditions.ConditionSeverityWarning: warning,
		asoconditions.ConditionSeverityError:   error,
	}
	toCAPISeverity := map[severity]clusterv1.ConditionSeverity{
		none:    clusterv1.ConditionSeverityNone,
		info:    clusterv1.ConditionSeverityInfo,
		warning: clusterv1.ConditionSeverityWarning,
		error:   clusterv1.ConditionSeverityError,
	}

	expectedReady := map[infrav1.StatusResource]struct{}{}
	actualReady := map[infrav1.StatusResource]bool{}
	for _, resource := range specResources {
		u := &unstructured.Unstructured{}
		u.UnmarshalJSON(resource.Raw) // TODO: err
		expectedReady[infrav1.StatusResource{
			Group: u.GroupVersionKind().Group,
			Kind:  u.GroupVersionKind().Kind,
			Name:  u.GetName(),
		}] = struct{}{}
	}
	highestSeverity := none
	for _, status := range obj.GetResourceStatuses() {
		actualReady[infrav1.StatusResource{
			Group: status.Resource.Group,
			Kind:  status.Resource.Kind,
			Name:  status.Resource.Name,
		}] = status.Condition.Status == metav1.ConditionTrue

		if severities[status.Condition.Severity] > highestSeverity {
			highestSeverity = severities[status.Condition.Severity]
		}
	}

	readyCount := 0
	staleCount := 0
	total := len(specResources)
	for expected := range expectedReady {
		if ready := actualReady[expected]; ready {
			readyCount++
		}
	}
	for actual := range actualReady {
		if _, exists := expectedReady[actual]; !exists {
			staleCount++
		}
	}

	var msgs []string
	if readyCount < total {
		msgs = append(msgs, fmt.Sprintf("%d of %d resources are ready", readyCount, total))
	}
	if staleCount > 0 {
		msgs = append(msgs, fmt.Sprintf("%d resources are being deleted", staleCount))
	}
	conditions.MarkTrue(obj, infrav1.ResourcesReady)
	if len(msgs) > 0 {
		if highestSeverity == none {
			// CAPI won't process this correctly when setting the Ready condition when no severity is set.
			// This could happen when there are no resource statuses.
			highestSeverity = info
		}
		conditions.MarkFalse(obj, infrav1.ResourcesReady, "ResourcesNotReady", toCAPISeverity[highestSeverity], strings.Join(msgs, ", "))
	}
}

func reconcileResultErr(obj conditions.Setter, resultErr error) {
	conditions.MarkTrue(obj, infrav1.Reconciled)
	if resultErr != nil {
		conditions.MarkFalse(obj, infrav1.Reconciled, "FailedToReconcile", clusterv1.ConditionSeverityError, resultErr.Error())
	}
}

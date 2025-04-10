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
	"encoding/json"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch/v5"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
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
//+kubebuilder:rbac:groups=network.azure.com,resources=publicipaddresses;loadbalancers;loadbalancersinboundnatrules;networksecuritygroups;networksecuritygroupssecurityrules;routetables,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=network.azure.com,resources=publicipaddresses/status;loadbalancers/status;loadbalancersinboundnatrules/status;networksecuritygroups/status;networksecuritygroupssecurityrules/status;routetables/status,verbs=get;list;watch

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

	resourceReconciler, err := r.resourceReconciler(ctx, asoCluster)
	if err != nil {
		return ctrl.Result{}, err
	}
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

	resourceReconciler, err := r.resourceReconciler(ctx, asoCluster)
	if err != nil {
		return ctrl.Result{}, err
	}
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

	resourceReconciler, err := r.resourceReconciler(ctx, asoCluster)
	if err != nil {
		return ctrl.Result{}, err
	}
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

func (r *AzureASOClusterReconciler) resourceReconciler(ctx context.Context, asoCluster *infrav1alphaexp.AzureASOCluster) (resourceReconciler, error) {
	resources, err := applyPatches(ctx, asoCluster.Spec.Resources, asoCluster.Spec.Patches)
	if err != nil {
		return nil, err
	}
	us, err := mutators.ToUnstructured(ctx, resources)
	if err != nil {
		return nil, err
	}
	return r.newResourceReconciler(asoCluster, us), nil
}

type json6902 struct {
	Op    infrav1alphaexp.JSONPatchOp `json:"op"`
	Path  string                      `json:"path"`
	From  string                      `json:"from,omitempty"`
	Value *apiextensionsv1.JSON       `json:"value,omitempty"`
}

func applyPatches(ctx context.Context, resources []runtime.RawExtension, patches []infrav1alphaexp.ResourcesPatch) ([]runtime.RawExtension, error) {
	ctx, _, done := tele.StartSpanWithLogger(ctx,
		"controllers.applyPatches",
	)
	defer done()

	us, err := mutators.ToUnstructured(ctx, resources)
	if err != nil {
		return nil, err
	}

	var patchedResources []runtime.RawExtension

	for i, u := range us {
		resourceYAML := resources[i].Raw
		resourceJSON, err := yaml.ToJSON(resourceYAML)
		if err != nil {
			return nil, err
		}

		for _, patch := range patches {
			var json6902Patches []json6902

			if !patchSelectorsMatch(patch.Selectors, u) {
				continue
			}

			for _, jsonPatch := range patch.JSONPatches {
				json6902Patches = append(json6902Patches, json6902{
					Op:    jsonPatch.Op,
					Path:  jsonPatch.Path,
					From:  jsonPatch.From,
					Value: jsonPatch.Value,
				})
			}
			patchData, err := json.Marshal(json6902Patches)
			if err != nil {
				return nil, err
			}
			jsonPatch, err := jsonpatch.DecodePatch(patchData)
			if err != nil {
				return nil, err
			}
			resourceJSON, err = jsonPatch.Apply(resourceJSON)
			if err != nil {
				return nil, err
			}
		}

		patchedResources = append(patchedResources, runtime.RawExtension{Raw: resourceJSON})
	}

	return patchedResources, nil
}

func patchSelectorsMatch(selectors []infrav1alphaexp.ResourcesPatchSelector, u *unstructured.Unstructured) bool {
	for _, selector := range selectors {
		if (selector.APIVersion == "" || selector.APIVersion == u.GetAPIVersion()) &&
			(selector.Kind == "" || selector.Kind == u.GetKind()) &&
			(selector.Name == "" || selector.Name == u.GetName()) {
			return true
		}
	}
	// No selectors matches everything
	return len(selectors) == 0
}

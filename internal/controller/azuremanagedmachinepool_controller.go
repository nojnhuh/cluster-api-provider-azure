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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	utilexp "sigs.k8s.io/cluster-api/exp/util"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20231001"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/v2/api/v2alpha1"
	"sigs.k8s.io/cluster-api-provider-azure/v2/internal/aks"
)

// AzureManagedMachinePoolReconciler reconciles a AzureManagedMachinePool object
type AzureManagedMachinePoolReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	Tracker               ClusterTracker
	controller            controller.Controller
	externalTracker       *external.ObjectTracker
	newResourceReconciler func(*infrav1.AzureManagedMachinePool, []*unstructured.Unstructured) resourceReconciler
}

type ClusterTracker interface {
	GetClient(context.Context, types.NamespacedName) (client.Client, error)
}

//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azuremanagedmachinepools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azuremanagedmachinepools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azuremanagedmachinepools/finalizers,verbs=update

// Reconcile reconciles an AzureManagedMachinePool.
func (r *AzureManagedMachinePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, resultErr error) {
	log := log.FromContext(ctx)

	azureManagedMachinePool := &infrav1.AzureManagedMachinePool{}
	err := r.Get(ctx, req.NamespacedName, azureManagedMachinePool)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(azureManagedMachinePool, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper: %w", err)
	}
	defer func() {
		if resultErr == nil {
			azureManagedMachinePool.Status.ObservedGeneration = azureManagedMachinePool.Generation
		}

		err = patchHelper.Patch(ctx, azureManagedMachinePool)
		if !azureManagedMachinePool.GetDeletionTimestamp().IsZero() {
			err = ignorePatchErrNotFound(err)
		}

		if err != nil && resultErr == nil {
			resultErr = err
			result = ctrl.Result{}
		}
	}()

	machinePool, err := utilexp.GetOwnerMachinePool(ctx, r.Client, azureManagedMachinePool.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machinePool == nil {
		log.Info("Waiting for MachinePool Controller to set OwnerRef on AzureManagedMachinePool")
		return ctrl.Result{}, nil
	}

	machinePoolBefore := machinePool.DeepCopy()
	defer func() {
		// Skip using a patch helper here because we will never modify the MachinePool status.
		err := r.Patch(ctx, machinePool, client.MergeFrom(machinePoolBefore))
		if err != nil && resultErr == nil {
			resultErr = err
			result = ctrl.Result{}
		}
	}()

	log = log.WithValues("MachinePool", machinePool.Name)
	ctx = ctrl.LoggerInto(ctx, log)

	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machinePool.ObjectMeta)
	if err != nil {
		log.Info("AzureManagedMachinePool owner MachinePool is missing cluster label or cluster does not exist")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info(fmt.Sprintf("Please associate this machine pool with a cluster using the label %s: <name of cluster>", clusterv1.ClusterNameLabel))
		return ctrl.Result{}, nil
	}
	if cluster.Spec.ControlPlaneRef == nil || cluster.Spec.ControlPlaneRef.Kind != "AzureManagedControlPlane" {
		return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("AzureManagedMachinePool cannot be used without AzureManagedControlPlane"))
	}

	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	if !azureManagedMachinePool.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, azureManagedMachinePool, cluster)
	}

	return r.reconcileNormal(ctx, azureManagedMachinePool, machinePool, cluster)
}

func (r *AzureManagedMachinePoolReconciler) reconcileNormal(ctx context.Context, azureManagedMachinePool *infrav1.AzureManagedMachinePool, machinePool *expv1.MachinePool, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	needsPatch := controllerutil.AddFinalizer(azureManagedMachinePool, infrav1.AzureManagedMachinePoolFinalizer)
	if !cluster.Spec.Paused {
		needsPatch = addBlockMoveAnnotation(azureManagedMachinePool) || needsPatch
	}
	if needsPatch {
		return ctrl.Result{Requeue: true}, nil
	}

	// MachinePools created by ClusterClass have no spec.template.spec.bootstrap.dataSecretName defined.
	// TODO: see if we really need a kubeadm config for clusterclass machinepools.
	// // Make sure bootstrap data is available and populated.
	// if machinePool.Spec.Template.Spec.Bootstrap.DataSecretName == nil {
	// 	return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("AzureManagedMachinePool does not use spec.template.spec.bootstrap.dataSecretName, set this to any value in MachinePool %s/%s to continue", machinePool.Namespace, machinePool.Name))
	// }

	azureManagedMachinePool.Status.Ready = false

	setDefaults := func(us []*unstructured.Unstructured) error {
		for _, u := range us {
			if u.GroupVersionKind().Group == asocontainerservicev1.GroupVersion.Group &&
				u.GroupVersionKind().Kind == "ManagedClustersAgentPool" {
				err := aks.SetAgentPoolDefaults(u, machinePool)
				if err != nil {
					return err
				}
				break
			}
		}
		return nil
	}
	mutators := []resourcesMutator{setDefaults}
	if cluster.Spec.Paused {
		mutators = append(mutators, pauseResources)
	}

	resources, err := applyMutators(azureManagedMachinePool.Spec.Resources, mutators...)
	if err != nil {
		return ctrl.Result{}, err
	}

	var agentPoolName string
	for _, resource := range resources {
		if resource.GroupVersionKind().Group == asocontainerservicev1.GroupVersion.Group &&
			resource.GroupVersionKind().Kind == "ManagedClustersAgentPool" {
			agentPoolName = resource.GetName()
			break
		}
	}
	if agentPoolName == "" {
		return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("no %s ManagedClustersAgentPools defined in AzureManagedMachinePool spec.resources", asocontainerservicev1.GroupVersion.Group))
	}

	resourceReconciler := r.newResourceReconciler(azureManagedMachinePool, resources)
	err = resourceReconciler.Reconcile(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, status := range azureManagedMachinePool.GetResourceStatuses() {
		if !status.Ready {
			return ctrl.Result{}, nil
		}
	}

	if cluster.Spec.Paused {
		removeBlockMoveAnnotation(azureManagedMachinePool)
	}

	// get a typed resource so we don't have to try to convert this unstructured ourselves since it might be a
	// different API version than we know how to deal with.
	agentPool := &asocontainerservicev1.ManagedClustersAgentPool{}
	err = r.Get(ctx, client.ObjectKey{Namespace: azureManagedMachinePool.Namespace, Name: agentPoolName}, agentPool)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error getting ManagedClustersAgentPool: %w", err)
	}

	// I'm not entirely convinced we need to watch nodes here.
	managedCluster := &asocontainerservicev1.ManagedCluster{}
	err = r.Get(ctx, client.ObjectKey{Namespace: agentPool.Namespace, Name: agentPool.Owner().Name}, managedCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error getting ManagedCluster: %w", err)
	}
	if managedCluster.Status.NodeResourceGroup == nil {
		return ctrl.Result{}, nil
	}
	rg := *managedCluster.Status.NodeResourceGroup

	clusterClient, err := r.Tracker.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		return ctrl.Result{}, err
	}
	nodes := &corev1.NodeList{}
	err = clusterClient.List(ctx, nodes,
		client.MatchingLabels(expectedNodeLabels(agentPool.AzureName(), rg)),
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list nodes in workload cluster: %w", err)
	}
	providerIDs := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		providerIDs = append(providerIDs, node.Spec.ProviderID)
	}
	azureManagedMachinePool.Spec.ProviderIDList = providerIDs

	azureManagedMachinePool.Status.Replicas = int32(ptr.Deref(agentPool.Status.Count, 0))
	if auto, ok := machinePool.Annotations[clusterv1.ReplicasManagedByAnnotation]; ok && auto != "false" {
		machinePool.Spec.Replicas = &azureManagedMachinePool.Status.Replicas
	}

	azureManagedMachinePool.Status.Ready = true

	return ctrl.Result{}, nil
}

func expectedNodeLabels(poolName, nodeRG string) map[string]string {
	return map[string]string{
		"kubernetes.azure.com/agentpool": poolName,
		"kubernetes.azure.com/cluster":   nodeRG,
	}
}

func (r *AzureManagedMachinePoolReconciler) reconcileDelete(ctx context.Context, azureManagedMachinePool *infrav1.AzureManagedMachinePool, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	// If the entire cluster is being deleted, this ASO ManagedClustersAgentPool will be deleted with the rest
	// of the ManagedCluster.
	if cluster.DeletionTimestamp.IsZero() {
		resources, err := applyMutators(azureManagedMachinePool.Spec.Resources)
		if err != nil {
			return ctrl.Result{}, err
		}
		resourceReconciler := r.newResourceReconciler(azureManagedMachinePool, resources)

		err = resourceReconciler.Delete(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}
		if len(azureManagedMachinePool.GetResourceStatuses()) > 0 {
			// waiting for resources to be deleted
			return ctrl.Result{}, nil
		}
	}

	controllerutil.RemoveFinalizer(azureManagedMachinePool, infrav1.AzureManagedMachinePoolFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AzureManagedMachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	clusterToAzureManagedMachinePools, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &infrav1.AzureManagedMachinePoolList{}, mgr.GetScheme())
	if err != nil {
		return err
	}
	log := ctrl.LoggerFrom(ctx)

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.AzureManagedMachinePool{}).
		WithEventFilter(predicates.ResourceNotPaused(log)).
		Watches(
			&expv1.MachinePool{},
			handler.EnqueueRequestsFromMapFunc(utilexp.MachinePoolToInfrastructureMapFunc(
				infrav1.GroupVersion.WithKind("AzureManagedMachinePool"), log)),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToAzureManagedMachinePools),
			builder.WithPredicates(
				predicates.Any(log,
					ClusterUpdatePauseChange(log),
					predicates.ClusterControlPlaneInitialized(log),
				),
			),
		).
		Build(r)
	if err != nil {
		return err
	}
	r.controller = c

	r.externalTracker = &external.ObjectTracker{
		Cache:      mgr.GetCache(),
		Controller: c,
	}

	r.newResourceReconciler = func(azureManagedMachinePool *infrav1.AzureManagedMachinePool, resources []*unstructured.Unstructured) resourceReconciler {
		return &InfraReconciler{
			Client:    r.Client,
			resources: resources,
			owner:     azureManagedMachinePool,
			watcher:   r.externalTracker,
		}
	}

	return nil
}

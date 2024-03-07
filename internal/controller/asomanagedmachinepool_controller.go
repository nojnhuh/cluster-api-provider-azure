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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/remote"
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
	infrav1 "github.com/nojnhuh/cluster-api-provider-aso/api/v1alpha1"
)

// ASOManagedMachinePoolReconciler reconciles a ASOManagedMachinePool object
type ASOManagedMachinePoolReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Tracker         *remote.ClusterCacheTracker
	controller      controller.Controller
	externalTracker *external.ObjectTracker
}

//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=asomanagedmachinepools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=asomanagedmachinepools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=asomanagedmachinepools/finalizers,verbs=update

// Reconcile reconciles an ASOManagedMachinePool.
func (r *ASOManagedMachinePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, resultErr error) {
	log := log.FromContext(ctx)

	asoMachinePool := &infrav1.ASOManagedMachinePool{}
	err := r.Get(ctx, req.NamespacedName, asoMachinePool)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	machinePool, err := utilexp.GetOwnerMachinePool(ctx, r.Client, asoMachinePool.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machinePool == nil {
		log.Info("Waiting for MachinePool Controller to set OwnerRef on ASOManagedMachinePool")
		return ctrl.Result{}, nil
	}
	machinePoolBefore := machinePool.DeepCopy()

	log = log.WithValues("MachinePool", machinePool.Name)
	ctx = ctrl.LoggerInto(ctx, log)

	patchHelper, err := patch.NewHelper(asoMachinePool, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper: %w", err)
	}
	defer func() {
		if resultErr == nil {
			asoMachinePool.Status.ObservedGeneration = asoMachinePool.Generation
		}

		// Skip using a patch helper here because we will never modify the MachinePool status.
		err := r.Patch(ctx, machinePool, client.MergeFrom(machinePoolBefore))
		if err == nil {
			err = patchHelper.Patch(ctx, asoMachinePool)
			if !asoMachinePool.GetDeletionTimestamp().IsZero() {
				err = ignorePatchErrNotFound(err)
			}
		}

		if err != nil && resultErr == nil {
			resultErr = err
			result = ctrl.Result{}
		}
	}()

	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machinePool.ObjectMeta)
	if err != nil {
		log.Info("ASOManagedMachinePool owner MachinePool is missing cluster label or cluster does not exist")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info(fmt.Sprintf("Please associate this machine pool with a cluster using the label %s: <name of cluster>", clusterv1.ClusterNameLabel))
		return ctrl.Result{}, nil
	}
	if cluster.Spec.ControlPlaneRef == nil || cluster.Spec.ControlPlaneRef.Kind != "ASOManagedControlPlane" {
		return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("ASOManagedMachinePool cannot be used without ASOManagedControlPlane"))
	}

	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	if !asoMachinePool.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, asoMachinePool, cluster)
	}

	return r.reconcileNormal(ctx, asoMachinePool, machinePool, cluster)
}

func (r *ASOManagedMachinePoolReconciler) reconcileNormal(ctx context.Context, asoMachinePool *infrav1.ASOManagedMachinePool, machinePool *expv1.MachinePool, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	if controllerutil.AddFinalizer(asoMachinePool, infrav1.ASOManagedMachinePoolFinalizer) {
		return ctrl.Result{Requeue: true}, nil
	}

	// MachinePools created by ClusterClass have no spec.template.spec.bootstrap.dataSecretName defined.
	// TODO: see if we really need a kubeadm config for clusterclass machinepools.
	// // Make sure bootstrap data is available and populated.
	// if machinePool.Spec.Template.Spec.Bootstrap.DataSecretName == nil {
	// 	return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("ASOManagedMachinePool does not use spec.template.spec.bootstrap.dataSecretName, set this to any value in MachinePool %s/%s to continue", machinePool.Namespace, machinePool.Name))
	// }

	asoMachinePool.Status.Ready = false

	// TODO: extract this "get ASO resource from CAPASO resource" into a function
	var resources []runtime.RawExtension
	var ump *unstructured.Unstructured
	for _, resource := range asoMachinePool.Spec.Resources {
		u := &unstructured.Unstructured{}
		if err := u.UnmarshalJSON(resource.Raw); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to unmarshal resource JSON: %w", err)
		}
		u.SetNamespace(cluster.Namespace)
		if u.GroupVersionKind().Group == asocontainerservicev1.GroupVersion.Group &&
			u.GroupVersionKind().Kind == "ManagedClustersAgentPool" &&
			ump == nil {
			// TODO: do this in a webhook. Or not? maybe never let users set this in the ASO resource and
			// silently propagate it here so the CAPASO manifest doesn't have two fields that mean the same
			// thing where it's not obvious which one is authoritative?
			err := unstructured.SetNestedField(u.UnstructuredContent(), strings.TrimPrefix(ptr.Deref(machinePool.Spec.Template.Spec.Version, ""), "v"), "spec", "orchestratorVersion")
			if err != nil {
				return ctrl.Result{}, err
			}

			var count any
			autoscaling, _, err := unstructured.NestedBool(u.UnstructuredContent(), "spec", "enableAutoScaling")
			if err != nil {
				return ctrl.Result{}, err
			}
			if autoscaling {
				if machinePool.Annotations == nil {
					machinePool.Annotations = make(map[string]string)
				}
				machinePool.Annotations[clusterv1.ReplicasManagedByAnnotation] = "aks"
			} else {
				count = int64(ptr.Deref(machinePool.Spec.Replicas, 1))
				delete(machinePool.Annotations, clusterv1.ReplicasManagedByAnnotation)
			}
			err = unstructured.SetNestedField(u.UnstructuredContent(), count, "spec", "count")
			if err != nil {
				return ctrl.Result{}, err
			}

			ump = u
		}

		defaulted, err := u.MarshalJSON()
		if err != nil {
			return ctrl.Result{}, err
		}
		resources = append(resources, runtime.RawExtension{Raw: defaulted})
	}
	if ump == nil {
		// TODO: move this to a webhook.
		return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("no %s ManagedClustersAgentPools defined in ASOManagedMachinePool spec.resources", asocontainerservicev1.GroupVersion.Group))
	}

	infraReconciler := &InfraReconciler{
		Client:    r.Client,
		resources: resources,
		owner:     asoMachinePool,
		watcher:   r.externalTracker,
	}
	err := infraReconciler.Reconcile(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, status := range asoMachinePool.GetResourceStatuses() {
		if !status.Ready {
			return ctrl.Result{}, nil
		}
	}

	// get a typed resource so we don't have to try to convert this unstructured ourselves since it might be a
	// different API version than we know how to deal with.
	agentPool := &asocontainerservicev1.ManagedClustersAgentPool{}
	err = r.Get(ctx, client.ObjectKeyFromObject(ump), agentPool)
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
		client.MatchingLabels{
			"kubernetes.azure.com/agentpool": agentPool.AzureName(),
			"kubernetes.azure.com/cluster":   rg,
		},
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list nodes in workload cluster: %w", err)
	}
	providerIDs := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		providerIDs = append(providerIDs, node.Spec.ProviderID)
	}
	asoMachinePool.Spec.ProviderIDList = providerIDs

	asoMachinePool.Status.Replicas = int32(ptr.Deref(agentPool.Status.Count, 0))
	if auto, ok := machinePool.Annotations[clusterv1.ReplicasManagedByAnnotation]; ok && auto != "false" {
		machinePool.Spec.Replicas = &asoMachinePool.Status.Replicas
	}

	asoMachinePool.Status.Ready = true

	return ctrl.Result{}, nil
}

func (r *ASOManagedMachinePoolReconciler) reconcileDelete(ctx context.Context, asoMachinePool *infrav1.ASOManagedMachinePool, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	infraReconciler := &InfraReconciler{
		Client:    r.Client,
		resources: asoMachinePool.Spec.Resources,
		owner:     asoMachinePool,
		watcher:   r.externalTracker,
	}

	// If the entire cluster is being deleted, this ASO ManagedClustersAgentPool will be deleted with the rest
	// of the ManagedCluster.
	if cluster.DeletionTimestamp.IsZero() {
		err := infraReconciler.Delete(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}
		if len(asoMachinePool.GetResourceStatuses()) > 0 {
			// waiting for resources to be deleted
			return ctrl.Result{}, nil
		}
	}

	controllerutil.RemoveFinalizer(asoMachinePool, infrav1.ASOManagedMachinePoolFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ASOManagedMachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	clusterToASOMachinePools, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &infrav1.ASOManagedMachinePoolList{}, mgr.GetScheme())
	if err != nil {
		return err
	}
	log := ctrl.LoggerFrom(ctx)

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.ASOManagedMachinePool{}).
		WithEventFilter(predicates.ResourceNotPaused(log)).
		Watches(
			&expv1.MachinePool{},
			handler.EnqueueRequestsFromMapFunc(utilexp.MachinePoolToInfrastructureMapFunc(
				infrav1.GroupVersion.WithKind("ASOManagedMachinePool"), log)),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToASOMachinePools),
			builder.WithPredicates(
				predicates.Any(log,
					predicates.ClusterCreateNotPaused(log),
					predicates.ClusterUpdateUnpaused(log),
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

	return nil
}

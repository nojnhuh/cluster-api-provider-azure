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
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	azprovider "sigs.k8s.io/cloud-provider-azure/pkg/provider"
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
	infraReconciler *InfraReconciler
	externalTracker *external.ObjectTracker
	vmssLister      vmssLister
	vmssName        string
}

type vmssLister interface {
	ListVMSS(ctx context.Context, resourceGroup string) ([]*armcompute.VirtualMachineScaleSet, error)
	ListVMSSInstances(ctx context.Context, resourceGroup, vmssName string) ([]*armcompute.VirtualMachineScaleSetVM, error)
}

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

	patchHelper, err := patch.NewHelper(asoMachinePool, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper: %w", err)
	}
	defer func() {
		if resultErr == nil {
			asoMachinePool.Status.ObservedGeneration = asoMachinePool.Generation
		}

		err := patchHelper.Patch(ctx, asoMachinePool)
		if !asoMachinePool.GetDeletionTimestamp().IsZero() {
			err = ignorePatchErrNotFound(err)
		}
		if err != nil && resultErr == nil {
			resultErr = err
			result = ctrl.Result{}
		}
	}()

	asoMachinePool.Status.Ready = false

	machinePool, err := utilexp.GetOwnerMachinePool(ctx, r.Client, asoMachinePool.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machinePool == nil {
		log.Info("Waiting for MachinePool Controller to set OwnerRef on ASOManagedMachinePool")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("MachinePool", machinePool.Name)
	ctx = ctrl.LoggerInto(ctx, log)

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

	r.infraReconciler = &InfraReconciler{
		Client:          r.Client,
		resources:       asoMachinePool.Spec.Resources,
		owner:           asoMachinePool,
		externalTracker: r.externalTracker,
	}

	if !asoMachinePool.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, asoMachinePool, cluster)
	}

	return r.reconcileNormal(ctx, asoMachinePool, machinePool, cluster)
}

func (r *ASOManagedMachinePoolReconciler) reconcileNormal(ctx context.Context, asoMachinePool *infrav1.ASOManagedMachinePool, machinePool *expv1.MachinePool, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	if controllerutil.AddFinalizer(asoMachinePool, infrav1.ASOManagedMachinePoolFinalizer) {
		return ctrl.Result{Requeue: true}, nil
	}

	// Make sure bootstrap data is available and populated.
	if machinePool.Spec.Template.Spec.Bootstrap.DataSecretName == nil {
		return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("ASOManagedMachinePool does not use spec.template.spec.bootstrap.dataSecretName, set this to any value in MachinePool %s/%s to continue", machinePool.Namespace, machinePool.Name))
	}

	var ump *unstructured.Unstructured
	for i, resource := range asoMachinePool.Spec.Resources {
		u := &unstructured.Unstructured{}
		if err := u.UnmarshalJSON(resource.Raw); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to unmarshal resource JSON: %w", err)
		}
		u.SetNamespace(cluster.Namespace)
		if u.GroupVersionKind().Group == asocontainerservicev1.GroupVersion.Group &&
			u.GroupVersionKind().Kind == "ManagedClustersAgentPool" {
			// TODO: autoscaling
			// TODO: do this in a webhook. Or not? maybe never let users set this in the ASO resource and
			// silently propagate it here so the CAPASO manifest doesn't have two fields that mean the same
			// thing where it's not obvious which one is authoritative?
			err := unstructured.SetNestedField(u.UnstructuredContent(), int64(ptr.Deref(machinePool.Spec.Replicas, 1)), "spec", "count")
			if err != nil {
				return ctrl.Result{}, err
			}
			err = unstructured.SetNestedField(u.UnstructuredContent(), strings.TrimPrefix(ptr.Deref(machinePool.Spec.Template.Spec.Version, ""), "v"), "spec", "orchestratorVersion")
			if err != nil {
				return ctrl.Result{}, err
			}
			asoMachinePool.Spec.Resources[i].Raw, err = u.MarshalJSON()
			if err != nil {
				return ctrl.Result{}, err
			}
			ump = u
			break
		}
	}
	if ump == nil {
		// TODO: move this to a webhook.
		return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("no %s ManagedClustersAgentPools defined in ASOManagedMachinePool spec.resources", asocontainerservicev1.GroupVersion.Group))
	}

	err := r.infraReconciler.Reconcile(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, status := range asoMachinePool.GetResourceStatuses() {
		// TODO: status.ready isn't reporting false while changes are being reconciled
		ctrl.LoggerFrom(ctx).Info("status for resource", "name", status.Name, "ready", status.Ready, "object", asoMachinePool.Status.Ready)
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

	managedCluster := &asocontainerservicev1.ManagedCluster{}
	err = r.Get(ctx, client.ObjectKey{Namespace: agentPool.Namespace, Name: agentPool.Owner().Name}, managedCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error getting ManagedCluster: %w", err)
	}
	if managedCluster.Status.NodeResourceGroup == nil {
		return ctrl.Result{}, nil
	}
	rg := *managedCluster.Status.NodeResourceGroup

	if r.vmssName == "" {
		vmsss, err := r.vmssLister.ListVMSS(ctx, rg)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to list VMSSes in resource group %s: %w", rg, err)
		}
		var vmss *armcompute.VirtualMachineScaleSet
		for _, v := range vmsss {
			if ptr.Deref(v.Tags["aks-managed-poolName"], "") == agentPool.AzureName() {
				vmss = v
				break
			}
		}
		if vmss == nil {
			return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("VMSS not found for ASOManagedMachinePool %s/%s in resource group %s", asoMachinePool.Namespace, asoMachinePool.Name, rg))
		}
		r.vmssName = *vmss.Name
	}

	instances, err := r.vmssLister.ListVMSSInstances(ctx, rg, r.vmssName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list VMSS instances: %w", err)
	}
	asoMachinePool.Spec.ProviderIDList = make([]string, 0, len(instances))
	for _, instance := range instances {
		providerID, err := azprovider.ConvertResourceGroupNameToLower("azure://" + *instance.ID)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to parse instance ID %s: %w", *instance.ID, err)
		}
		asoMachinePool.Spec.ProviderIDList = append(asoMachinePool.Spec.ProviderIDList, providerID)
	}
	asoMachinePool.Status.Replicas = int32(ptr.Deref(agentPool.Status.Count, 0))

	asoMachinePool.Status.Ready = true

	return ctrl.Result{}, nil
}

func (r *ASOManagedMachinePoolReconciler) reconcileDelete(ctx context.Context, asoMachinePool *infrav1.ASOManagedMachinePool, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	err := r.infraReconciler.Delete(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(asoMachinePool.GetResourceStatuses()) > 0 {
		// waiting for resources to be deleted
		return ctrl.Result{}, nil
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

	r.externalTracker = &external.ObjectTracker{
		Cache:      mgr.GetCache(),
		Controller: c,
	}

	// TODO: move these creds
	subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return err
	}
	computeClientFactory, err := armcompute.NewClientFactory(subscriptionID, cred, nil)
	if err != nil {
		return err
	}
	r.vmssLister = &sdkVMSSLister{
		vmss: computeClientFactory.NewVirtualMachineScaleSetsClient(),
		vm:   computeClientFactory.NewVirtualMachineScaleSetVMsClient(),
	}

	return nil
}

type sdkVMSSLister struct {
	vmss *armcompute.VirtualMachineScaleSetsClient
	vm   *armcompute.VirtualMachineScaleSetVMsClient
}

func (c *sdkVMSSLister) ListVMSS(ctx context.Context, resourceGroup string) ([]*armcompute.VirtualMachineScaleSet, error) {
	ctrl.LoggerFrom(ctx).Info("DOING AZURE API CALLS HOLD YOUR HORSES LIST VMSS")
	// TODO: throttle these calls
	var vmss []*armcompute.VirtualMachineScaleSet
	pager := c.vmss.NewListPager(resourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		vmss = append(vmss, page.Value...)
	}
	return vmss, nil
}

func (c *sdkVMSSLister) ListVMSSInstances(ctx context.Context, resourceGroup string, vmssName string) ([]*armcompute.VirtualMachineScaleSetVM, error) {
	ctrl.LoggerFrom(ctx).Info("DOING AZURE API CALLS HOLD YOUR HORSES LIST VMSS INSTANCES")
	// TODO: throttle these calls
	var vms []*armcompute.VirtualMachineScaleSetVM
	pager := c.vm.NewListPager(resourceGroup, vmssName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		vms = append(vms, page.Value...)
	}
	return vms, nil
}

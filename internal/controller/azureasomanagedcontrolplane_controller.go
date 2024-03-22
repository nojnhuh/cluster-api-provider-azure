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

	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20231001"
	"github.com/Azure/azure-service-operator/v2/pkg/genruntime"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	"sigs.k8s.io/cluster-api/util/secret"
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

var (
	invalidClusterKindErr = errors.New("AzureASOManagedControlPlane cannot be used without AzureASOManagedCluster")
)

// AzureASOManagedControlPlaneReconciler reconciles a AzureASOManagedControlPlane object
type AzureASOManagedControlPlaneReconciler struct {
	client.Client
	Scheme                *runtime.Scheme // TODO: do we need this? Should this ever be different from Client.GetScheme()?
	externalTracker       *external.ObjectTracker
	newResourceReconciler func(*infrav1.AzureASOManagedControlPlane, []*unstructured.Unstructured) resourceReconciler
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasomanagedcontrolplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasomanagedcontrolplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureasomanagedcontrolplanes/finalizers,verbs=update

// Reconcile reconciles an AzureASOManagedControlPlane
func (r *AzureASOManagedControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, resultErr error) {
	asoManagedControlPlane := &infrav1.AzureASOManagedControlPlane{}
	err := r.Get(ctx, req.NamespacedName, asoManagedControlPlane)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(asoManagedControlPlane, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper: %w", err)
	}
	defer func() {
		r.reconcileConditions(asoManagedControlPlane, resultErr)

		opts := []patch.Option{
			patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				clusterv1.ReadyCondition,
				infrav1.Reconciled,
				infrav1.ResourcesReady,
			}},
		}
		if resultErr == nil {
			opts = append(opts, patch.WithStatusObservedGeneration{})
		}

		err := patchHelper.Patch(ctx, asoManagedControlPlane, opts...)
		if !asoManagedControlPlane.GetDeletionTimestamp().IsZero() {
			err = ignorePatchErrNotFound(err)
		}
		if err != nil && resultErr == nil {
			resultErr = err
			result = ctrl.Result{}
		}
	}()

	asoManagedControlPlane.Status.ExternalManagedControlPlane = true

	cluster, err := util.GetOwnerCluster(ctx, r.Client, asoManagedControlPlane.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !asoManagedControlPlane.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, asoManagedControlPlane, cluster)
	}

	return r.reconcileNormal(ctx, asoManagedControlPlane, cluster)
}

func (r *AzureASOManagedControlPlaneReconciler) reconcileNormal(ctx context.Context, asoManagedControlPlane *infrav1.AzureASOManagedControlPlane, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}
	if cluster.Spec.InfrastructureRef == nil || cluster.Spec.InfrastructureRef.Kind != "AzureASOManagedCluster" {
		return ctrl.Result{}, reconcile.TerminalError(invalidClusterKindErr)
	}

	needsPatch := controllerutil.AddFinalizer(asoManagedControlPlane, clusterv1.ClusterFinalizer)
	if !cluster.Spec.Paused {
		needsPatch = addBlockMoveAnnotation(asoManagedControlPlane) || needsPatch
	}
	if needsPatch {
		return ctrl.Result{Requeue: true}, nil
	}

	resources, err := mutators.ApplyMutators(ctx, asoManagedControlPlane.Spec.Resources,
		mutators.SetASOReconciliationPolicy(cluster),
		mutators.SetManagedClusterDefaults(r.Client, asoManagedControlPlane, cluster),
	)
	if err != nil {
		// TODO: watch AzureASOManagedMachinePools instead of requeueing here? Or maybe this is good enough?
		if errors.Is(err, mutators.NoAzureASOManagedMachinePoolsErr) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	// Mutations are not persisted to keep the ClusterClass controller from trying to overwrite them.

	var managedClusterName string
	for _, resource := range resources {
		if resource.GroupVersionKind().Group == asocontainerservicev1.GroupVersion.Group &&
			resource.GroupVersionKind().Kind == "ManagedCluster" {
			managedClusterName = resource.GetName()
			break
		}
	}
	if managedClusterName == "" {
		return ctrl.Result{}, reconcile.TerminalError(mutators.NoManagedClusterDefinedErr)
	}

	infraReconciler := r.newResourceReconciler(asoManagedControlPlane, resources)
	err = infraReconciler.Reconcile(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, status := range asoManagedControlPlane.GetResourceStatuses() {
		if status.Condition.Status != metav1.ConditionTrue {
			return ctrl.Result{}, nil
		}
	}

	if cluster.Spec.Paused {
		removeBlockMoveAnnotation(asoManagedControlPlane)
	}

	// get a typed resource so we don't have to try to convert this unstructured ourselves since it might be a
	// different API version than we know how to deal with.
	managedCluster := &asocontainerservicev1.ManagedCluster{}
	err = r.Get(ctx, client.ObjectKey{Namespace: asoManagedControlPlane.Namespace, Name: managedClusterName}, managedCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error getting ManagedCluster: %w", err)
	}

	if managedCluster.Status.Fqdn != nil {
		asoManagedControlPlane.Status.ControlPlaneEndpoint = clusterv1.APIEndpoint{
			Host: ptr.Deref(managedCluster.Status.Fqdn, ""),
			Port: 443,
		}
	}
	if managedCluster.Status.ApiServerAccessProfile != nil &&
		ptr.Deref(managedCluster.Status.ApiServerAccessProfile.EnablePrivateCluster, false) &&
		!ptr.Deref(managedCluster.Status.ApiServerAccessProfile.EnablePrivateClusterPublicFQDN, false) {
		asoManagedControlPlane.Status.ControlPlaneEndpoint = clusterv1.APIEndpoint{
			Host: ptr.Deref(managedCluster.Status.PrivateFQDN, ""),
			Port: 443,
		}
	}

	if managedCluster.Status.CurrentKubernetesVersion != nil {
		asoManagedControlPlane.Status.Version = "v" + *managedCluster.Status.CurrentKubernetesVersion
	}

	err = r.reconcileKubeconfig(ctx, asoManagedControlPlane, cluster, managedCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile kubeconfig: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *AzureASOManagedControlPlaneReconciler) reconcileKubeconfig(ctx context.Context, asoManagedControlPlane *infrav1.AzureASOManagedControlPlane, cluster *clusterv1.Cluster, managedCluster *asocontainerservicev1.ManagedCluster) error {
	var secretRef *genruntime.SecretDestination
	if managedCluster.Spec.OperatorSpec != nil &&
		managedCluster.Spec.OperatorSpec.Secrets != nil {
		secretRef = managedCluster.Spec.OperatorSpec.Secrets.UserCredentials
		if managedCluster.Spec.OperatorSpec.Secrets.AdminCredentials != nil {
			secretRef = managedCluster.Spec.OperatorSpec.Secrets.AdminCredentials
		}
	}
	if secretRef == nil {
		return reconcile.TerminalError(fmt.Errorf("ManagedCluster must define at least one of spec.operatorSpec.secrets.{userCredentials,adminCredentials}"))
	}
	asoKubeconfig := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: secretRef.Name}, asoKubeconfig)
	if err != nil {
		err = fmt.Errorf("failed to fetch secret created by ASO: %w", err)
		if apierrors.IsNotFound(err) {
			// we will requeue when ASO finishes creating the secret
			return reconcile.TerminalError(err)
		}
		return err
	}

	expectedSecret := &corev1.Secret{
		TypeMeta: asoKubeconfig.TypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name(cluster.Name, secret.Kubeconfig),
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(asoManagedControlPlane, infrav1.GroupVersion.WithKind("AzureASOManagedControlPlane")),
			},
			Labels: map[string]string{clusterv1.ClusterNameLabel: cluster.Name},
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: asoKubeconfig.Data[secretRef.Key],
		},
	}

	return r.Patch(ctx, expectedSecret, client.Apply, client.FieldOwner("capz-manager"), client.ForceOwnership)
}

func (r *AzureASOManagedControlPlaneReconciler) reconcileConditions(asoManagedControlPlane *infrav1.AzureASOManagedControlPlane, resultErr error) {
	reconcileResourcesReadyCondition(asoManagedControlPlane, asoManagedControlPlane.Spec.Resources)
	reconcileResultErr(asoManagedControlPlane, resultErr)
	conditions.SetSummary(asoManagedControlPlane)
	asoManagedControlPlane.Status.Ready = conditions.IsTrue(asoManagedControlPlane, clusterv1.ReadyCondition)
	asoManagedControlPlane.Status.Initialized = asoManagedControlPlane.Status.Ready
}

func (r *AzureASOManagedControlPlaneReconciler) reconcileDelete(ctx context.Context, asoManagedControlPlane *infrav1.AzureASOManagedControlPlane, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	if cluster == nil {
		// Cluster owner ref not set
		controllerutil.RemoveFinalizer(asoManagedControlPlane, clusterv1.ClusterFinalizer)
		return ctrl.Result{}, nil
	}

	resources, err := mutators.ApplyMutators(ctx, asoManagedControlPlane.Spec.Resources)
	if err != nil {
		return ctrl.Result{}, err
	}
	infraReconciler := r.newResourceReconciler(asoManagedControlPlane, resources)

	err = infraReconciler.Delete(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(asoManagedControlPlane.GetResourceStatuses()) > 0 {
		// waiting for resources to be deleted
		return ctrl.Result{}, nil
	}

	controllerutil.RemoveFinalizer(asoManagedControlPlane, clusterv1.ClusterFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AzureASOManagedControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	log := ctrl.LoggerFrom(ctx)
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.AzureASOManagedControlPlane{}).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToAzureASOManagedControlPlane),
			builder.WithPredicates(
				predicates.Any(
					log,
					predicates.ClusterCreateInfraReady(log),
					predicates.ClusterUpdateInfraReady(log),
					ClusterUpdatePauseChange(log),
				),
			),
		).
		// User errors that CAPZ passes through agentPoolProfiles on create must be fixed in the
		// AzureASOManagedMachinePool, so trigger a reconciliation to consume those fixes.
		Watches(
			&infrav1.AzureASOManagedMachinePool{},
			handler.EnqueueRequestsFromMapFunc(r.azureASOManagedMachinePoolToAzureASOManagedControlPlane),
			// TODO: add predicates so we only enqueue when the AzureASOManagedControlPlane is in charge of the
			// agent pools (when the ManagedCluster's status.agentPoolProfile is empty).
		).
		Owns(&corev1.Secret{}).
		Build(r)
	if err != nil {
		return err
	}

	r.externalTracker = &external.ObjectTracker{
		Cache:      mgr.GetCache(),
		Controller: c,
	}

	r.newResourceReconciler = func(asoManagedControlPlane *infrav1.AzureASOManagedControlPlane, resources []*unstructured.Unstructured) resourceReconciler {
		return &InfraReconciler{
			Client:    r.Client,
			resources: resources,
			owner:     asoManagedControlPlane,
			watcher:   r.externalTracker,
		}
	}

	return nil
}

// clusterToAzureASOManagedControlPlane is a handler.ToRequestsFunc to be used to enqueue requests for
// reconciliation for AzureASOManagedControlPlane based on updates to a Cluster.
func clusterToAzureASOManagedControlPlane(_ context.Context, o client.Object) []ctrl.Request {
	controlPlaneRef := o.(*clusterv1.Cluster).Spec.ControlPlaneRef
	if controlPlaneRef != nil && controlPlaneRef.Kind == "AzureASOManagedControlPlane" {
		return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: controlPlaneRef.Namespace, Name: controlPlaneRef.Name}}}
	}
	return nil
}

func (r *AzureASOManagedControlPlaneReconciler) azureASOManagedMachinePoolToAzureASOManagedControlPlane(ctx context.Context, o client.Object) []ctrl.Request {
	asoManagedMachinePool := o.(*infrav1.AzureASOManagedMachinePool)
	clusterName := asoManagedMachinePool.Labels[clusterv1.ClusterNameLabel]
	if clusterName == "" {
		return nil
	}
	cluster, err := util.GetClusterByName(ctx, r.Client, asoManagedMachinePool.Namespace, clusterName)
	if client.IgnoreNotFound(err) != nil || cluster == nil {
		return nil
	}
	return clusterToAzureASOManagedControlPlane(ctx, cluster)
}

// ClusterUpdatePauseChange returns a predicate that returns true for an update event when a cluster has
// Spec.Paused changed or when a cluster is created in order to manage the clusterctl block-move annotation.
func ClusterUpdatePauseChange(logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log := logger.WithValues("predicate", "ClusterUpdateUnpaused", "eventType", "update")

			oldCluster, ok := e.ObjectOld.(*clusterv1.Cluster)
			if !ok {
				log.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.ObjectOld))
				return false
			}
			log = log.WithValues("Cluster", klog.KObj(oldCluster))

			newCluster := e.ObjectNew.(*clusterv1.Cluster)

			if oldCluster.Spec.Paused != newCluster.Spec.Paused {
				log.V(4).Info("Cluster spec.paused was changed, allowing further processing")
				return true
			}

			log.V(6).Info("Cluster spec.paused was not changed, blocking further processing")
			return false
		},
		CreateFunc: func(event.CreateEvent) bool { return true },
		DeleteFunc: func(event.DeleteEvent) bool { return false },
	}
}

// addBlockMoveAnnotation adds CAPI's block-move annotation and returns whether or not the annotation was added.
func addBlockMoveAnnotation(obj metav1.Object) bool {
	annotations := obj.GetAnnotations()

	if _, exists := annotations[clusterctlv1.BlockMoveAnnotation]; exists {
		return false
	}

	if annotations == nil {
		annotations = make(map[string]string)
	}

	// this value doesn't mean anything, only the presence of the annotation matters.
	annotations[clusterctlv1.BlockMoveAnnotation] = "true"
	obj.SetAnnotations(annotations)

	return true
}

// removeBlockMoveAnnotation removes CAPI's block-move annotation.
func removeBlockMoveAnnotation(obj metav1.Object) {
	annotations := obj.GetAnnotations()
	delete(annotations, clusterctlv1.BlockMoveAnnotation)
	obj.SetAnnotations(annotations)
}

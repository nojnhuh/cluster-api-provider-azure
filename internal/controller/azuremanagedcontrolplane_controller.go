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

	infrav1 "sigs.k8s.io/cluster-api-provider-azure/v2/api/v2alpha1"
	"sigs.k8s.io/cluster-api-provider-azure/v2/internal/mutators"
)

var (
	invalidClusterKindErr = errors.New("AzureManagedControlPlane cannot be used without AzureManagedCluster")
)

// AzureManagedControlPlaneReconciler reconciles a AzureManagedControlPlane object
type AzureManagedControlPlaneReconciler struct {
	client.Client
	Scheme                *runtime.Scheme // TODO: do we need this? Should this ever be different from Client.GetScheme()?
	externalTracker       *external.ObjectTracker
	newResourceReconciler func(*infrav1.AzureManagedControlPlane, []*unstructured.Unstructured) resourceReconciler
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azuremanagedcontrolplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azuremanagedcontrolplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azuremanagedcontrolplanes/finalizers,verbs=update

// Reconcile reconciles an AzureManagedControlPlane
func (r *AzureManagedControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, resultErr error) {
	azureManagedControlPlane := &infrav1.AzureManagedControlPlane{}
	err := r.Get(ctx, req.NamespacedName, azureManagedControlPlane)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(azureManagedControlPlane, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper: %w", err)
	}
	defer func() {
		if resultErr == nil {
			azureManagedControlPlane.Status.ObservedGeneration = azureManagedControlPlane.Generation
		}

		err := patchHelper.Patch(ctx, azureManagedControlPlane)
		if !azureManagedControlPlane.GetDeletionTimestamp().IsZero() {
			err = ignorePatchErrNotFound(err)
		}
		if err != nil && resultErr == nil {
			resultErr = err
			result = ctrl.Result{}
		}
	}()

	azureManagedControlPlane.Status.ExternalManagedControlPlane = true

	cluster, err := util.GetOwnerCluster(ctx, r.Client, azureManagedControlPlane.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !azureManagedControlPlane.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, azureManagedControlPlane, cluster)
	}

	return r.reconcileNormal(ctx, azureManagedControlPlane, cluster)
}

func (r *AzureManagedControlPlaneReconciler) reconcileNormal(ctx context.Context, azureManagedControlPlane *infrav1.AzureManagedControlPlane, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}
	if cluster.Spec.InfrastructureRef == nil || cluster.Spec.InfrastructureRef.Kind != "AzureManagedCluster" {
		return ctrl.Result{}, reconcile.TerminalError(invalidClusterKindErr)
	}

	needsPatch := controllerutil.AddFinalizer(azureManagedControlPlane, clusterv1.ClusterFinalizer)
	if !cluster.Spec.Paused {
		needsPatch = addBlockMoveAnnotation(azureManagedControlPlane) || needsPatch
	}
	if needsPatch {
		return ctrl.Result{Requeue: true}, nil
	}

	azureManagedControlPlane.Status.Ready = false
	azureManagedControlPlane.Status.Initialized = azureManagedControlPlane.Status.Ready

	resources, err := mutators.ApplyMutators(ctx, azureManagedControlPlane.Spec.Resources,
		mutators.SetASOReconciliationPolicy(cluster),
		mutators.SetManagedClusterDefaults(r.Client, azureManagedControlPlane, cluster),
	)
	if err != nil {
		// TODO: watch AzureManagedMachinePools instead of requeueing here? Or maybe this is good enough?
		if errors.Is(err, mutators.NoAzureManagedMachinePoolsErr) {
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

	infraReconciler := r.newResourceReconciler(azureManagedControlPlane, resources)
	err = infraReconciler.Reconcile(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, status := range azureManagedControlPlane.GetResourceStatuses() {
		if !status.Ready {
			return ctrl.Result{}, nil
		}
	}

	if cluster.Spec.Paused {
		removeBlockMoveAnnotation(azureManagedControlPlane)
	}

	// get a typed resource so we don't have to try to convert this unstructured ourselves since it might be a
	// different API version than we know how to deal with.
	managedCluster := &asocontainerservicev1.ManagedCluster{}
	err = r.Get(ctx, client.ObjectKey{Namespace: azureManagedControlPlane.Namespace, Name: managedClusterName}, managedCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error getting ManagedCluster: %w", err)
	}

	if managedCluster.Status.Fqdn != nil {
		azureManagedControlPlane.Status.ControlPlaneEndpoint = clusterv1.APIEndpoint{
			Host: ptr.Deref(managedCluster.Status.Fqdn, ""),
			Port: 443,
		}
	}
	if managedCluster.Status.ApiServerAccessProfile != nil &&
		ptr.Deref(managedCluster.Status.ApiServerAccessProfile.EnablePrivateCluster, false) &&
		!ptr.Deref(managedCluster.Status.ApiServerAccessProfile.EnablePrivateClusterPublicFQDN, false) {
		azureManagedControlPlane.Status.ControlPlaneEndpoint = clusterv1.APIEndpoint{
			Host: ptr.Deref(managedCluster.Status.PrivateFQDN, ""),
			Port: 443,
		}
	}

	if managedCluster.Status.CurrentKubernetesVersion != nil {
		azureManagedControlPlane.Status.Version = "v" + *managedCluster.Status.CurrentKubernetesVersion
	}

	err = r.reconcileKubeconfig(ctx, azureManagedControlPlane, cluster, managedCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile kubeconfig: %w", err)
	}

	azureManagedControlPlane.Status.Ready = true
	azureManagedControlPlane.Status.Initialized = azureManagedControlPlane.Status.Ready

	return ctrl.Result{}, nil
}

func (r *AzureManagedControlPlaneReconciler) reconcileKubeconfig(ctx context.Context, azureManagedControlPlane *infrav1.AzureManagedControlPlane, cluster *clusterv1.Cluster, managedCluster *asocontainerservicev1.ManagedCluster) error {
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
				*metav1.NewControllerRef(azureManagedControlPlane, infrav1.GroupVersion.WithKind("AzureManagedControlPlane")),
			},
			Labels: map[string]string{clusterv1.ClusterNameLabel: cluster.Name},
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: asoKubeconfig.Data[secretRef.Key],
		},
	}

	return r.Patch(ctx, expectedSecret, client.Apply, client.FieldOwner("capz-manager"), client.ForceOwnership)
}

func (r *AzureManagedControlPlaneReconciler) reconcileDelete(ctx context.Context, azureManagedControlPlane *infrav1.AzureManagedControlPlane, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	if cluster == nil {
		// Cluster owner ref not set
		controllerutil.RemoveFinalizer(azureManagedControlPlane, clusterv1.ClusterFinalizer)
		return ctrl.Result{}, nil
	}

	resources, err := mutators.ApplyMutators(ctx, azureManagedControlPlane.Spec.Resources)
	if err != nil {
		return ctrl.Result{}, err
	}
	infraReconciler := r.newResourceReconciler(azureManagedControlPlane, resources)

	err = infraReconciler.Delete(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(azureManagedControlPlane.GetResourceStatuses()) > 0 {
		// waiting for resources to be deleted
		return ctrl.Result{}, nil
	}

	controllerutil.RemoveFinalizer(azureManagedControlPlane, clusterv1.ClusterFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AzureManagedControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	log := ctrl.LoggerFrom(ctx)
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.AzureManagedControlPlane{}).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.ClusterToKubeadmControlPlane),
			builder.WithPredicates(
				predicates.Any(
					log,
					predicates.ClusterCreateInfraReady(log),
					predicates.ClusterUpdateInfraReady(log),
					ClusterUpdatePauseChange(log),
				),
			),
		).
		// TODO: watch AzureManagedMachinePools. User errors that end CAPZ passes through agentPoolProfiles on
		// create must be fixed in the AzureManagedMachinePool, but no AzureManagedControlPlane reconciliation
		// is triggered.
		Owns(&corev1.Secret{}).
		Build(r)
	if err != nil {
		return err
	}

	r.externalTracker = &external.ObjectTracker{
		Cache:      mgr.GetCache(),
		Controller: c,
	}

	r.newResourceReconciler = func(azureManagedControlPlane *infrav1.AzureManagedControlPlane, resources []*unstructured.Unstructured) resourceReconciler {
		return &InfraReconciler{
			Client:    r.Client,
			resources: resources,
			owner:     azureManagedControlPlane,
			watcher:   r.externalTracker,
		}
	}

	return nil
}

// ClusterToAzureManagedControlPlane is a handler.ToRequestsFunc to be used to enqueue requests for
// reconciliation for AzureManagedControlPlane based on updates to a Cluster.
func (r *AzureManagedControlPlaneReconciler) ClusterToKubeadmControlPlane(_ context.Context, o client.Object) []ctrl.Request {
	controlPlaneRef := o.(*clusterv1.Cluster).Spec.ControlPlaneRef
	if controlPlaneRef != nil && controlPlaneRef.Kind == "AzureManagedControlPlane" {
		return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: controlPlaneRef.Namespace, Name: controlPlaneRef.Name}}}
	}

	return nil
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
		CreateFunc: func(e event.CreateEvent) bool { return true },
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

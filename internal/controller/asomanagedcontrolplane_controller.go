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

	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20231001"
	"github.com/Azure/azure-service-operator/v2/pkg/genruntime"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	"sigs.k8s.io/cluster-api/util/secret"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1 "github.com/nojnhuh/cluster-api-provider-aso/api/v1alpha1"
)

// ASOManagedControlPlaneReconciler reconciles a ASOManagedControlPlane object
type ASOManagedControlPlaneReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	externalTracker *external.ObjectTracker
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=asomanagedcontrolplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=asomanagedcontrolplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=asomanagedcontrolplanes/finalizers,verbs=update

// Reconcile reconciles an ASOManagedControlPlane
func (r *ASOManagedControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, resultErr error) {
	asoControlPlane := &infrav1.ASOManagedControlPlane{}
	err := r.Get(ctx, req.NamespacedName, asoControlPlane)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(asoControlPlane, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper: %w", err)
	}
	defer func() {
		if resultErr == nil {
			asoControlPlane.Status.ObservedGeneration = asoControlPlane.Generation
		}

		err := patchHelper.Patch(ctx, asoControlPlane)
		if !asoControlPlane.GetDeletionTimestamp().IsZero() {
			err = ignorePatchErrNotFound(err)
		}
		if err != nil && resultErr == nil {
			resultErr = err
			result = ctrl.Result{}
		}
	}()

	asoControlPlane.Status.ExternalManagedControlPlane = true

	cluster, err := util.GetOwnerCluster(ctx, r.Client, asoControlPlane.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !asoControlPlane.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, asoControlPlane, cluster)
	}

	return r.reconcileNormal(ctx, asoControlPlane, cluster)
}

func (r *ASOManagedControlPlaneReconciler) reconcileNormal(ctx context.Context, asoControlPlane *infrav1.ASOManagedControlPlane, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}
	if cluster.Spec.InfrastructureRef == nil || cluster.Spec.InfrastructureRef.Kind != "ASOManagedCluster" {
		return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("ASOManagedControlPlane cannot be used without ASOManagedCluster"))
	}

	if controllerutil.AddFinalizer(asoControlPlane, clusterv1.ClusterFinalizer) {
		return ctrl.Result{Requeue: true}, nil
	}

	asoControlPlane.Status.Ready = false
	asoControlPlane.Status.Initialized = asoControlPlane.Status.Ready

	// resources is a copy of the resources as they are defined in the spec, with some fields defaulted based
	// on other fields that are defined in the CAPI contract. These defaults are not persisted to keep the
	// ClusterClass controller from trying to overwrite them.
	var resources []runtime.RawExtension
	var managedClusterName string
	for _, resource := range asoControlPlane.Spec.Resources {
		u := &unstructured.Unstructured{}
		if err := u.UnmarshalJSON(resource.Raw); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to unmarshal resource JSON: %w", err)
		}
		if u.GroupVersionKind().Group == asocontainerservicev1.GroupVersion.Group &&
			u.GroupVersionKind().Kind == "ManagedCluster" &&
			managedClusterName == "" {
			managedClusterName = u.GetName()
			// TODO: default this in a webhook. Check to make sure ClusterClass controller doesn't fight with
			// a webhook-defaulted value
			// TODO: auto upgrades?
			err := unstructured.SetNestedField(u.UnstructuredContent(), strings.TrimPrefix(asoControlPlane.Spec.Version, "v"), "spec", "kubernetesVersion")
			if err != nil {
				return ctrl.Result{}, err
			}
		}

		defaulted, err := u.MarshalJSON()
		if err != nil {
			return ctrl.Result{}, err
		}
		resources = append(resources, runtime.RawExtension{Raw: defaulted})
	}
	if managedClusterName == "" {
		// TODO: move this to a webhook.
		return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("no %s ManagedCluster defined in ASOManagedControlPlane spec.resources", asocontainerservicev1.GroupVersion.Group))
	}

	infraReconciler := &InfraReconciler{
		Client:          r.Client,
		resources:       resources,
		owner:           asoControlPlane,
		externalTracker: r.externalTracker,
	}
	err := infraReconciler.Reconcile(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, status := range asoControlPlane.GetResourceStatuses() {
		if !status.Ready {
			return ctrl.Result{}, nil
		}
	}

	// get a typed resource so we don't have to try to convert this unstructured ourselves since it might be a
	// different API version than we know how to deal with.
	managedCluster := &asocontainerservicev1.ManagedCluster{}
	err = r.Get(ctx, client.ObjectKey{Namespace: asoControlPlane.Namespace, Name: managedClusterName}, managedCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error getting ManagedCluster: %w", err)
	}

	if managedCluster.Status.Fqdn != nil {
		asoControlPlane.Status.ControlPlaneEndpoint = clusterv1.APIEndpoint{
			Host: ptr.Deref(managedCluster.Status.Fqdn, ""),
			Port: 443,
		}
	}
	if managedCluster.Status.ApiServerAccessProfile != nil &&
		ptr.Deref(managedCluster.Status.ApiServerAccessProfile.EnablePrivateCluster, false) &&
		!ptr.Deref(managedCluster.Status.ApiServerAccessProfile.EnablePrivateClusterPublicFQDN, false) {
		asoControlPlane.Status.ControlPlaneEndpoint = clusterv1.APIEndpoint{
			Host: ptr.Deref(managedCluster.Status.PrivateFQDN, ""),
			Port: 443,
		}
	}

	if managedCluster.Status.CurrentKubernetesVersion != nil {
		asoControlPlane.Status.Version = "v" + *managedCluster.Status.CurrentKubernetesVersion
	}

	err = r.reconcileKubeconfig(ctx, asoControlPlane, cluster, managedCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile kubeconfig: %w", err)
	}

	asoControlPlane.Status.Ready = true
	asoControlPlane.Status.Initialized = asoControlPlane.Status.Ready

	return ctrl.Result{}, nil
}

func (r *ASOManagedControlPlaneReconciler) reconcileKubeconfig(ctx context.Context, asoControlPlane *infrav1.ASOManagedControlPlane, cluster *clusterv1.Cluster, managedCluster *asocontainerservicev1.ManagedCluster) error {
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
				*metav1.NewControllerRef(asoControlPlane, infrav1.GroupVersion.WithKind("ASOManagedControlPlane")),
			},
			Labels: map[string]string{clusterv1.ClusterNameLabel: cluster.Name},
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: asoKubeconfig.Data[secretRef.Key],
		},
	}

	return r.Patch(ctx, expectedSecret, client.Apply, client.FieldOwner("capaso"), client.ForceOwnership)
}

func (r *ASOManagedControlPlaneReconciler) reconcileDelete(ctx context.Context, asoControlPlane *infrav1.ASOManagedControlPlane, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	if cluster == nil {
		// Cluster owner ref not set
		controllerutil.RemoveFinalizer(asoControlPlane, clusterv1.ClusterFinalizer)
		return ctrl.Result{}, nil
	}

	infraReconciler := &InfraReconciler{
		Client:          r.Client,
		resources:       asoControlPlane.Spec.Resources,
		owner:           asoControlPlane,
		externalTracker: r.externalTracker,
	}

	err := infraReconciler.Delete(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(asoControlPlane.GetResourceStatuses()) > 0 {
		// waiting for resources to be deleted
		return ctrl.Result{}, nil
	}

	controllerutil.RemoveFinalizer(asoControlPlane, clusterv1.ClusterFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ASOManagedControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.ASOManagedControlPlane{}).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.ClusterToKubeadmControlPlane),
			builder.WithPredicates(
				// TODO: watch for changes in pause to pause ASO resources.
				predicates.ClusterUnpausedAndInfrastructureReady(log.FromContext(ctx)),
			),
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
	return nil
}

// ClusterToASOManagedControlPlane is a handler.ToRequestsFunc to be used to enqueue requests for
// reconciliation for ASOManagedControlPlane based on updates to a Cluster.
func (r *ASOManagedControlPlaneReconciler) ClusterToKubeadmControlPlane(_ context.Context, o client.Object) []ctrl.Request {
	c, ok := o.(*clusterv1.Cluster)
	if !ok {
		panic(fmt.Sprintf("Expected a Cluster but got a %T", o))
	}

	controlPlaneRef := c.Spec.ControlPlaneRef
	if controlPlaneRef != nil && controlPlaneRef.Kind == "ASOManagedControlPlane" {
		return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: controlPlaneRef.Namespace, Name: controlPlaneRef.Name}}}
	}

	return nil
}

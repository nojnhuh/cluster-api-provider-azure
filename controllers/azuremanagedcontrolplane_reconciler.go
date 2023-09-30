/*
Copyright 2019 The Kubernetes Authors.

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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/azure/scope"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/groups"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/managedclusters"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/privateendpoints"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/resourcehealth"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/subnets"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/virtualnetworks"
	"sigs.k8s.io/cluster-api-provider-azure/util/tele"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// azureManagedControlPlaneService contains the services required by the cluster controller.
type azureManagedControlPlaneService struct {
	kubeclient client.Client
	scope      managedclusters.ManagedClusterScope
	services   []azure.ServiceReconciler
}

// newAzureManagedControlPlaneReconciler populates all the services based on input scope.
func newAzureManagedControlPlaneReconciler(scope *scope.ManagedControlPlaneScope) (*azureManagedControlPlaneService, error) {
	privateEndpointsSvc, err := privateendpoints.New(scope)
	if err != nil {
		return nil, err
	}
	resourceHealthSvc, err := resourcehealth.New(scope)
	if err != nil {
		return nil, err
	}
	subnetsSvc, err := subnets.New(scope)
	if err != nil {
		return nil, err
	}
	virtualNetworksSvc, err := virtualnetworks.New(scope)
	if err != nil {
		return nil, err
	}
	return &azureManagedControlPlaneService{
		kubeclient: scope.Client,
		scope:      scope,
		services: []azure.ServiceReconciler{
			groups.New(scope),
			virtualNetworksSvc,
			subnetsSvc,
			managedclusters.New(scope),
			privateEndpointsSvc,
			resourceHealthSvc,
		},
	}, nil
}

// Reconcile reconciles all the services in a predetermined order.
func (r *azureManagedControlPlaneService) Reconcile(ctx context.Context) error {
	ctx, _, done := tele.StartSpanWithLogger(ctx, "controllers.azureManagedControlPlaneService.Reconcile")
	defer done()

	for _, service := range r.services {
		if err := service.Reconcile(ctx); err != nil {
			return errors.Wrapf(err, "failed to reconcile AzureManagedControlPlane service %s", service.Name())
		}
	}

	if err := r.reconcileKubeconfig(ctx); err != nil {
		return errors.Wrap(err, "failed to reconcile kubeconfig secret")
	}

	return nil
}

// Pause pauses all components making up the cluster.
func (r *azureManagedControlPlaneService) Pause(ctx context.Context) error {
	ctx, _, done := tele.StartSpanWithLogger(ctx, "controllers.azureManagedControlPlaneService.Pause")
	defer done()

	for _, service := range r.services {
		pauser, ok := service.(azure.Pauser)
		if !ok {
			continue
		}
		if err := pauser.Pause(ctx); err != nil {
			return errors.Wrapf(err, "failed to pause AzureManagedControlPlane service %s", service.Name())
		}
	}

	return nil
}

// Delete reconciles all the services in a predetermined order.
func (r *azureManagedControlPlaneService) Delete(ctx context.Context) error {
	ctx, _, done := tele.StartSpanWithLogger(ctx, "controllers.azureManagedControlPlaneService.Delete")
	defer done()

	// Delete services in reverse order of creation.
	for i := len(r.services) - 1; i >= 0; i-- {
		if err := r.services[i].Delete(ctx); err != nil {
			return errors.Wrapf(err, "failed to delete AzureManagedControlPlane service %s", r.services[i].Name())
		}
	}

	return nil
}

func (r *azureManagedControlPlaneService) reconcileKubeconfig(ctx context.Context) error {
	ctx, _, done := tele.StartSpanWithLogger(ctx, "controllers.azureManagedControlPlaneService.reconcileKubeconfig")
	defer done()

	// ASO creates the kubeconfig secret automatically, but we still need to add the cluster name label for
	// CAPI to be able to find it.
	managedCluster, err := r.scope.ManagedClusterSpec().Parameters(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to get parameters for managed cluster")
	}
	kubeconfigKey := client.ObjectKey{
		Name:      managedCluster.Spec.OperatorSpec.Secrets.AdminCredentials.Name,
		Namespace: r.scope.ManagedClusterSpec().ResourceRef().Namespace,
	}
	kubeconfig := &corev1.Secret{}
	err = r.scope.GetClient().Get(ctx, kubeconfigKey, kubeconfig)
	if err != nil {
		return errors.Wrapf(err, "failed to get secret %s", kubeconfigKey)
	}
	labels := kubeconfig.GetLabels()
	if labels[clusterv1.ClusterNameLabel] == r.scope.ClusterName() {
		return nil
	}
	if labels == nil {
		labels = map[string]string{}
	}
	labels[clusterv1.ClusterNameLabel] = r.scope.ClusterName()
	kubeconfig.SetLabels(labels)
	err = r.scope.GetClient().Update(ctx, kubeconfig)
	return errors.Wrapf(err, "failed to update secret %s", &kubeconfig.ObjectMeta)
}

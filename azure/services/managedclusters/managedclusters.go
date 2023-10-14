/*
Copyright 2020 The Kubernetes Authors.

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

package managedclusters

import (
	"context"

	asocontainerservicev1 "github.com/Azure/azure-service-operator/v2/api/containerservice/v1api20230201"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/aso"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/token"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	serviceName        = "managedcluster"
	kubeletIdentityKey = "kubeletidentity"

	// The aadResourceID is the application-id used by the server side. The access token accessing AKS clusters need to be issued for this app.
	// Refer: https://azure.github.io/kubelogin/concepts/aks.html?highlight=6dae42f8-4368-4678-94ff-3960e28e3630#azure-kubernetes-service-aad-server
	aadResourceID = "6dae42f8-4368-4678-94ff-3960e28e3630"
)

// ManagedClusterScope defines the scope interface for a managed cluster.
type ManagedClusterScope interface {
	aso.Scope
	azure.Authorizer
	ManagedClusterSpec() azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedCluster]
	SetControlPlaneEndpoint(clusterv1.APIEndpoint)
	MakeEmptyKubeConfigSecret() corev1.Secret
	GetAdminKubeconfigData() []byte
	SetAdminKubeconfigData([]byte)
	GetUserKubeconfigData() []byte
	SetUserKubeconfigData([]byte)
	IsAADEnabled() bool
	AreLocalAccountsDisabled() bool
	SetOIDCIssuerProfileStatus(*infrav1.OIDCIssuerProfileStatus)
}

// Service provides operations on azure resources.
type Service struct {
	Scope ManagedClusterScope
	*aso.Service[*asocontainerservicev1.ManagedCluster, ManagedClusterScope]
}

// New creates a new service.
func New(scope ManagedClusterScope) *Service {
	svc := aso.NewService[*asocontainerservicev1.ManagedCluster](serviceName, scope)
	svc.Specs = []azure.ASOResourceSpecGetter[*asocontainerservicev1.ManagedCluster]{scope.ManagedClusterSpec()}
	svc.ConditionType = infrav1.ManagedClusterRunningCondition
	svc.PostCreateOrUpdateResourceHook = postCreateOrUpdateResourceHook
	return &Service{
		Scope:   scope,
		Service: svc,
	}
}

// Name returns the service name.
func (s *Service) Name() string {
	return serviceName
}

func postCreateOrUpdateResourceHook(scope ManagedClusterScope, managedCluster *asocontainerservicev1.ManagedCluster, err error) {
	if err != nil {
		return
	}

	// Update control plane endpoint.
	endpoint := clusterv1.APIEndpoint{
		Host: ptr.Deref(managedCluster.Status.Fqdn, ""),
		Port: 443,
	}
	scope.SetControlPlaneEndpoint(endpoint)

	// Update kubeconfig data
	// Always fetch credentials in case of rotation
	adminKubeConfigData, userKubeConfigData, err := reconcileKubeconfig(context.TODO(), scope, managedCluster.Namespace)
	if err != nil {
		// TODO:
		// return errors.Wrap(err, "error while reconciling adminKubeConfigData")
		return
	}
	scope.SetAdminKubeconfigData(adminKubeConfigData)
	scope.SetUserKubeconfigData(userKubeConfigData)

	scope.SetOIDCIssuerProfileStatus(nil)
	if managedCluster.Status.OidcIssuerProfile != nil && managedCluster.Status.OidcIssuerProfile.IssuerURL != nil {
		scope.SetOIDCIssuerProfileStatus(&infrav1.OIDCIssuerProfileStatus{
			IssuerURL: managedCluster.Status.OidcIssuerProfile.IssuerURL,
		})
	}
}

// reconcileKubeconfig will reconcile admin kubeconfig and user kubeconfig.
/*
  Returns the admin kubeconfig and user kubeconfig
  If aad is enabled a user kubeconfig will also get generated and stored in the secret <cluster-name>-kubeconfig-user
  If we disable local accounts for aad clusters we do not have access to admin kubeconfig, hence we need to create
  the admin kubeconfig by authenticating with the user credentials and retrieving the token for kubeconfig.
  The token is used to create the admin kubeconfig.
  The user needs to ensure to provide service principle with admin aad privileges.
*/
func reconcileKubeconfig(ctx context.Context, scope ManagedClusterScope, namespace string) (userKubeConfigData []byte, adminKubeConfigData []byte, err error) {
	if scope.IsAADEnabled() {
		if userKubeConfigData, err = getUserKubeconfigData(ctx, scope, namespace); err != nil {
			return nil, nil, errors.Wrap(err, "error while trying to get user kubeconfig")
		}
	}

	if scope.AreLocalAccountsDisabled() {
		userKubeconfigWithToken, err := getUserKubeConfigWithToken(userKubeConfigData, ctx, scope)
		if err != nil {
			return nil, nil, errors.Wrap(err, "error while trying to get user kubeconfig with token")
		}
		return userKubeconfigWithToken, userKubeConfigData, nil
	}

	asoSecret := &corev1.Secret{}
	err = scope.GetClient().Get(
		ctx,
		client.ObjectKey{
			Namespace: namespace,
			Name:      adminKubeconfigSecretName(scope.ClusterName()),
		},
		asoSecret,
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get ASO admin kubeconfig")
	}
	adminKubeConfigData = asoSecret.Data[secret.KubeconfigDataName]
	return adminKubeConfigData, userKubeConfigData, nil
}

// getUserKubeconfigData gets user kubeconfig when aad is enabled for the aad clusters.
func getUserKubeconfigData(ctx context.Context, scope ManagedClusterScope, namespace string) ([]byte, error) {
	asoSecret := &corev1.Secret{}
	err := scope.GetClient().Get(
		ctx,
		client.ObjectKey{
			Namespace: namespace,
			Name:      userKubeconfigSecretName(scope.ClusterName()),
		},
		asoSecret,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get ASO user kubeconfig")
	}
	kubeConfigData := asoSecret.Data[secret.KubeconfigDataName]
	return kubeConfigData, nil
}

// getUserKubeConfigWithToken returns the kubeconfig with user token, for capz to create the target cluster.
func getUserKubeConfigWithToken(userKubeConfigData []byte, ctx context.Context, scope azure.Authorizer) ([]byte, error) {
	tokenClient, err := token.NewClient(scope)
	if err != nil {
		return nil, errors.Wrap(err, "error while getting aad token client")
	}

	token, err := tokenClient.GetAzureActiveDirectoryToken(ctx, aadResourceID)
	if err != nil {
		return nil, errors.Wrap(err, "error while getting aad token for user kubeconfig")
	}

	return createUserKubeconfigWithToken(token, userKubeConfigData)
}

// createUserKubeconfigWithToken gets the kubeconfig data for authenticating with target cluster.
func createUserKubeconfigWithToken(token string, userKubeConfigData []byte) ([]byte, error) {
	config, err := clientcmd.Load(userKubeConfigData)
	if err != nil {
		return nil, errors.Wrap(err, "error while trying to unmarshal new user kubeconfig with token")
	}
	for _, auth := range config.AuthInfos {
		auth.Token = token
		auth.Exec = nil
	}
	kubeconfig, err := clientcmd.Write(*config)
	if err != nil {
		return nil, errors.Wrap(err, "error while trying to marshal new user kubeconfig with token")
	}
	return kubeconfig, nil
}

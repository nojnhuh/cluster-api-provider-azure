/*
Copyright 2024 The Kubernetes Authors.

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

package azure

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"k8s.io/utils/lru"
)

type credentialCache struct {
	cache       *lru.Cache
	credFactory credentialFactory
}

type credentialFactory interface {
	newClientSecretCredential(tenantID string, clientID string, clientSecret string, opts *azidentity.ClientSecretCredentialOptions) (azcore.TokenCredential, error)
	newClientCertificateCredential(tenantID string, clientID string, clientCertificate []byte, clientCertificatePassword []byte, opts *azidentity.ClientCertificateCredentialOptions) (azcore.TokenCredential, error)
	newManagedIdentityCredential(opts *azidentity.ManagedIdentityCredentialOptions) (azcore.TokenCredential, error)
	newWorkloadIdentityCredential(opts *azidentity.WorkloadIdentityCredentialOptions) (azcore.TokenCredential, error)
}

// CredentialType represents the auth mechanism in use.
type CredentialType int

const (
	// CredentialTypeClientSecret is for Service Principals with Client Secrets.
	CredentialTypeClientSecret CredentialType = iota
	// CredentialTypeClientCert is for Service Principals with Client certificates.
	CredentialTypeClientCert
	// CredentialTypeManagedIdentity is for Managed Identities.
	CredentialTypeManagedIdentity
	// CredentialTypeWorkloadIdentity is for Workload Identity.
	CredentialTypeWorkloadIdentity
)

type credentialCacheKey struct {
	authorityHost  string
	credentialType CredentialType
	tenantID       string
	clientID       string
	secret         string
}

// NewCredentialCache creates a new, empty LRU CredentialCache bounded by the given size. A size of zero
// results in a cache of unbounded size.
func NewCredentialCache(size int) CredentialCache {
	return &credentialCache{
		cache:       lru.New(size),
		credFactory: azureCredentialFactory{},
	}
}

func (c *credentialCache) GetOrStoreClientSecret(tenantID, clientID, clientSecret string, opts *azidentity.ClientSecretCredentialOptions) (azcore.TokenCredential, error) {
	return c.getOrStore(
		credentialCacheKey{
			authorityHost:  opts.Cloud.ActiveDirectoryAuthorityHost,
			credentialType: CredentialTypeClientSecret,
			tenantID:       tenantID,
			clientID:       clientID,
			secret:         clientSecret,
		},
		func() (azcore.TokenCredential, error) {
			return c.credFactory.newClientSecretCredential(tenantID, clientID, clientSecret, opts)
		},
	)
}

func (c *credentialCache) GetOrStoreClientCert(tenantID, clientID string, cert, certPassword []byte, opts *azidentity.ClientCertificateCredentialOptions) (azcore.TokenCredential, error) {
	return c.getOrStore(
		credentialCacheKey{
			authorityHost:  opts.Cloud.ActiveDirectoryAuthorityHost,
			credentialType: CredentialTypeClientCert,
			tenantID:       tenantID,
			clientID:       clientID,
			secret:         string(append(cert, certPassword...)),
		},
		func() (azcore.TokenCredential, error) {
			return c.credFactory.newClientCertificateCredential(tenantID, clientID, cert, certPassword, opts)
		},
	)
}

func (c *credentialCache) GetOrStoreManagedIdentity(opts *azidentity.ManagedIdentityCredentialOptions) (azcore.TokenCredential, error) {
	return c.getOrStore(
		credentialCacheKey{
			authorityHost:  opts.Cloud.ActiveDirectoryAuthorityHost,
			credentialType: CredentialTypeManagedIdentity,
			// tenantID not used for managed identity
			clientID: opts.ID.String(),
		},
		func() (azcore.TokenCredential, error) {
			return c.credFactory.newManagedIdentityCredential(opts)
		},
	)
}

func (c *credentialCache) GetOrStoreWorkloadIdentity(opts *azidentity.WorkloadIdentityCredentialOptions) (azcore.TokenCredential, error) {
	return c.getOrStore(
		credentialCacheKey{
			authorityHost:  opts.Cloud.ActiveDirectoryAuthorityHost,
			credentialType: CredentialTypeWorkloadIdentity,
			tenantID:       opts.TenantID,
			clientID:       opts.ClientID,
		},
		func() (azcore.TokenCredential, error) {
			return c.credFactory.newWorkloadIdentityCredential(opts)
		},
	)
}

func (c *credentialCache) getOrStore(key credentialCacheKey, newCredFunc func() (azcore.TokenCredential, error)) (azcore.TokenCredential, error) {
	if cred, exists := c.cache.Get(key); exists {
		return cred.(azcore.TokenCredential), nil
	}
	cred, err := newCredFunc()
	if err != nil {
		return nil, err
	}
	c.cache.Add(key, cred)
	return cred, nil
}

type azureCredentialFactory struct{}

func (azureCredentialFactory) newClientSecretCredential(tenantID string, clientID string, clientSecret string, opts *azidentity.ClientSecretCredentialOptions) (azcore.TokenCredential, error) {
	return azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, opts)
}

func (azureCredentialFactory) newClientCertificateCredential(tenantID string, clientID string, clientCertificate []byte, clientCertificatePassword []byte, opts *azidentity.ClientCertificateCredentialOptions) (azcore.TokenCredential, error) {
	certs, certKey, err := azidentity.ParseCertificates(clientCertificate, clientCertificatePassword)
	if err != nil {
		return nil, err
	}
	return azidentity.NewClientCertificateCredential(tenantID, clientID, certs, certKey, opts)
}

func (azureCredentialFactory) newManagedIdentityCredential(opts *azidentity.ManagedIdentityCredentialOptions) (azcore.TokenCredential, error) {
	return azidentity.NewManagedIdentityCredential(opts)
}

func (azureCredentialFactory) newWorkloadIdentityCredential(opts *azidentity.WorkloadIdentityCredentialOptions) (azcore.TokenCredential, error) {
	return azidentity.NewWorkloadIdentityCredential(opts)
}

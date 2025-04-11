# ASO API

This document describes how to define self-managed clusters using the ASO API. The `ASOSelfManaged` feature
gate must be enabled to use these APIs (`EXP_ASO_SELF_MANAGED=true clusterctl init -i azure ...`).

<aside class="note warning">

<h1> Warning </h1>

These APIs are for advanced users with expert proficiency in both Azure and Cluster API. They are difficult to
use by design for simple use cases and allow for countless ways to misconfigure clusters if you do not know
what you are doing. Users should prefer the existing AzureCluster, ASOMachine, and ASOMachinePool types to
orchestrate self-managed clusters.

</aside>

## Overview

The ASO API refers to CAPZ APIs with kinds named `AzureASO...` whose main feature is a list of embedded
Kubernetes resources defining [Azure Service Operator (ASO)](https://azure.github.io/azure-service-operator/)
objects. The APIs allow for nearly infinite flexibility in the infrastructure that can be created to satisfy
the most advanced use cases at the cost of YAML verbosity and ease of use.

An example resource might look something like the following template:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: AzureASOCluster
metadata:
  name: ${CLUSTER_NAME}
  namespace: default
spec:
  resources:
  - apiVersion: resources.azure.com/v1api20200601
    kind: ResourceGroup
    metadata:
      name: ${CLUSTER_NAME}
      annotations:
        serviceoperator.azure.com/credential-from: ${ASO_CREDENTIAL_SECRET_NAME}
    spec:
      location: ${AZURE_LOCATION}
```

Since ASO resource types are generated from Azure API specs, they expose the entire Azure API
surface area for a subset of available API versions. This allows users to fine-tune every parameter on each
Azure resource that makes up the cluster.

Furthermore, the ASO API does not prescribe any particular Azure resource topology. For example, AzureClusters
generally come with the same set of resources that all clusters are assumed to need, like a public IP address,
load balancer, virtual network, etc. The ASO API makes none of those assumptions, allowing users to select
exactly which infrastructure resources should comprise a particular CAPZ resource.

Because CAPZ makes no assumptions about which Azure resources to create for an ASO API resource, it puts the
burden squarely on the user to fully define each of those resources. A basic AzureCluster can be defined in
about 20 lines of YAML. A comparable AzureASOCluster requires at least 200 lines of YAML. These CAPZ APIs are
not for the faint of heart.

## Common Patterns

All ASO API resources share some common elements with equivalent usage.

### Authentication

ASO API resources _do not_ use [AzureClusterIdentity](/topics/identities.md) to configure Azure
authentication. Instead, [ASO-native credentials](https://azure.github.io/azure-service-operator/guide/authentication/) should be configured directly.

### Resources

The `spec.resources` field defines literal ASO objects inline whose lifecycles will be tied to the
enveloping CAPZ resource.

e.g.
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: AzureASOCluster
metadata:
  name: ${CLUSTER_NAME}
spec:
  resources:
  - apiVersion: resources.azure.com/v1api20200601
    kind: ResourceGroup
    metadata:
      name: ${CLUSTER_NAME}
    spec:
      location: ${AZURE_LOCATION}
  - apiVersion: network.azure.com/v1api20240301
    kind: VirtualNetwork
    metadata:
      name: ${CLUSTER_NAME}
    spec:
      owner:
        name: ${CLUSTER_NAME}
      location: ${AZURE_LOCATION}
      addressSpace:
        addressPrefixes:
        - 10.0.0.0/16
```

Any ASO resource is technically valid to put in any CAPZ ASO API type. Webhook validation for embedded
resources is _not_ performed by CAPZ's webhooks when an ASO API resource is created or updated, only when CAPZ
attempts to actually create or update those embedded resources. If the CAPZ controller manager attempts to
create an invalid object and fails, it will log an error message with the API server response.

In each reconciliation loop, CAPZ will perform a [server-side apply
patch](https://kubernetes.io/docs/reference/using-api/server-side-apply/) on each element of `spec.resources`
with the definition as it appears in the CAPZ object with any [patches](#patches) applied. Server-side apply protects modifications made by other
actors (notably the ASO control plane) to non-overlapping segments of the resources managed by CAPZ from being
overwritten by CAPZ.

The `status.resources` field describes the ASO resources for which the CAPZ object is responsible. It exposes
only enough information for CAPZ to map back to a resource in `spec.resources` and determine whether or not
the resource has been successfully provisioned. For more details about the status of an ASO resource, check
the ASO resource directly.

### Patches

The `spec.patches` field defines [RFC 6902](https://tools.ietf.org/html/rfc6902) JSON patches to apply
directly to the literal objects defined in `spec.resources`.

e.g.
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: AzureASOCluster
metadata:
  name: ${CLUSTER_NAME}
spec:
  patches:
  - jsonPatches: # add credentials to all resources
    - op: add
      path: /metadata/annotations
      value:
        serviceoperator.azure.com/credential-from: aso-credentials
  - selectors: # kinds requiring location
    - kind: ResourceGroup
    - kind: VirtualNetwork
    jsonPatches:
    - op: add
      path: /spec/location
      value: ${AZURE_LOCATION}
  resources:
  - apiVersion: resources.azure.com/v1api20200601
    kind: ResourceGroup
    metadata:
      name: ${CLUSTER_NAME}
    spec: {}
  - apiVersion: network.azure.com/v1api20240301
    kind: VirtualNetwork
    metadata:
      name: ${CLUSTER_NAME}
    spec:
      owner:
        name: ${CLUSTER_NAME}
      addressSpace:
        addressPrefixes:
        - 10.0.0.0/16
```

`jsonPatches` defines the literal RFC 6902 JSON patch to apply. `selectors` defines a list of selectors
matched against each element of `spec.resources`. The `jsonPatches` apply to a resource if _any_ of the
`selectors` match the resource, or if no `selectors` are defined for a patch.

The above example is equivalent to if the patches had been applied directly to `spec.resources`:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: AzureASOCluster
metadata:
  name: ${CLUSTER_NAME}
spec:
  resources:
  - apiVersion: resources.azure.com/v1api20200601
    kind: ResourceGroup
    metadata:
      name: ${CLUSTER_NAME}
      annotations:
        serviceoperator.azure.com/credential-from: aso-credentials
    spec:
      location: ${AZURE_LOCATION}
  - apiVersion: network.azure.com/v1api20240301
    kind: VirtualNetwork
    metadata:
      name: ${CLUSTER_NAME}
      annotations:
        serviceoperator.azure.com/credential-from: aso-credentials
    spec:
      location: ${AZURE_LOCATION}
      owner:
        name: ${CLUSTER_NAME}
      addressSpace:
        addressPrefixes:
        - 10.0.0.0/16
```

#### Value Templates

The `valueFrom` field is inspired by [ClusterClass
patches](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/write-clusterclass#advanced-features-of-clusterclass-with-patches)
and extends RFC 6902 JSON patches to supply non-literal values, such as from evaluating a Go `text/template`.
See the API documentation for more details about how to set this field.

In this partially-complete example, the Cluster's `spec.clusterNetwork.apiServerPort` is propagated to the
`frontendPort` of one of a LoadBalancer's load balancing rules. The LoadBalancer's name is also derived from
the AzureASOCluster's `metadata.name`:

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: ${CLUSTER_NAME}
spec:
  clusterNetwork:
    apiServerPort: 6443
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
    kind: AzureASOCluster
    name: ${CLUSTER_NAME}
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: AzureASOCluster
metadata:
  name: ${CLUSTER_NAME}
spec:
  patches:
  - selectors:
    - kind: LoadBalancer
    jsonPatches:
    - op: add
      path: /spec/loadBalancingRules/0/frontendPort
      valueFrom:
        template: |-
          {{ .cluster.spec.clusterNetwork.apiServerPort }}
    - op: add
      path: /metadata
      valueFrom:
        template: |-
          name: {{ .self.metadata.name }}-load-balancer
  resources:
  - apiVersion: network.azure.com/v1api20240301
    kind: LoadBalancer
    spec:
      loadBalancingRules:
      - name: controlplane
        frontendIPConfiguration:
          reference:
            armId: /subscriptions/${AZURE_SUBSCRIPTION_ID}/resourceGroups/${CLUSTER_NAME}/providers/Microsoft.Network/loadBalancers/${CLUSTER_NAME}/frontendIPConfigurations/controlplane
```

### Pause

When a CAPI Cluster object or ASO API resource is paused, CAPZ automatically adds the
[`serviceoperator.azure.com/reconcile-policy=skip`](https://azure.github.io/azure-service-operator/guide/annotations/#serviceoperatorazurecomreconcile-policy)
annotation to its owned ASO resources to prevent any further CAPI reconciliation of the Azure resources.

### Bring-Your-Own (BYO) Azure Resources

The ASO API resources allow users to refer to pre-existing Azure resources in `spec.resources`. See [ASO's
adoption docs](https://azure.github.io/azure-service-operator/guide/adoption/) for more details.

### Making more ASO types available to CAPZ

CAPZ exposes a setting at install time to configure [extra ASO CRDs](/topics/aso.md#installing-more-crds) which should be installed.

To allow CAPZ to manage objects of those types in `spec.resources`, the CAPZ controller manager will need to
be granted RBAC permissions. Permissions may be granted by using an [aggregated ClusterRole](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#aggregated-clusterroles).

For example, to enable referring to [Azure KeyVault ASO resources](https://azure.github.io/azure-service-operator/reference/keyvault/v1api20230701/#Vault) in CAPZ specs, first set `ADDITIONAL_ASO_CRDS` when installing CAPZ:
```
export ADDITIONAL_ASO_CRDS="keyvault.azure.com/Vault"
clusterctl init -i azure ...
```

Then create an aggregated ClusterRole with at least the following permissions:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: capz-aso-vault
  labels:
    cluster.x-k8s.io/aggregate-to-capz-manager: "true"
rules:
- apiGroups:
  - keyvault.azure.com
  resources:
  - vaults
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - keyvault.azure.com
  resources:
  - vaults/status
  verbs:
  - get
  - list
  - watch
```

### Troubleshooting

#### CAPZ does not create or update an ASO resource defined in `spec.resources`

This usually means that the inline ASO resource defined in `spec.resources` is invalid and CAPZ encounters an
error while trying to create or update the resource. Check the CAPZ controller manager logs for details about
the error.

## AzureASOCluster

AzureASOCluster defines resources for which there are a fixed number for the cluster. This generally includes
resources like ResourceGroups, VirtualNetworks, and LoadBalancers.

### Control Plane Endpoint

AzureASOCluster fulfills the [Cluster API InfraCluster
contract](https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-cluster). As such, unless an
externally hosted control plane is used, it is expected to produce a `spec.controlPlaneEndpoint`. The
AzureASOCluster API makes no assumptions about which resources make up which parts of the control plane
endpoint, or even if any do at all. Therefore, it is up to the user either to define
`spec.controlPlaneEndpoint` themselves, or specify a `spec.controlPlaneEndpointSource` which describes where
the endpoint's constituent parts can be found.

One way to define a `spec.controlPlaneEndpointSource` is to reference specific fields in ConfigMaps holding
the control plane endpoint's `host` and `port`. Together with [ASO's native ability to produce
ConfigMaps](https://azure.github.io/azure-service-operator/guide/configmaps/#how-to-export-configmap-data-from-aso)
with details about the provisioned resources, an AzureASOCluster can declare specific parts of specific
resources as the control plane endpoint.

The following partial example shows how to populate a `spec.controlPlaneEndpoint` based on a PublicIPAddress
and LoadBalancer:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: AzureASOCluster
metadata:
  name: ${CLUSTER_NAME}
spec:
  controlPlaneEndpointSource:
    host:
      configMap:
        name:
          value: ${CLUSTER_NAME}-controlplane-ip
        key:
          value: host
    port:
      configMap:
        name:
          value: ${CLUSTER_NAME}-controlplane-lb
        key:
          value: port
  resources:
  - apiVersion: network.azure.com/v1api20240301
    kind: PublicIPAddress
    metadata:
      name: ${CLUSTER_NAME}-controlplane
    spec:
      operatorSpec:
        configMapExpressions:
        - name: ${CLUSTER_NAME}-controlplane-ip
          key: host
          value: self.status.ipAddress
  - apiVersion: network.azure.com/v1api20240301
    kind: LoadBalancer
    metadata:
      name: ${CLUSTER_NAME}
    spec:
      frontendIPConfigurations:
      - name: controlplane
        publicIPAddress:
          reference:
            group: network.azure.com
            kind: PublicIPAddress
            name: ${CLUSTER_NAME}-controlplane
      loadBalancingRules:
      - name: controlplane
        frontendPort: 6443
        frontendIPConfiguration:
          reference:
            armId: /subscriptions/${AZURE_SUBSCRIPTION_ID}/resourceGroups/${CLUSTER_NAME}/providers/Microsoft.Network/loadBalancers/${CLUSTER_NAME}/frontendIPConfigurations/controlplane
      operatorSpec:
        configMapExpressions:
        - name: ${CLUSTER_NAME}-controlplane-lb
          key: port
          value: string(self.spec.loadBalancingRules[0].frontendPort)
```

## AzureASOMachine

AzureASOMachine defines resources for which there are a fixed number per Machine. This generally includes
resources like VirtualMachines, NetworkInterfaces, and Disks.

# Cluster API Provider Azure Service Operator (CAPASO)

Cluster API Provider Azure Service Operator (CAPASO) aims to demonstrate an alternative approach to
implementing a Cluster API infrastructure provider motivated by the following observations from CAPZ:

- CAPZ's custom resource types can only represent a fraction of what can be configured with Azure's REST APIs
- Discovering, implementing, and validating the gaps in CAPZ's feature set is a continual source of
  development overhead, especially for CAPZ's managed cluster stack
- At the same time, many users appreciate having a narrow set of discrete features which makes it easy to get
  up and running
- Users looking to adopt CAPZ to manage existing clusters are often left hanging because each resource
  represents a narrow set of topologies and can't express the set of the user's current resources
  (AzureManagedControlPlane = resource group + vnet + subnet + managed cluster)

The main goals of CAPASO are:

- To fulfill advanced use cases by defining a low-level interface allowing the utmost flexibility for users to
  customize the topology and other details of infrastructure components making up a Cluster
- To fulfill basic use cases by exposing a higher-level interface building on top of CAPASO's low-level
  interface.

Here is an example of CAPASO's ASOManagedCluster resource:
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ASOManagedCluster
metadata:
  name: capaso
spec:
  resources:
  - apiVersion: resources.azure.com/v1api20200601
    kind: ResourceGroup
    metadata:
      name: capaso
    spec:
      location: eastus
```

The main element of CAPASO's API types is a `spec.resources` field which defines arbitrary Kubernetes
resources to be reconciled. CAPASO strives to make as few assumptions as possible about the details of these
resources, simply applying these resources to the management cluster in its reconciliation loop and reflecting
their status.

CAPASO's main assumption is that each object in `spec.resources` is an Azure Service Operator (ASO)
resource, which allows CAPASO to understand the state of the underlying Azure resources. It also must make
some assumptions like that an ASOManagedControlPlane defines an ASO ManagedCluster resource. These assumptions
are necessary to fulfill CAPASO's contract with Cluster API.

This interface requires much more configuration than existing providers to represent the same underlying
infrastructure, but allows users the flexibility to construct their clusters in virtually any of infinite
possible configurations. To provide a simpler way for users to quickly deploy a "blessed" configuration,
CAPASO provides a Helm chart defining a workload cluster, including the CAPI and CAPASO components and
supporting resources like credentials. Creating a cluster based on the Helm chart may look something like:

```sh
% helm install my-workload ./charts/capaso-workload -f my-creds.yaml --set clusterName=my-cluster --set machinePools.pool0.replicas=3
```

CAPASO's workload cluster Helm chart aims to satisfy most use cases while serving as a reference upon which
advanced users can build more tailored configuration.

## Getting Started

### Prerequisites
- go version v1.21.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/capaso:tag
```

**NOTE:** This image ought to be published in the personal registry you specified. 
And it is required to have access to pull the image from the working environment. 
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/capaso:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin 
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/capaso:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/capaso/<tag or branch>/dist/install.yaml
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

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


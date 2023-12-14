---
title: Automate AKS Features Available in CAPZ
authors:
  - "@nojnhuh"
reviewers:
  - "@CecileRobertMichon"
  - "@matthchr"
  - "@dtzar"
creation-date: 2023-11-22
last-updated: 2023-11-28
status: provisional
see-also:
  - "docs/proposals/20230123-azure-service-operator.md"
---

# Automate AKS Features Available in CAPZ

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Glossary](#glossary)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals/Future Work](#non-goalsfuture-work)
- [Proposal](#proposal)
  - [User Stories](#user-stories)
    - [Story 1](#story-1)
    - [Story 2](#story-2)
    - [Story 3](#story-3)
  - [API Design Options](#api-design-options)
    - [Option 1: CAPZ resource references an existing ASO resource](#option-1-capz-resource-references-an-existing-aso-resource)
    - [Option 2: CAPZ resource references a non-functional ASO "template" resource](#option-2-capz-resource-references-a-non-functional-aso-template-resource)
    - [Option 3: CAPZ resource defines an entire unstructured ASO resource inline](#option-3-capz-resource-defines-an-entire-unstructured-aso-resource-inline)
    - [Option 4: CAPZ resource defines an entire typed ASO resource inline](#option-4-capz-resource-defines-an-entire-typed-aso-resource-inline)
    - [Decision](#decision)
  - [Security Model](#security-model)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Alternatives](#alternatives)
- [Upgrade Strategy](#upgrade-strategy)
- [Additional Details](#additional-details)
  - [Test Plan](#test-plan)
  - [Graduation Criteria](#graduation-criteria)
  - [Version Skew Strategy](#version-skew-strategy)
- [Implementation History](#implementation-history)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Glossary

## Summary

CAPZ's AzureManagedControlPlane and AzureManagedMachinePool resources expose AKS's managed cluster and agent
pool resources for Cluster API. Currently, new features in AKS require manual changes to teach CAPZ about
those features for users to be able to take advantage of them natively in Cluster API. As a result, there are
several AKS features available that cannot be used from Cluster API. This proposal describes how CAPZ will
automatically make all AKS features available on an ongoing basis with minimal maintenance.

## Motivation

Historically, CAPZ has exposed an opinionated subset of AKS features that are tested and known to work within
the Cluster API ecosystem. Since then, it has become increasingly clear that new AKS features are generally
suitable to implement in CAPZ and users are interested in having all AKS features available to them from
Cluster API.

When gaps exist in the set of features available in AKS and what CAPZ offers, users of other infrastructure
management solutions may not be able to adopt CAPZ. If all AKS features could be used from CAPZ, this would
not be an issue.

The AKS feature set changes rapidly alongside CAPZ's users' desire to utilize those features. Because making
new AKS features available in CAPZ requires a considerable amount of mechanical, manual effort to implement,
review, and test, requests for new AKS features account for a large portion of the cost to maintain CAPZ.

Another long-standing feature request in CAPZ is the ability to adopt existing AKS clusters into management by
Cluster API (https://github.com/kubernetes-sigs/cluster-api-provider-azure/issues/1173). Making the entire AKS
API surface area available from CAPZ is required to enable this so existing clusters' full representations can
be reflected in Cluster API.

### Goals

- Narrow the gap between the sets of features offered by AKS and CAPZ.
- Reduce the maintenance cost of making new AKS features available from CAPZ.
- Preserve the behavior of existing CAPZ AKS definitions while allowing users to utilize the new API pattern
  iteratively to use new features not currently implemented in CAPZ.

### Non-Goals/Future Work

- Automate the features available from other Azure services besides AKS.

## Proposal

### User Stories

#### Story 1

As a managed cluster user, I want to be able to use all available AKS features natively from CAPZ so that I
can more consistently manage my CAPZ AKS clusters that use advanced or niche features more quickly than having
to wait for each of them to be implemented in CAPZ.

#### Story 2

As an AKS user looking to adopt Cluster API over an existing infrastructure management solution, I want to be
able to use all AKS features natively from CAPZ so that I can adopt Cluster API with the confidence that all
the AKS features I currently utilize are still supported.

#### Story 3

As a CAPZ developer, I want to be able to make new AKS features available from CAPZ more easily in order to
meet user demand.

### API Design Options

There are a few different ways the entire AKS API surface area could be exposed from the CAPZ API. The
following options all rely on ASO's ManagedCluster and ManagedClustersAgentPool resources to define the full
AKS API, roughly organized in order of increasing responsibility for CAPZ. The examples below use
AzureManagedControlPlane and ManagedCluster to help illustrate, but all of the same ideas should also apply to
AzureManagedMachinePool and ManagedClustersAgentPool.

#### Option 1: CAPZ resource references an existing ASO resource

Here, the AzureManagedControlPlane spec would be updated with a field that references an ASO ManagedCluster
resource:

```go
type AzureManagedControlPlaneSpec struct {
	...
	
	// ManagedClusterName is the name of the ASO ManagedCluster backing this AzureManagedControlPlane.
	ManagedClusterName string `json:"managedClusterName"`
}
```

CAPZ will _not_ create this ManagedCluster and instead rely on it being created by any other means. CAPZ's
`aks` flavor template will be updated to include a ManagedCluster to fulfill this requirement. CAPZ will also
not modify the ManagedCluster except to fulfill CAPI contracts, such as managing replica count. Users modify
other parameters which are not managed by CAPI on the ManagedCluster directly. Users should be fairly familiar
with this pattern since it is already used extensively throughout CAPI, e.g. by modifying a virtual machine
through its InfraMachine resource for parameters not defined on the Machine resource.

This approach has two key benefits. First, it can leverage ASO's conversion webhooks to allow CAPZ to interact
with the ManagedCluster through one version of the API, and users to use a different API version, including
newer or preview (https://github.com/kubernetes-sigs/cluster-api-provider-azure/issues/2625) API versions.
Second, since ASO can [adopt existing Azure
resources](https://azure.github.io/azure-service-operator/guide/frequently-asked-questions/#how-can-i-import-existing-azure-resources-into-aso),
adopting existing AKS clusters into CAPZ
(https://github.com/kubernetes-sigs/cluster-api-provider-azure/issues/1173) additionally would only require
the extra steps to create the AzureManagedControlPlane referring to the adopted ManagedCluster.

One other consideration is that this approach trades the requirement that CAPZ has the necessary RBAC
permissions to create ManagedClusters for users having that same capability. CAPZ would still require
permissions to read, update, and delete ManagedClusters.

Drawbacks with this approach include:
- A requirement to define ASO resources in templates in addition to the existing suite of CAPI/CAPZ resources
  (one ASO ManagedCluster and one ASO ManagedClustersAgentPool for each MahcinePool).
- Inconsistency between how CAPZ manages some resources through references to pre-created ASO objects (the
  managed cluster and agent pools) and some by creating the ASO objects on its own (resource group, virtual
  network, subnets).
- An increased risk of users conflicting with CAPZ if users and CAPZ are both expected to modify mutually
  exclusive sets of fields on the same resources.

Other main roadblocks with this method relate to ClusterClass. If CAPZ's AzureManagedControlPlane controller is
not responsible for creating the ASO ManagedCluster resource, then users would need to manage those
separately, defeating much of the purpose of ClusterClass. Additionally, since each AzureManagedControlPlane
will be referring to a distinct ManagedCluster, the new `ManagedClusterName` field should not be defined in an
AzureManagedControlPlaneTemplate.

#### Option 2: CAPZ resource references a non-functional ASO "template" resource

This method is similar to [Option 1]. To better enable ClusterClass, instead of defining the full name of the
ManagedCluster resource, the name of a "template" resource is defined instead:

```go
type AzureManagedControlPlaneClassSpec struct {
	...
	
	// ManagedClusterTemplateName is the name of the ASO ManagedCluster to be used as a template from which
	// new ManagedClusters will be created.
	ManagedClusterTemplateName string `json:"managedClusterTemplateName"`
}
```

This template resource will be a ManagedCluster used as a base from which the AzureManagedControlPlane
controller will create a new ManagedCluster. The template ManagedCluster will have ASO's [`skip` reconcile
policy](https://azure.github.io/azure-service-operator/guide/annotations/#serviceoperatorazurecomreconcile-policy)
applied so it does not result in any AKS resource being created in Azure. The ManagedClusters created based on
the template will be reconciled normally to create AKS resources in Azure. The non-template ManagedClusters
will be linked to the AzureManagedControlPlane through the standard `cluster.x-k8s.io/cluster-name` label.

To modify parameters on the AKS cluster, either the template or non-template ManagedCluster may be updated.
CAPZ will propagate changes made to a template to instances of that template. Parameters defined on the
template take precedence over the same parameters on the instances.

The main difference with [Option 1] that enables ClusterClass is that the same ManagedCluster template
resource can be referenced by multiple AzureManagedControlPlanes, so this new `ManagedClusterTemplateName`
field can be defined on the AzureManagedControlPlaneClassSpec so a set of AKS parameters defined once in the
template can be applied to all Clusters built from that ClusterClass.

This method makes all ManagedCluster fields available to define in a template which could lead to
misconfiguration if certain parameters that must be unique to a cluster are erroneously shared through a
template. Since those fields cannot be automatically identified and may evolve between AKS API versions, CAPZ
will not attempt to categorize ASO ManagedCluster fields that way like it does between the fields present and
omitted from the `AzureClusterClassSpec` type, for example. CAPZ could document a best-effort list of known
fields which could or should not be defined in template types and will otherwise rely on AKS to provide
reasonable error messages for misconfigurations.

Like [Option 1], this method keeps particular versions of CAPZ decoupled from particular API versions of ASO
resources (including allowing preview versions) and opens the door for streamlined adoption of existing AKS
clusters.

#### Option 3: CAPZ resource defines an entire unstructured ASO resource inline

This method is functionally equivalent to [Option 2] except that the template resource is defined inline
within the AzureManagedControlPlane:

```go
type AzureManagedControlPlaneClassSpec struct {
	...
	
	// ManagedClusterTemplate is the ASO ManagedCluster to be used as a template from which new
	// ManagedClusters will be created.
	ManagedClusterTemplate map[string]interface{} `json:"managedClusterTemplate"`
}
```

One variant of this method could be to using `string` instead of `map[string]interface{}` for the template
type, though that would make defining patches unwieldy (like for ClusterClass).

Compared to [Option 2], this method loses schema and webhook validation that would be performed by ASO when
creating a separate ManagedCluster to serve as a template. That validation would still be performed when CAPZ
creates the ManagedCluster resource, but that would be some time after the AzureManagedControlPlane is created
and error messages may not be quite as visible.

#### Option 4: CAPZ resource defines an entire typed ASO resource inline

This method is functionally equivalent to [Option 3] except that the template field's type is the exact same
as an ASO ManagedCluster:

```go
type AzureManagedControlPlaneClassSpec struct {
	...
	
	// ManagedClusterSpec defines the spec of the ASO ManagedCluster managed by this AzureManagedControlPlane.
	ManagedClusterSpec v1api20230201.ManagedCluster_Spec `json:"managedClusterTemplate"`
}
```

This method allows CAPZ to leverage schema validation defined by ASO's CRDs upon AzureManagedControlPlane
creation, but would still lose any further webhook validation done by ASO unless CAPZ can invoke that itself.

It also has the drawback that one version of CAPZ is tied directly to a single AKS API version. The spec could
potentially contain separate fields for each API version and enforce in webhooks that only one API version is
being used at a time. Alternatively, users may set fields only present in a newer API version directly on the
ManagedCluster after creation (if allowed by AKS) because CAPZ will not override user-provided fields for
which it does not have its own opinion on ASO resources.

While it couples CAPZ to one ASO API version, this approach allows CAPZ to move a more calculated pace with
regards to AKS API versions the way it's done today. This also narrows CAPZ's scope of responsibility which
reduces CAPZ's exposure to potential incompatibilities with certain ASO API versions.

Regarding ClusterClass, this option functions the same as [Option 2] or [Option 3], where all ASO fields can
be defined in a template. This option opens up an additional safeguard though, where webhooks could flag
fields which should not be defined in a template. Similar webhook checks would be less practical in the other
options where CAPZ would need to be aware of the set of disallowed fields for each ASO API version that a user
could use.

#### Decision

We are opting to move forward with [Option 4]. That approach is most consistent with how the API is defined
today and would make for the smoothest transition for users.

### Security Model

Overall, none of the approaches outlined above change what data is ultimately represented in the API, only the
higher-level shape of the API. That means there is no further transport or handling of secrets or other
sensitive information beyond what CAPZ already does.

### Risks and Mitigations

Increasing CAPZ's reliance on ASO and exposing ASO to users at the API level further increases the risk
[previously discussed](https://github.com/kubernetes-sigs/cluster-api-provider-azure/blob/ce3c130266b23a8b67aa5ef9a21f257ff9e6d63e/docs/proposals/20230123-azure-service-operator.md?plain=1#L169)
that since ASO has not yet been proven to be as much of a staple as the other projects that manage
infrastructure on Azure, ASO's lifespan may be more limited than others. If ASO were to sunset while CAPZ
still relies on it, CAPZ would have to rework its APIs. This risk is mitigated by the fact that no
announcements have yet been made regarding ASO's end-of-life and the project continues to be very active.

## Alternatives

The main alternative to leveraging ASO to expose the entire AKS API from CAPZ would be for CAPZ to build its
own code generation pipeline which takes as input the Azure API specs and ultimately produces definitions for
AzureManagedControlPlane and AzureManagedMachinePool and mappings to ASO's ManagedCluster and
ManagedClustersAgentPool. Such a solution would likely end up being too similar to what ASO already provides
for the effort to be worthwhile.

## Upgrade Strategy

Each of the [options above](#api-design-options) suggest additive, backwards-compatible API changes. Users
will not need to perform any action after upgrading to the first version of CAPZ that implements this
proposal.

The new API field allowing the entire ASO resource to be defined in the CAPZ API will overlap with several
existing CAPZ API fields. e.g. with [Option 4], AzureManagedControlPlane's `spec.location` maps to the same
ASO API field as `spec.managedClusterSpec.location`. If both are defined, then the `spec.managedCluster.*`
field takes precedence. If only `spec.location` is defined, its value will still be used to construct the
desired state for the ManagedCluster.

An alternative available with [Option 3] and [Option 4] is to introduce a new CAPZ API version for
AzureManagedControlPlane and AzureManagedMachinePool, such as v1beta2, which includes only the ASO type and
other non-overlapping fields. Then, conversion webhooks can be implemented to convert between v1beta1 and
v1beta2.

## Additional Details

### Test Plan

Existing end-to-end tests will verify that CAPZ's current behavior does not regress. New tests will verify
that the new API fields proposed here behave as expected.

### Graduation Criteria

In CAPZ version `1.N` which first implements this proposal, setting any new CAPZ API fields introduced by this
proposal will not be allowed without enabling an `AKSASOAPI` feature flag in CAPZ. The feature flag will be
required throughout `1.N+2` and dropped for `1.N+3` where the feature will be enabled by default.

### Version Skew Strategy

With options 1-3 above, users are free to use any ASO API versions for AKS resources. CAPZ may internally
operate against a different API version and ASO's webhooks will transparently perform any necessary conversion
between versions.

## Implementation History

- [ ] 06/15/2023: Issue opened: https://github.com/kubernetes-sigs/cluster-api-provider-azure/issues/3629
- [ ] 11/22/2023: Iteration begins on this proposal document
- [ ] 11/28/2023: First complete draft of this document is made available for review

[Option 1]: #option-1-capz-resource-references-an-existing-aso-resource
[Option 2]: #option-2-capz-resource-references-a-non-functional-aso-template-resource
[Option 3]: #option-3-capz-resource-defines-an-entire-unstructured-aso-resource-inline
[Option 4]: #option-4-capz-resource-defines-an-entire-typed-aso-resource-inline

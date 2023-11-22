---
title: Automate AKS Features Available in CAPZ
authors:
  - "@nojnhuh"
reviewers:
  - "@CecileRobertMichon"
  - "@matthchr"
  - "@dtzar"
creation-date: 2023-11-22
last-updated: 2023-11-22
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
  - [Requirements (Optional)](#requirements-optional)
    - [Functional Requirements](#functional-requirements)
      - [FR1](#fr1)
      - [FR2](#fr2)
    - [Non-Functional Requirements](#non-functional-requirements)
      - [NFR1](#nfr1)
      - [NFR2](#nfr2)
  - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
  - [Security Model](#security-model)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Alternatives](#alternatives)
- [Upgrade Strategy](#upgrade-strategy)
- [Additional Details](#additional-details)
  - [Test Plan [optional]](#test-plan-optional)
  - [Graduation Criteria [optional]](#graduation-criteria-optional)
  - [Version Skew Strategy [optional]](#version-skew-strategy-optional)
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

### Non-Goals/Future Work

- Automate the features available from other Azure services besides AKS.

## Proposal

This is where we get down to the nitty gritty of what the proposal actually is.

- What is the plan for implementing this feature?
- What data model changes, additions, or removals are required?
- Provide a scenario, or example.
- Use diagrams to communicate concepts, flows of execution, and states.

[PlantUML](http://plantuml.com) is the preferred tool to generate diagrams,
place your `.plantuml` files under `images/` and run `make diagrams` from the docs folder.

### User Stories

- Detail the things that people will be able to do if this proposal is implemented.
- Include as much detail as possible so that people can understand the "how" of the system.
- The goal here is to make this feel real for users without getting bogged down.

#### Story 1

#### Story 2

### Requirements (Optional)

Some authors may wish to use requirements in addition to user stories.
Technical requirements should derived from user stories, and provide a trace from
use case to design, implementation and test case. Requirements can be prioritised
using the MoSCoW (MUST, SHOULD, COULD, WON'T) criteria.

The FR and NFR notation is intended to be used as cross-references across a CAEP.

The difference between goals and requirements is that between an executive summary
and the body of a document. Each requirement should be in support of a goal,
but narrowly scoped in a way that is verifiable or ideally - testable.

#### Functional Requirements

Functional requirements are the properties that this design should include.

##### FR1

##### FR2

#### Non-Functional Requirements

Non-functional requirements are user expectations of the solution. Include
considerations for performance, reliability and security.

##### NFR1

##### NFR2

### Implementation Details/Notes/Constraints

- What are some important details that didn't come across above.
- What are the caveats to the implementation?
- Go in to as much detail as necessary here.
- Talk about core concepts and how they releate.

### Security Model

Document the intended security model for the proposal, including implications
on the Kubernetes RBAC model. Questions you may want to answer include:

* Does this proposal implement security controls or require the need to do so?
  * If so, consider describing the different roles and permissions with tables.
* Are their adequate security warnings where appropriate (see https://adam.shostack.org/ReederEtAl_NEATatMicrosoft.pdf for guidance).
* Are regex expressions going to be used, and are their appropriate defenses against DOS.
* Is any sensitive data being stored in a secret, and only exists for as long as necessary?

### Risks and Mitigations

- What are the risks of this proposal and how do we mitigate? Think broadly.
- How will UX be reviewed and by whom?
- How will security be reviewed and by whom?
- Consider including folks that also work outside the SIG or subproject.

## Alternatives

The `Alternatives` section is used to highlight and record other possible approaches to delivering the value proposed by a proposal.

## Upgrade Strategy

If applicable, how will the component be upgraded? Make sure this is in the test plan.

Consider the following in developing an upgrade strategy for this enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing cluster required to make on upgrade in order to make use of the enhancement?

## Additional Details

### Test Plan [optional]

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy.
Anything that would count as tricky in the implementation and anything particularly challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage expectations).
Please adhere to the [Kubernetes testing guidelines][testing-guidelines] when drafting this test plan.

[testing-guidelines]: https://git.k8s.io/community/contributors/devel/sig-testing/testing.md

### Graduation Criteria [optional]

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal should keep
this high-level with a focus on what signals will be looked at to determine graduation.

Consider the following in developing the graduation criteria for this enhancement:
- [Maturity levels (`alpha`, `beta`, `stable`)][maturity-levels]
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

### Version Skew Strategy [optional]

If applicable, how will the component handle version skew with other components? What are the guarantees? Make sure
this is in the test plan.

Consider the following in developing a version skew strategy for this enhancement:
- Does this enhancement involve coordinating behavior in the control plane and in the kubelet? How does an n-2 kubelet without this feature available behave when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI or CNI may require updating that component before the kubelet.

## Implementation History

- [ ] 06/15/2023: Issue opened: https://github.com/kubernetes-sigs/cluster-api-provider-azure/issues/3629
- [ ] 11/22/2023: Iteration begins on this proposal document.

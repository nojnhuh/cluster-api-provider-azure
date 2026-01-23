# Component-Based Kustomize Templates

This directory contains a restructured approach to building CAPZ cluster templates using Kustomize.
The goal is to reduce duplication and make template composition more maintainable.

**Copilot Session ID:** `f67abf8b-0100-4079-b222-e2c121c04771`

## Structure

```
components/
├── cluster/
│   ├── base/           # Cluster object without infrastructure/controlPlane refs
│   ├── core/           # Self-managed: AzureCluster + KubeadmControlPlane
│   └── aks/            # AKS-managed: AzureManagedCluster + AzureManagedControlPlane
├── machines/
│   ├── linux-md/       # Linux MachineDeployment for self-managed clusters
│   ├── windows-md/     # Windows MachineDeployment for self-managed clusters
│   └── aks-pool/       # AKS MachinePool + AzureManagedMachinePool
└── identity/
    └── cluster-identity/   # AzureClusterIdentity with patches for both cluster types
```

## Key Concepts

### 1. Components are Atomic Building Blocks

Each component is self-contained with its own `kustomization.yaml` and can be built independently.
Components have a single responsibility (e.g., "provide linux worker nodes" or "provide AKS control plane").

### 2. Patches as Component Outputs

Components can export patch files that consuming kustomizations apply. This allows a component
to modify resources from other components without tight coupling.

Examples:
- `cluster/core/cluster-refs.yaml` - Patches the Cluster to reference AzureCluster + KubeadmControlPlane
- `cluster/aks/cluster-refs.yaml` - Patches the Cluster to reference AzureManagedCluster + AzureManagedControlPlane
- `identity/cluster-identity/azurecluster-identity-ref.yaml` - Patches AzureCluster with identity ref
- `identity/cluster-identity/managedcontrolplane-identity-ref.yaml` - Patches AzureManagedControlPlane with identity ref

### 3. Flavors Compose Components

Flavors in `flavors-v2/` compose components into complete cluster definitions:

```yaml
# Example: flavors-v2/default/kustomization.yaml
resources:
- ../../components/cluster/core
- ../../components/machines/linux-md
- ../../components/identity/cluster-identity

patches:
- path: ../../components/identity/cluster-identity/azurecluster-identity-ref.yaml
```

### 4. Cluster Base Factoring

The `cluster/base` component contains just the Cluster object without `infrastructureRef` or
`controlPlaneRef`. Both `cluster/core` (self-managed) and `cluster/aks` (managed) extend it
and apply their own refs via patches. This avoids duplicating the Cluster resource.

## Usage

Build a flavor with kustomize:

```bash
kustomize build --load-restrictor=LoadRestrictionsNone templates/flavors-v2/default
```

## Adding New Components

1. Create a directory under the appropriate category (`cluster/`, `machines/`, etc.)
2. Add resource YAML files
3. Create a `kustomization.yaml` listing resources
4. If the component needs to modify resources from other components, export a patch file
5. Document which patch(es) consumers should apply

## Adding New Flavors

1. Create a directory under `flavors-v2/`
2. Create a `kustomization.yaml` that:
   - Lists component resources to include
   - Applies any required patches from those components
3. Test with `kustomize build --load-restrictor=LoadRestrictionsNone`

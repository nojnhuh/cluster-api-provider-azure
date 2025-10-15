# E2E Test Label Migration

This document describes the migration from custom `[Label]` format in test names to Ginkgo's native label filtering system.

## Changes Made

### Test Labels

The following custom labels in test names have been replaced with Ginkgo's `Label()` decorator:

- `[REQUIRED]` → `Label("required")` - Essential tests that must pass
- `[OPTIONAL]` → `Label("optional")` - Optional tests that may be skipped
- `[K8s-Upgrade]` → `Label("k8s-upgrade")` - Kubernetes upgrade tests
- `[Managed Kubernetes]` → `Label("aks")` - AKS/managed cluster tests
- `[API-Server-ILB]` → `Label("api-server-ilb")` - API Server ILB feature tests
- Windows tests → `Label("windows")` - Tests that include Windows worker nodes

### Test Files Updated

- `test/e2e/azure_test.go` - Updated all Context() declarations to use Label()
- `test/e2e/capi_test.go` - Updated Context() declarations for K8s upgrade tests

### Configuration Changes

#### Makefile
- Changed default from `GINKGO_FOCUS ?= \[REQUIRED\]` to `GINKGO_LABEL_FILTER ?= required`
- Added `GINKGO_LABEL_FILTER` as the primary filtering mechanism
- Kept `GINKGO_FOCUS` and `GINKGO_SKIP` for backward compatibility

#### e2e.mk
- Updated ginkgo command to include `--label-filter="$(GINKGO_LABEL_FILTER)"`
- Label filter is applied before focus/skip filters

### Documentation Updated

- `docs/book/src/developers/jobs.md` - Updated CI job examples to use label filters
- `docs/book/src/developers/development.md` - Updated E2E testing documentation
- `docs/book/src/developers/tilt-with-aks-as-mgmt-ilb.md` - Updated testing guidance

## Usage Examples

### Running Tests by Label

```bash
# Run only required tests (default)
make test-e2e

# Run all tests (required + optional)
GINKGO_LABEL_FILTER="" make test-e2e

# Run only optional tests
GINKGO_LABEL_FILTER="optional" make test-e2e

# Run only AKS tests
GINKGO_LABEL_FILTER="aks" make test-e2e

# Run only Windows tests
GINKGO_LABEL_FILTER="windows" make test-e2e

# Run only K8s upgrade tests
GINKGO_LABEL_FILTER="k8s-upgrade" make test-e2e
```

### Combining Labels

Ginkgo's label filter supports boolean expressions:

```bash
# Run required tests excluding Windows
GINKGO_LABEL_FILTER="required && !windows" make test-e2e

# Run required tests excluding Windows and AKS
GINKGO_LABEL_FILTER="required && !windows && !aks" make test-e2e

# Run either required or k8s-upgrade tests
GINKGO_LABEL_FILTER="required || k8s-upgrade" make test-e2e

# Run optional tests with API Server ILB
GINKGO_LABEL_FILTER="optional && api-server-ilb" make test-e2e
```

### Using with GINKGO_FOCUS (Backward Compatibility)

You can still use `GINKGO_FOCUS` for text-based filtering:

```bash
# Focus on specific test by name
GINKGO_FOCUS="Creating a highly available cluster" make test-e2e

# Combine label filter with focus
GINKGO_LABEL_FILTER="required" GINKGO_FOCUS="VMSS" make test-e2e
```

## Benefits of Label-Based Filtering

1. **Cleaner Test Names**: Test names no longer contain filtering metadata
2. **More Flexible**: Support for boolean expressions (AND, OR, NOT)
3. **Multiple Labels**: Tests can have multiple labels for better organization
4. **Better Reporting**: Ginkgo reports show labels associated with each test
5. **Standard Approach**: Uses Ginkgo's built-in feature rather than custom parsing

## Migration for CI/CD

CI jobs should be updated to use `GINKGO_LABEL_FILTER` instead of text-based `GINKGO_FOCUS`:

**Before:**
```bash
GINKGO_FOCUS="Workload cluster creation" GINKGO_SKIP=".*Windows.*|.*AKS.*"
```

**After:**
```bash
GINKGO_LABEL_FILTER="required && !windows && !aks"
```

## Label Reference

| Label | Description | Used By |
|-------|-------------|---------|
| `required` | Essential tests for PR validation | Pre-submit CI |
| `optional` | Optional tests (flaky, resource-intensive) | Periodic CI |
| `windows` | Tests with Windows worker nodes | Windows-specific CI |
| `aks` | AKS/managed Kubernetes tests | AKS-specific CI |
| `k8s-upgrade` | Kubernetes version upgrade tests | Upgrade testing |
| `api-server-ilb` | API Server ILB feature tests | Feature-specific testing |

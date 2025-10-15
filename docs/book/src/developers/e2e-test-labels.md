# E2E Test Label Migration Guide

## Overview

CAPZ e2e tests now support **both** legacy regex-based filtering and modern Ginkgo label-based filtering. This provides a smooth migration path while maintaining full backward compatibility.

## Current State (Backward Compatible)

All tests retain their original `[Label]` markers in test names AND have Ginkgo `Label()` decorators added. This means:

- ✅ Existing CI configurations continue to work unchanged
- ✅ Existing scripts using `GINKGO_FOCUS` and `GINKGO_SKIP` work as before
- ✅ New label-based filtering is available as an opt-in feature
- ✅ Both approaches can be used simultaneously

## Test Label Mapping

| Test Name Marker | Ginkgo Label | Description |
|------------------|--------------|-------------|
| `[REQUIRED]` | `Label("required")` | Essential tests for PR validation |
| `[OPTIONAL]` | `Label("optional")` | Optional tests (resource-intensive, flaky) |
| `[K8s-Upgrade]` | `Label("k8s-upgrade")` | Kubernetes version upgrade tests |
| `[Managed Kubernetes]` | `Label("aks")` | AKS/managed cluster tests |
| `[API-Server-ILB]` | `Label("api-server-ilb")` | API Server ILB feature tests |
| Windows tests (It level) | `Label("windows")` | Tests with Windows worker nodes |
| Azure CNI v1 (When level) | `Label("azure-cni-v1")` | Azure CNI v1 tests |

## Usage

### Legacy Approach (Still Works!)

**Default behavior** - runs tests matching `[REQUIRED]` marker:
```bash
make test-e2e
```

**Filter by regex pattern:**
```bash
# Run only Windows tests
GINKGO_FOCUS=".*Windows.*" make test-e2e

# Run AKS tests
GINKGO_FOCUS=".*AKS.*" make test-e2e

# Run specific test
GINKGO_FOCUS="Creating a highly available cluster" make test-e2e

# Skip certain tests
GINKGO_FOCUS="Workload cluster creation" GINKGO_SKIP="GPU|Windows" make test-e2e
```

### New Label-Based Approach (Recommended)

**Filter by label expression:**
```bash
# Run only required tests (using labels)
GINKGO_LABEL_FILTER="required" make test-e2e

# Run all tests
GINKGO_LABEL_FILTER="" make test-e2e

# Run optional tests
GINKGO_LABEL_FILTER="optional" make test-e2e

# Run AKS tests
GINKGO_LABEL_FILTER="aks" make test-e2e

# Run Windows tests
GINKGO_LABEL_FILTER="windows" make test-e2e

# Run K8s upgrade tests
GINKGO_LABEL_FILTER="k8s-upgrade" make test-e2e
```

### Boolean Expressions with Labels

Labels support powerful boolean expressions:

```bash
# Required tests excluding Windows
GINKGO_LABEL_FILTER="required && !windows" make test-e2e

# Required tests excluding Windows and AKS
GINKGO_LABEL_FILTER="required && !windows && !aks" make test-e2e

# Run either required or k8s-upgrade tests
GINKGO_LABEL_FILTER="required || k8s-upgrade" make test-e2e

# Optional tests with API Server ILB
GINKGO_LABEL_FILTER="optional && api-server-ilb" make test-e2e

# All tests except Windows
GINKGO_LABEL_FILTER="!windows" make test-e2e
```

### Combining Both Approaches

When `GINKGO_LABEL_FILTER` is set, it takes precedence but can be combined:

```bash
# Filter by label, then focus on specific test name
GINKGO_LABEL_FILTER="required" GINKGO_FOCUS="VMSS" make test-e2e
```

## Behavior Details

### When GINKGO_LABEL_FILTER is NOT set (default):
- Uses traditional `GINKGO_FOCUS` (default: `\[REQUIRED\]`)
- Uses traditional `GINKGO_SKIP`
- Existing behavior is preserved

### When GINKGO_LABEL_FILTER is set:
- Label filtering is applied first
- `GINKGO_FOCUS` and `GINKGO_SKIP` can still be used for additional filtering
- Empty string `GINKGO_LABEL_FILTER=""` runs all tests

## Migration Path for CI/CD

### Current CI Configuration (Still Works)
```bash
# Pre-submit: Required tests without Windows/AKS
GINKGO_FOCUS="Workload cluster creation" GINKGO_SKIP="GPU|.*Windows.*|.*AKS.*"
```

### Recommended New Configuration
```bash
# Pre-submit: Required tests without Windows/AKS
GINKGO_LABEL_FILTER="required && !windows && !aks"
```

### Migration Strategy

1. **Phase 1 (Current)**: Both approaches available, existing CI unchanged
2. **Phase 2 (Gradual)**: Update CI jobs one by one to use label filters
3. **Phase 3 (Future)**: Consider removing `[Label]` markers from test names once all CI is migrated

## Examples for Common Scenarios

### Scenario: Run pre-submit tests locally
```bash
# Legacy approach
GINKGO_FOCUS="\[REQUIRED\]" GINKGO_SKIP="Windows|AKS" make test-e2e

# New approach (cleaner)
GINKGO_LABEL_FILTER="required && !windows && !aks" make test-e2e
```

### Scenario: Run only Windows tests
```bash
# Legacy approach
GINKGO_FOCUS=".*Windows.*" make test-e2e

# New approach
GINKGO_LABEL_FILTER="windows" make test-e2e
```

### Scenario: Run all optional tests
```bash
# Legacy approach
GINKGO_FOCUS="\[OPTIONAL\]" make test-e2e

# New approach
GINKGO_LABEL_FILTER="optional" make test-e2e
```

### Scenario: Run specific test by name
```bash
# Legacy approach (still works)
GINKGO_FOCUS="Creating a VMSS cluster" make test-e2e

# Or combine with labels
GINKGO_LABEL_FILTER="required" GINKGO_FOCUS="VMSS" make test-e2e
```

## Benefits of Label-Based Filtering

1. **Cleaner Expressions**: `"required && !windows"` vs `"GINKGO_FOCUS='\[REQUIRED\]' GINKGO_SKIP='Windows'"`
2. **More Powerful**: Boolean logic (AND, OR, NOT) for complex scenarios
3. **Multiple Labels**: Tests can have multiple labels for better organization
4. **Better Reporting**: Ginkgo shows labels in test output
5. **Standard Feature**: Uses Ginkgo's built-in functionality

## FAQ

**Q: Do I need to change my existing CI scripts?**
A: No, existing scripts continue to work unchanged.

**Q: What happens if I set both GINKGO_LABEL_FILTER and GINKGO_FOCUS?**
A: Label filter is applied first, then focus/skip patterns are applied to the filtered results.

**Q: Can I run all tests?**
A: Yes, use `GINKGO_LABEL_FILTER=""` or `GINKGO_FOCUS="" GINKGO_SKIP=""`.

**Q: Are test names changing?**
A: No, test names retain their `[Label]` markers for backward compatibility.

**Q: When should I migrate to label-based filtering?**
A: Migrate at your convenience. There's no rush - both approaches are fully supported.

**Q: Can I use label filters in scripts?**
A: Yes, for example: `GINKGO_LABEL_FILTER="required && !windows && !aks" ./scripts/ci-e2e.sh`

## Support

For questions or issues:
- Review test files: `test/e2e/azure_test.go`, `test/e2e/capi_test.go`
- Check Makefile variables: `Makefile` (line ~184)
- See Ginkgo docs: https://onsi.github.io/ginkgo/#filtering-specs

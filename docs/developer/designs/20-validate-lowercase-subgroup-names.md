# Validate Lowercase SubGroup Names

## Overview

This document proposes adding validation to enforce lowercase subgroup names in PodGroup resources. Currently, the scheduler silently lowercases subgroup parent names in `formatParentName()`, creating a hidden assumption that can lead to confusing behavior when users create subgroups with mixed-case names.

## Problem Statement

The scheduler assumes subgroup names are lowercase in `pkg/scheduler/api/podgroup_info/subgroup_info/factory.go`:

```go
func formatParentName(parentName *string) string {
    if parentName == nil {
        return RootSubGroupSetName
    }
    return strings.ToLower(*parentName)
}
```

However, the existing PodGroup admission webhook (`pkg/apis/scheduling/v2alpha2/podgroup_webhook.go`) only validates:
- Duplicate subgroup names
- Valid parent references
- Cycle detection

It does **not** enforce lowercase names. This creates a gap where:
1. A user creates a subgroup named `"Workers"` with parent `"Master"`
2. The webhook accepts this as valid
3. The scheduler lowercases the parent to `"master"`, which may not exist
4. Silent failures or unexpected behavior occur

## Goals

- **Validate lowercase subgroup names** at admission time to fail fast with clear error messages
- **Validate lowercase parent references** to ensure parent lookups work correctly
- **Maintain backward compatibility** - existing valid (lowercase) PodGroups continue to work
- **Provide clear error messages** that guide users to the correct format

## Non-Goals

- **Auto-converting mixed-case names to lowercase** - explicit validation is preferred over silent conversion
- **Changing the scheduler's internal behavior** - the `formatParentName()` function remains unchanged
- **Adding DNS-1123 label validation** - subgroup names are internal identifiers, not Kubernetes object names

## Alternatives Considered

### Alternative 1: Remove the Lowercase Requirement (Not Recommended)

Remove `strings.ToLower()` from `formatParentName()` and allow any case.

**Pros:**
- No validation changes needed
- More flexible for users

**Cons:**
- Breaking change - existing workloads using lowercase might break if mixed with uppercase
- Inconsistent with Kubernetes naming conventions (DNS labels are lowercase)
- Requires audit of all code paths that compare subgroup names

**Decision:** Rejected - too risky and inconsistent with Kubernetes patterns.

### Alternative 2: Kubebuilder Pattern Validation (Not Recommended for this case)

Add a kubebuilder validation pattern to the SubGroup struct:

```go
// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
Name string `json:"name"`
```

**Pros:**
- Declarative, self-documenting
- CEL/regex validation at CRD level

**Cons:**
- Cannot provide custom error messages
- Need separate validation for the `Parent` field (which is a pointer)
- Pattern validation errors are generic and less helpful

**Decision:** Rejected - custom webhook validation provides better error messages.

### Alternative 3: Webhook Validation with `validateSubGroupName()` (Recommended)

Add a dedicated validation function in the existing webhook that checks each subgroup name and parent reference.

**Pros:**
- Clear, specific error messages
- Consistent with existing validation pattern
- Easy to test and maintain
- No API changes required

**Cons:**
- Runtime validation only (not at CRD schema level)

**Decision:** Accepted - simplest approach with best user experience.

## Design Proposal

### Implementation

Add a new validation function `validateSubGroupName()` and integrate it into the existing `validateSubGroups()` function:

```go
// validateSubGroupName checks if a subgroup name is valid (lowercase alphanumeric with hyphens)
func validateSubGroupName(name string) error {
    if name != strings.ToLower(name) {
        return fmt.Errorf("subgroup name %q must be lowercase", name)
    }
    return nil
}

func validateSubGroups(subGroups []SubGroup) error {
    subGroupMap := map[string]*SubGroup{}
    for _, subGroup := range subGroups {
        // Validate subgroup name is lowercase
        if err := validateSubGroupName(subGroup.Name); err != nil {
            return err
        }
        
        // Validate parent name is lowercase (if specified)
        if subGroup.Parent != nil {
            if err := validateSubGroupName(*subGroup.Parent); err != nil {
                return fmt.Errorf("parent of subgroup %q: %w", subGroup.Name, err)
            }
        }
        
        if subGroupMap[subGroup.Name] != nil {
            return fmt.Errorf("duplicate subgroup name %s", subGroup.Name)
        }
        subGroupMap[subGroup.Name] = &subGroup
    }

    if err := validateParent(subGroupMap); err != nil {
        return err
    }

    if detectCycle(subGroupMap) {
        return errors.New("cycle detected in subgroups")
    }
    return nil
}
```

### Validation Rules

The validation enforces:
1. **Subgroup names must be lowercase** - `"workers"` is valid, `"Workers"` is not
2. **Parent references must be lowercase** - `parent: "master"` is valid, `parent: "Master"` is not
3. **Existing validations remain unchanged** - the following validations already exist in `validateSubGroups()` and will continue to function as before:
   - **Duplicate name detection**: Rejects PodGroups where two subgroups have the same name
   - **Parent existence validation**: Ensures that if a subgroup specifies a `parent`, that parent subgroup exists in the list
   - **Cycle detection**: Prevents circular parent references (e.g., A→B→C→A)

### Error Messages

Clear, actionable error messages:
- `subgroup name "Workers" must be lowercase`
- `parent of subgroup "worker-1": subgroup name "Master" must be lowercase`

## Examples

### Example 1: Valid PodGroup with Lowercase SubGroups

```yaml
apiVersion: scheduling.run.ai/v2alpha2
kind: PodGroup
metadata:
  name: training-job
  namespace: default
spec:
  minMember: 2
  queue: default
  subGroups:
    - name: master
      minMember: 1
    - name: workers
      parent: master
      minMember: 4
```

**Result:** Accepted ✓

### Example 2: Invalid - Mixed Case SubGroup Name

```yaml
apiVersion: scheduling.run.ai/v2alpha2
kind: PodGroup
metadata:
  name: training-job
  namespace: default
spec:
  minMember: 2
  queue: default
  subGroups:
    - name: Master        # Invalid: uppercase
      minMember: 1
    - name: workers
      parent: master
      minMember: 4
```

**Result:** Rejected with error: `subgroup name "Master" must be lowercase`

### Example 3: Invalid - Mixed Case Parent Reference

```yaml
apiVersion: scheduling.run.ai/v2alpha2
kind: PodGroup
metadata:
  name: training-job
  namespace: default
spec:
  minMember: 2
  queue: default
  subGroups:
    - name: master
      minMember: 1
    - name: workers
      parent: Master      # Invalid: uppercase parent reference
      minMember: 4
```

**Result:** Rejected with error: `parent of subgroup "workers": subgroup name "Master" must be lowercase`

### Example 4: Valid - Hierarchical SubGroups

```yaml
apiVersion: scheduling.run.ai/v2alpha2
kind: PodGroup
metadata:
  name: inference-workload
  namespace: default
spec:
  minMember: 2
  queue: gpu-queue
  subGroups:
    - name: decode
      minMember: 2
    - name: decode-workers
      parent: decode
      minMember: 4
    - name: decode-leaders
      parent: decode
      minMember: 1
    - name: prefill
      minMember: 2
    - name: prefill-workers
      parent: prefill
      minMember: 4
```

**Result:** Accepted ✓

### Example 5: Invalid - CamelCase Names

```yaml
apiVersion: scheduling.run.ai/v2alpha2
kind: PodGroup
metadata:
  name: ml-pipeline
  namespace: default
spec:
  minMember: 3
  queue: default
  subGroups:
    - name: dataLoader     # Invalid: camelCase
      minMember: 1
    - name: modelTrainer   # Invalid: camelCase
      minMember: 2
    - name: resultWriter   # Invalid: camelCase
      minMember: 1
```

**Result:** Rejected with error: `subgroup name "dataLoader" must be lowercase`

## Test Strategy

### Unit Tests

Add test cases to `pkg/apis/scheduling/v2alpha2/podgroup_webhook_test.go`:

```go
func TestValidateSubGroups(t *testing.T) {
    tests := []struct {
        name      string
        subGroups []SubGroup
        wantErr   error
    }{
        // Existing tests...
        
        {
            name: "Uppercase subgroup name",
            subGroups: []SubGroup{
                {Name: "Master", MinMember: 1},
                {Name: "workers", Parent: ptr.To("master"), MinMember: 4},
            },
            wantErr: errors.New(`subgroup name "Master" must be lowercase`),
        },
        {
            name: "Uppercase parent reference",
            subGroups: []SubGroup{
                {Name: "master", MinMember: 1},
                {Name: "workers", Parent: ptr.To("Master"), MinMember: 4},
            },
            wantErr: errors.New(`parent of subgroup "workers": subgroup name "Master" must be lowercase`),
        },
        {
            name: "CamelCase subgroup name",
            subGroups: []SubGroup{
                {Name: "dataLoader", MinMember: 1},
            },
            wantErr: errors.New(`subgroup name "dataLoader" must be lowercase`),
        },
        {
            name: "Valid lowercase names",
            subGroups: []SubGroup{
                {Name: "master", MinMember: 1},
                {Name: "worker-1", Parent: ptr.To("master"), MinMember: 2},
                {Name: "worker-2", Parent: ptr.To("master"), MinMember: 2},
            },
            wantErr: nil,
        },
        {
            name: "Valid lowercase with numbers",
            subGroups: []SubGroup{
                {Name: "replica-1", MinMember: 1},
                {Name: "replica-2", MinMember: 1},
            },
            wantErr: nil,
        },
    }
    // ... test execution
}

func TestValidateSubGroupName(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"lowercase", "workers", false},
        {"lowercase-with-hyphens", "decode-workers", false},
        {"lowercase-with-numbers", "worker-1", false},
        {"uppercase", "Workers", true},
        {"mixed-case", "decodeWorkers", true},
        {"all-uppercase", "WORKERS", true},
        {"uppercase-with-hyphens", "Decode-Workers", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateSubGroupName(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("validateSubGroupName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
            }
        })
    }
}
```

### Integration Tests

Integration tests should verify the webhook rejects invalid PodGroups via the API server:

1. Create a PodGroup with uppercase subgroup name → expect rejection
2. Create a PodGroup with uppercase parent reference → expect rejection
3. Update a PodGroup to have uppercase subgroup name → expect rejection
4. Create a valid PodGroup with lowercase names → expect acceptance

### Coverage Target

- Unit test coverage for validation logic: >90%
- Integration test coverage for webhook behavior: >80%

## Migration and Backward Compatibility

### Impact Assessment

- **No breaking changes** - this is purely additive validation
- **Existing valid PodGroups** (with lowercase names) continue to work
- **Existing invalid PodGroups** (with mixed-case names) would have been silently broken anyway

### Migration Path

1. **No migration required** - new validation only affects new/updated resources
2. **Documentation update** - add note about lowercase requirement to API docs
3. **Audit existing workloads** (optional) - users can check for mixed-case names

## Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Users have existing PodGroups with mixed-case names | Medium | Low | These were already broken; validation surfaces the issue |
| Validation breaks automation/scripts | Low | Low | Error message clearly indicates the fix |
| Performance impact of additional validation | Negligible | Very Low | Simple string comparison, O(n) where n = subgroups |

## Implementation Plan

### Phase 1: Implementation (1-2 days)

1. Add `validateSubGroupName()` function to `podgroup_webhook.go`
2. Integrate validation into `validateSubGroups()`
3. Add unit tests for all validation scenarios
4. Add integration tests for webhook behavior

### Phase 2: Documentation (0.5 day)

1. Update PodGroup API documentation to note lowercase requirement
2. Add migration guide if needed

### Phase 3: Release

1. Include in next minor release
2. No feature flag needed - this is a bug fix / validation improvement

## References

- Issue: #20 - Validate subgroup names are lowercase on podgroup creation
- Related code:
  - `pkg/apis/scheduling/v2alpha2/podgroup_webhook.go` - webhook validation
  - `pkg/apis/scheduling/v2alpha2/podgroup_types.go` - SubGroup type definition
  - `pkg/scheduler/api/podgroup_info/subgroup_info/factory.go` - `formatParentName()` function

---
*Designed by KAI Agent*

# Validate Subgroup Names Are Lowercase

**Issue:** [#15](https://github.com/omer-dayan/KAI-Scheduler/issues/15)  
**Status:** Draft  
**Authors:** KAI Design Agent  
**Created:** 2026-02-08

## Summary

Add validation to the PodGroup admission webhook to enforce that subgroup names are lowercase. The scheduler internally converts subgroup names to lowercase when processing parent references, creating a hidden assumption that can lead to unexpected behavior when users specify mixed-case subgroup names.

## Problem Statement

The scheduler's `formatParentName` function in `pkg/scheduler/api/podgroup_info/subgroup_info/factory.go` silently converts parent names to lowercase:

```go
func formatParentName(parentName *string) string {
    if parentName == nil {
        return RootSubGroupSetName
    }
    return strings.ToLower(*parentName)
}
```

This creates a hidden assumption that is not validated at API admission time. When a user creates a PodGroup with mixed-case subgroup names (e.g., `MyWorker` and `myworker`), the scheduler treats these as the same subgroup, which can cause:

1. **Silent failures**: Parent references may resolve to unintended subgroups
2. **Confusing behavior**: Users don't receive feedback that their naming doesn't match expectations
3. **Debugging difficulty**: Issues manifest at scheduling time rather than creation time

## Goals

1. **Enforce lowercase subgroup names** at PodGroup creation and update time via the existing validating webhook
2. **Provide clear error messages** that guide users to correct naming
3. **Maintain backward compatibility** with existing valid PodGroups (those already using lowercase names)
4. **Document the naming requirement** in the API field comments

## Non-Goals

1. **Removing the lowercase requirement** - The scheduler's internal logic depends on this behavior for parent matching
2. **Auto-correcting names** - Mutating webhooks would change user intent; validation with clear errors is preferred
3. **Supporting case-insensitive matching** - Would require significant scheduler changes and add complexity
4. **Validating subgroup names in pod labels** - Out of scope; this focuses on PodGroup spec validation

## Alternatives Considered

### Alternative 1: Remove the Lowercase Requirement (Not Recommended)

**Approach:** Modify the scheduler to preserve case in parent name matching.

**Pros:**
- More flexible naming for users
- Aligns with Kubernetes naming conventions (which allow mixed case in some contexts)

**Cons:**
- Requires changes to scheduler internals (`formatParentName` and related logic)
- Risk of breaking existing workloads that rely on current behavior
- Higher complexity and testing burden
- Mixed-case names can lead to confusion (is `Worker` different from `worker`?)

**Decision:** Rejected. The current lowercase behavior is intentional and provides consistency. Validating early is simpler and safer.

### Alternative 2: Add Kubebuilder Validation Pattern (Considered)

**Approach:** Use `+kubebuilder:validation:Pattern` annotation on the SubGroup.Name field.

```go
// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
Name string `json:"name"`
```

**Pros:**
- Declarative, generated into CRD schema
- Enforced by Kubernetes API server natively

**Cons:**
- Pattern validation alone doesn't provide helpful error messages
- Users see generic "pattern mismatch" errors
- Harder to explain the "why" in the error message

**Decision:** Use both - kubebuilder pattern for CRD-level enforcement plus webhook validation for better error messages.

### Alternative 3: Webhook Validation Only (Chosen)

**Approach:** Add a validation function in the existing `validateSubGroups` function.

**Pros:**
- Simplest implementation - single location to modify
- Clear, descriptive error messages
- Follows existing validation patterns in the codebase
- Can be combined with kubebuilder pattern for defense-in-depth

**Cons:**
- Requires webhook to be running (but it's already required for other validations)

**Decision:** Chosen as the primary approach, with optional kubebuilder pattern addition.

## Design

### API Changes

Update the SubGroup struct in `pkg/apis/scheduling/v2alpha2/podgroup_types.go`:

```go
type SubGroup struct {
    // Name uniquely identifies the SubGroup within the PodGroup.
    // Must be lowercase and match the pattern [a-z0-9]([-a-z0-9]*[a-z0-9])?.
    // Examples: "workers", "decode-leaders", "replica-1"
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=63
    // +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
    Name string `json:"name"`

    // MinMember defines the minimal number of members to run this SubGroup;
    // if there are not enough resources to start all required members, the scheduler will not start anyone.
    // +kubebuilder:validation:Minimum=1
    MinMember int32 `json:"minMember,omitempty"`

    // Parent is an optional attribute that specifies the name of the parent SubGroup.
    // Must reference an existing subgroup name (which must also be lowercase).
    // +kubebuilder:validation:Optional
    Parent *string `json:"parent,omitempty"`

    // TopologyConstraint defines the topology constraints for this SubGroup
    TopologyConstraint *TopologyConstraint `json:"topologyConstraint,omitempty"`
}
```

### Webhook Validation Changes

Update `pkg/apis/scheduling/v2alpha2/podgroup_webhook.go`:

```go
import (
    "regexp"
    // ... existing imports
)

// lowercaseNamePattern matches valid lowercase subgroup names.
// Format: starts with alphanumeric, contains only lowercase alphanumeric and hyphens,
// ends with alphanumeric. Similar to DNS label format but lowercase only.
var lowercaseNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func validateSubGroups(subGroups []SubGroup) error {
    subGroupMap := map[string]*SubGroup{}
    for _, subGroup := range subGroups {
        // Validate lowercase naming
        if err := validateSubGroupName(subGroup.Name); err != nil {
            return err
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

// validateSubGroupName checks that a subgroup name is valid.
// Names must be lowercase, start and end with alphanumeric characters,
// and may contain hyphens in the middle.
func validateSubGroupName(name string) error {
    if name == "" {
        return fmt.Errorf("subgroup name cannot be empty")
    }
    if len(name) > 63 {
        return fmt.Errorf("subgroup name %q exceeds maximum length of 63 characters", name)
    }
    if !lowercaseNamePattern.MatchString(name) {
        // Check if it's specifically a case issue to provide better error message
        if lowercaseNamePattern.MatchString(strings.ToLower(name)) {
            return fmt.Errorf("subgroup name %q must be lowercase; use %q instead", name, strings.ToLower(name))
        }
        return fmt.Errorf("subgroup name %q is invalid: must consist of lowercase alphanumeric characters or '-', start with an alphanumeric character, and end with an alphanumeric character", name)
    }
    return nil
}
```

### Component Interactions

```mermaid
sequenceDiagram
    participant User
    participant API as Kubernetes API Server
    participant Webhook as PodGroup Webhook
    participant Scheduler as KAI Scheduler

    User->>API: Create PodGroup with subgroups
    API->>Webhook: ValidateCreate()
    
    alt Invalid subgroup name (e.g., "MyWorker")
        Webhook-->>API: Error: "subgroup name 'MyWorker' must be lowercase; use 'myworker' instead"
        API-->>User: Admission denied with error
    else Valid lowercase names
        Webhook-->>API: Admission allowed
        API->>API: Store PodGroup
        API-->>User: PodGroup created
        
        Note over Scheduler: Later, during scheduling
        Scheduler->>API: Watch PodGroup
        Scheduler->>Scheduler: Process subgroups (already lowercase)
    end
```

## Examples

### Example 1: Valid PodGroup with Lowercase Subgroup Names

```yaml
apiVersion: scheduling.kai.io/v2alpha2
kind: PodGroup
metadata:
  name: inference-workload
  namespace: default
spec:
  minMember: 2
  queue: default
  subGroups:
    - name: decode-workers    # Valid: lowercase with hyphen
      minMember: 4
    - name: prefill           # Valid: simple lowercase
      minMember: 2
      parent: decode-workers
    - name: replica1          # Valid: lowercase with number
      minMember: 1
```

**Result:** PodGroup created successfully.

### Example 2: Invalid PodGroup with Mixed-Case Name

```yaml
apiVersion: scheduling.kai.io/v2alpha2
kind: PodGroup
metadata:
  name: inference-workload
  namespace: default
spec:
  minMember: 2
  queue: default
  subGroups:
    - name: DecodeWorkers     # Invalid: contains uppercase
      minMember: 4
    - name: prefill
      minMember: 2
```

**Result:** Admission denied with error:
```
Error from server: error when creating "podgroup.yaml": admission webhook 
"vpodgroup.kb.io" denied the request: subgroup name "DecodeWorkers" must 
be lowercase; use "decodeworkers" instead
```

### Example 3: Invalid PodGroup with Underscore (Invalid Character)

```yaml
apiVersion: scheduling.kai.io/v2alpha2
kind: PodGroup
metadata:
  name: inference-workload
  namespace: default
spec:
  minMember: 1
  queue: default
  subGroups:
    - name: decode_workers    # Invalid: contains underscore
      minMember: 4
```

**Result:** Admission denied with error:
```
Error from server: error when creating "podgroup.yaml": admission webhook 
"vpodgroup.kb.io" denied the request: subgroup name "decode_workers" is 
invalid: must consist of lowercase alphanumeric characters or '-', start 
with an alphanumeric character, and end with an alphanumeric character
```

### Example 4: Invalid Name Starting with Hyphen

```yaml
apiVersion: scheduling.kai.io/v2alpha2
kind: PodGroup
metadata:
  name: inference-workload
  namespace: default
spec:
  minMember: 1
  queue: default
  subGroups:
    - name: -workers          # Invalid: starts with hyphen
      minMember: 4
```

**Result:** Admission denied with error:
```
Error from server: error when creating "podgroup.yaml": admission webhook 
"vpodgroup.kb.io" denied the request: subgroup name "-workers" is invalid: 
must consist of lowercase alphanumeric characters or '-', start with an 
alphanumeric character, and end with an alphanumeric character
```

### Example 5: Hierarchical PodGroup with Valid Names

```yaml
apiVersion: scheduling.kai.io/v2alpha2
kind: PodGroup
metadata:
  name: multi-tier-workload
  namespace: default
spec:
  minMember: 2
  queue: default
  subGroups:
    - name: tier1
      minMember: 2
    - name: tier1-leaders
      minMember: 1
      parent: tier1
    - name: tier1-workers
      minMember: 3
      parent: tier1
    - name: tier2
      minMember: 2
    - name: tier2-leaders
      minMember: 1
      parent: tier2
    - name: tier2-workers
      minMember: 3
      parent: tier2
```

**Result:** PodGroup created successfully. All names are lowercase with valid characters.

## Test Strategy

### Unit Tests

Add test cases to `pkg/apis/scheduling/v2alpha2/podgroup_webhook_test.go`:

```go
func TestValidateSubGroupName(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
        errMsg  string
    }{
        // Valid names
        {name: "simple lowercase", input: "workers", wantErr: false},
        {name: "with hyphen", input: "decode-workers", wantErr: false},
        {name: "with numbers", input: "replica1", wantErr: false},
        {name: "single char", input: "a", wantErr: false},
        {name: "number only", input: "1", wantErr: false},
        {name: "complex valid", input: "tier-1-workers-group", wantErr: false},
        
        // Invalid names - uppercase
        {name: "uppercase start", input: "Workers", wantErr: true, errMsg: "must be lowercase"},
        {name: "mixed case", input: "decodeWorkers", wantErr: true, errMsg: "must be lowercase"},
        {name: "all uppercase", input: "WORKERS", wantErr: true, errMsg: "must be lowercase"},
        
        // Invalid names - invalid characters
        {name: "underscore", input: "decode_workers", wantErr: true, errMsg: "is invalid"},
        {name: "space", input: "decode workers", wantErr: true, errMsg: "is invalid"},
        {name: "special char", input: "workers!", wantErr: true, errMsg: "is invalid"},
        
        // Invalid names - boundary issues
        {name: "starts with hyphen", input: "-workers", wantErr: true, errMsg: "is invalid"},
        {name: "ends with hyphen", input: "workers-", wantErr: true, errMsg: "is invalid"},
        {name: "empty string", input: "", wantErr: true, errMsg: "cannot be empty"},
        
        // Edge cases
        {name: "max length 63", input: strings.Repeat("a", 63), wantErr: false},
        {name: "exceeds max length", input: strings.Repeat("a", 64), wantErr: true, errMsg: "exceeds maximum length"},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateSubGroupName(tt.input)
            if tt.wantErr {
                if err == nil {
                    t.Errorf("expected error containing %q, got nil", tt.errMsg)
                } else if !strings.Contains(err.Error(), tt.errMsg) {
                    t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
                }
            } else if err != nil {
                t.Errorf("expected no error, got %v", err)
            }
        })
    }
}

func TestValidateSubGroups_LowercaseValidation(t *testing.T) {
    tests := []struct {
        name      string
        subGroups []SubGroup
        wantErr   error
    }{
        {
            name: "Valid lowercase names",
            subGroups: []SubGroup{
                {Name: "workers", MinMember: 1},
                {Name: "leaders", MinMember: 1},
            },
            wantErr: nil,
        },
        {
            name: "Invalid uppercase in first subgroup",
            subGroups: []SubGroup{
                {Name: "Workers", MinMember: 1},  // uppercase W
                {Name: "leaders", MinMember: 1},
            },
            wantErr: errors.New(`subgroup name "Workers" must be lowercase; use "workers" instead`),
        },
        {
            name: "Invalid uppercase in parent reference",
            subGroups: []SubGroup{
                {Name: "parent", MinMember: 1},
                {Name: "Child", MinMember: 1, Parent: ptr.To("parent")},  // uppercase C
            },
            wantErr: errors.New(`subgroup name "Child" must be lowercase; use "child" instead`),
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateSubGroups(tt.subGroups)
            if (err != nil && tt.wantErr == nil) || (err == nil && tt.wantErr != nil) {
                t.Fatalf("expected error %v, got %v", tt.wantErr, err)
            }
            if err != nil && tt.wantErr != nil && err.Error() != tt.wantErr.Error() {
                t.Fatalf("expected error %v, got %v", tt.wantErr, err)
            }
        })
    }
}
```

### Integration Tests

Add integration tests using envtest to verify webhook behavior:

1. **Test valid PodGroup creation** - Verify lowercase names are accepted
2. **Test invalid PodGroup rejection** - Verify mixed-case names are rejected with proper error
3. **Test PodGroup update** - Verify validation applies to updates as well

### E2E Tests

1. **Create PodGroup with valid names** - Verify scheduling works correctly
2. **Attempt to create PodGroup with invalid names** - Verify admission is denied
3. **Verify existing PodGroups continue to work** - Backward compatibility test

### Coverage Goal

Target **>90% coverage** for the new `validateSubGroupName` function and updated `validateSubGroups` function.

## Migration and Backward Compatibility

### Backward Compatibility

This change is **fully backward compatible**:

1. **Existing valid PodGroups** (those using lowercase subgroup names) will continue to work without modification
2. **The scheduler's behavior** remains unchanged - it already expects lowercase names internally
3. **No migration required** - this is a validation-only change

### Impact on Existing Workloads

| Scenario | Impact |
|----------|--------|
| PodGroups with lowercase subgroup names | ✅ No impact |
| PodGroups without subgroups | ✅ No impact |
| PodGroups with mixed-case names (if any exist) | ⚠️ Updates will be rejected; new PodGroups with same names will be rejected |

### Rollout Considerations

1. **Audit existing PodGroups** before upgrade to identify any with mixed-case names
2. **Communicate the change** in release notes
3. **No feature flag needed** - this enforces existing implicit behavior

## Risks and Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Existing PodGroups have mixed-case names | Users cannot update those PodGroups | Low | Document in release notes; names were already being lowercased internally |
| Webhook unavailability | Validation bypassed temporarily | Low | Kubernetes retries webhook calls; kubebuilder pattern provides backup |
| Regex performance | Slight admission latency | Very Low | Pattern is simple; validated at admission time only |

## Implementation Plan

### Phase 1: Implementation (This PR)

1. Add `validateSubGroupName` function to webhook
2. Update `validateSubGroups` to call new validation
3. Add comprehensive unit tests
4. Update API field comments with naming requirements
5. Add kubebuilder validation pattern to SubGroup.Name field

### Phase 2: Documentation (Follow-up)

1. Update user-facing documentation with naming requirements
2. Add examples to API reference documentation

## Open Questions

None - the approach is straightforward and aligns with existing patterns.

## References

- [Issue #15](https://github.com/omer-dayan/KAI-Scheduler/issues/15)
- [Kubernetes naming conventions](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/)
- [Hierarchical PodGroup design](docs/developer/designs/hierarchical-podgroup/README.md)

---
🤖 Design by [KAI Design Agent](https://github.com/romanbaron/KAI-Agents)

# Validate Lowercase SubGroup Names

**Issue:** [#12](https://github.com/omer-dayan/KAI-Scheduler/issues/12)  
**Status:** Proposal  
**Author:** KAI Design Agent

## Overview

This design proposes adding validation to enforce lowercase subgroup names in PodGroup resources. Currently, the scheduler silently lowercases subgroup parent references in `formatParentName()`, creating a hidden assumption that can cause confusing behavior when users specify mixed-case names.

## Problem Statement

The scheduler's `formatParentName()` function in `pkg/scheduler/api/podgroup_info/subgroup_info/factory.go` silently converts parent names to lowercase:

```go
func formatParentName(parentName *string) string {
    if parentName == nil {
        return RootSubGroupSetName
    }
    return strings.ToLower(*parentName)
}
```

This creates a hidden API contract where:
1. Users can create subgroups with mixed-case names (e.g., `DecodeWorkers`)
2. Parent references are silently lowercased (e.g., `parent: DecodeWorkers` becomes `decodeworkers`)
3. If the subgroup name itself isn't lowercased, the parent lookup fails silently or produces unexpected behavior

This violates the principle of explicit API contracts and can cause hard-to-debug scheduling issues.

## Goals

1. **Make the lowercase requirement explicit** by validating subgroup names in the admission webhook
2. **Provide clear error messages** when users specify non-lowercase names
3. **Maintain backward compatibility** for existing PodGroups with lowercase names
4. **Document the API contract** clearly in the CRD field documentation

## Non-Goals

1. **Auto-normalizing names** - We will not automatically lowercase names; instead, we reject invalid input with a clear error
2. **Changing the internal behavior** - The `formatParentName()` function remains unchanged as a defensive measure
3. **Supporting mixed-case names** - This design explicitly chooses to enforce lowercase rather than remove the requirement

## Alternatives Considered

### Alternative 1: Remove the Lowercase Requirement (Not Recommended)

**Approach:** Remove `strings.ToLower()` from `formatParentName()` and allow any case.

**Pros:**
- More flexible for users
- No breaking changes for mixed-case names

**Cons:**
- Case-sensitivity can cause subtle bugs (e.g., `Workers` vs `workers` being different)
- Kubernetes conventions favor lowercase names (e.g., label values, resource names)
- Would require careful migration to avoid breaking existing deployments that rely on the current behavior

**Decision:** Not recommended. Lowercase enforcement aligns with Kubernetes naming conventions and prevents case-related bugs.

### Alternative 2: Validate Lowercase Names in Webhook (Recommended)

**Approach:** Add validation in the PodGroup admission webhook to reject non-lowercase subgroup names.

**Pros:**
- Makes the API contract explicit and discoverable
- Provides immediate feedback to users
- Aligns with Kubernetes conventions
- Simple to implement with minimal code changes

**Cons:**
- Breaking change for users who have been using mixed-case names (though this was never intentionally supported)

**Decision:** Recommended. This is the simplest solution that makes the existing behavior explicit.

### Alternative 3: Case-Insensitive Comparison with Normalization Warning

**Approach:** Accept any case but emit a warning and normalize internally.

**Pros:**
- Graceful handling of mixed-case input
- Non-breaking for existing resources

**Cons:**
- More complex implementation
- Warnings are often ignored
- Inconsistent behavior between what users specify and what the system uses

**Decision:** Not recommended. Warnings are often ignored, and silent normalization is confusing.

## Design Proposal

### API Changes

Add a kubebuilder validation pattern to the SubGroup `Name` field to enforce lowercase names:

```go
type SubGroup struct {
    // Name uniquely identifies the SubGroup within the PodGroup.
    // Must be lowercase and may contain lowercase letters, numbers, and hyphens.
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=63
    // +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
    Name string `json:"name"`

    // MinMember defines the minimal number of members to run this SubGroup;
    // if there are not enough resources to start all required members, the scheduler will not start anyone.
    // +kubebuilder:validation:Minimum=1
    MinMember int32 `json:"minMember,omitempty"`

    // Parent is an optional attribute that specifies the name of the parent SubGroup.
    // Must reference an existing SubGroup name (lowercase).
    // +kubebuilder:validation:Optional
    // +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
    Parent *string `json:"parent,omitempty"`

    // TopologyConstraint defines the topology constraints for this SubGroup
    TopologyConstraint *TopologyConstraint `json:"topologyConstraint,omitempty"`
}
```

### Webhook Validation Changes

Add explicit validation in `pkg/apis/scheduling/v2alpha2/podgroup_webhook.go`:

```go
func validateSubGroups(subGroups []SubGroup) error {
    subGroupMap := map[string]*SubGroup{}
    for _, subGroup := range subGroups {
        // Validate lowercase name
        if err := validateSubGroupName(subGroup.Name); err != nil {
            return fmt.Errorf("invalid subgroup name %q: %w", subGroup.Name, err)
        }
        
        // Validate lowercase parent reference
        if subGroup.Parent != nil {
            if err := validateSubGroupName(*subGroup.Parent); err != nil {
                return fmt.Errorf("invalid parent name %q for subgroup %q: %w", 
                    *subGroup.Parent, subGroup.Name, err)
            }
        }
        
        // Existing duplicate check
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

// validateSubGroupName validates that a subgroup name follows DNS-1123 label format
// and is lowercase.
func validateSubGroupName(name string) error {
    if name == "" {
        return fmt.Errorf("name cannot be empty")
    }
    if len(name) > 63 {
        return fmt.Errorf("name must be no more than 63 characters")
    }
    // DNS-1123 label: lowercase alphanumeric, may contain hyphens, 
    // must start and end with alphanumeric
    pattern := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
    if !pattern.MatchString(name) {
        return fmt.Errorf("must consist of lowercase alphanumeric characters or '-', "+
            "must start and end with an alphanumeric character")
    }
    return nil
}
```

## Examples

### Example 1: Valid Lowercase SubGroup Names

```yaml
apiVersion: scheduling.run.ai/v2alpha2
kind: PodGroup
metadata:
  name: my-workload
  namespace: default
spec:
  minMember: 2
  queue: default
  subGroups:
    - name: decode-workers    # Valid: lowercase with hyphen
      minMember: 4
    - name: prefill           # Valid: lowercase
      minMember: 2
      parent: decode-workers
```

### Example 2: Invalid Mixed-Case Name (Rejected)

```yaml
apiVersion: scheduling.run.ai/v2alpha2
kind: PodGroup
metadata:
  name: my-workload
  namespace: default
spec:
  minMember: 2
  queue: default
  subGroups:
    - name: DecodeWorkers     # Invalid: contains uppercase
      minMember: 4
```

**Error message:**
```
Error from server: error when creating "podgroup.yaml": admission webhook 
"podgroupcontroller.run.ai" denied the request: invalid subgroup name 
"DecodeWorkers": must consist of lowercase alphanumeric characters or '-', 
must start and end with an alphanumeric character
```

### Example 3: Invalid Parent Reference (Rejected)

```yaml
apiVersion: scheduling.run.ai/v2alpha2
kind: PodGroup
metadata:
  name: my-workload
  namespace: default
spec:
  minMember: 2
  queue: default
  subGroups:
    - name: workers
      minMember: 4
    - name: leaders
      minMember: 1
      parent: Workers        # Invalid: uppercase parent reference
```

**Error message:**
```
Error from server: error when creating "podgroup.yaml": admission webhook 
"podgroupcontroller.run.ai" denied the request: invalid parent name "Workers" 
for subgroup "leaders": must consist of lowercase alphanumeric characters or '-', 
must start and end with an alphanumeric character
```

### Example 4: Invalid Characters (Rejected)

```yaml
apiVersion: scheduling.run.ai/v2alpha2
kind: PodGroup
metadata:
  name: my-workload
  namespace: default
spec:
  minMember: 1
  queue: default
  subGroups:
    - name: decode_workers   # Invalid: underscore not allowed
      minMember: 4
```

**Error message:**
```
Error from server: error when creating "podgroup.yaml": admission webhook 
"podgroupcontroller.run.ai" denied the request: invalid subgroup name 
"decode_workers": must consist of lowercase alphanumeric characters or '-', 
must start and end with an alphanumeric character
```

### Example 5: Hierarchical SubGroups with Valid Names

```yaml
apiVersion: scheduling.run.ai/v2alpha2
kind: PodGroup
metadata:
  name: inference-workload
  namespace: ml-team
spec:
  minMember: 2
  queue: inference
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
    - name: prefill-leaders
      parent: prefill
      minMember: 1
```

## Test Strategy

### Unit Tests

Add tests to `pkg/apis/scheduling/v2alpha2/podgroup_webhook_test.go`:

```go
func TestValidateSubGroupName(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {name: "valid lowercase", input: "workers", wantErr: false},
        {name: "valid with hyphen", input: "decode-workers", wantErr: false},
        {name: "valid with numbers", input: "worker1", wantErr: false},
        {name: "valid single char", input: "a", wantErr: false},
        {name: "invalid uppercase", input: "Workers", wantErr: true},
        {name: "invalid mixed case", input: "decodeWorkers", wantErr: true},
        {name: "invalid underscore", input: "decode_workers", wantErr: true},
        {name: "invalid starts with hyphen", input: "-workers", wantErr: true},
        {name: "invalid ends with hyphen", input: "workers-", wantErr: true},
        {name: "invalid empty", input: "", wantErr: true},
        {name: "invalid too long", input: strings.Repeat("a", 64), wantErr: true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateSubGroupName(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("validateSubGroupName(%q) error = %v, wantErr %v", 
                    tt.input, err, tt.wantErr)
            }
        })
    }
}

func TestValidateSubGroups_LowercaseValidation(t *testing.T) {
    tests := []struct {
        name      string
        subGroups []SubGroup
        wantErr   bool
        errMsg    string
    }{
        {
            name: "valid lowercase names",
            subGroups: []SubGroup{
                {Name: "workers", MinMember: 4},
                {Name: "leaders", MinMember: 1, Parent: ptr.To("workers")},
            },
            wantErr: false,
        },
        {
            name: "invalid uppercase name",
            subGroups: []SubGroup{
                {Name: "Workers", MinMember: 4},
            },
            wantErr: true,
            errMsg:  "invalid subgroup name",
        },
        {
            name: "invalid uppercase parent",
            subGroups: []SubGroup{
                {Name: "workers", MinMember: 4},
                {Name: "leaders", MinMember: 1, Parent: ptr.To("Workers")},
            },
            wantErr: true,
            errMsg:  "invalid parent name",
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateSubGroups(tt.subGroups)
            if (err != nil) != tt.wantErr {
                t.Errorf("validateSubGroups() error = %v, wantErr %v", err, tt.wantErr)
            }
            if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
                t.Errorf("validateSubGroups() error = %v, want error containing %q", 
                    err, tt.errMsg)
            }
        })
    }
}
```

### Integration Tests

Add integration tests using envtest to verify webhook behavior end-to-end:

1. Create PodGroup with valid lowercase names → should succeed
2. Create PodGroup with uppercase names → should be rejected by webhook
3. Update PodGroup to add invalid subgroup → should be rejected

### E2E Tests

Add E2E tests to verify the full flow:

1. Deploy PodGroup with valid subgroups → verify scheduling works correctly
2. Attempt to deploy PodGroup with invalid names → verify rejection with proper error message

**Coverage Target:** >90% for validation functions, >80% overall for the webhook module.

## Migration and Backward Compatibility

### Impact Assessment

- **Existing PodGroups with lowercase names:** No impact - continue to work as before
- **Existing PodGroups with mixed-case names:** Will fail validation on update; however, this was never officially supported behavior

### Migration Path

1. Users should audit existing PodGroups for non-lowercase subgroup names
2. Update any non-compliant PodGroups to use lowercase names before upgrading
3. The validation will only affect new creates and updates, not existing resources in etcd

### Rollout Strategy

Since this is a simple validation addition with clear error messages:

1. **No feature flag required** - The validation is a bug fix making implicit behavior explicit
2. **Release notes** should document:
   - SubGroup names must be lowercase (DNS-1123 label format)
   - This was always the intended behavior; validation now enforces it explicitly
3. **Documentation update** to clearly state the naming requirements

## Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Breaking existing PodGroups with mixed-case names | Medium | Low | Document in release notes; mixed-case was never officially supported |
| Users confused by validation errors | Low | Low | Provide clear, actionable error messages |
| Regex pattern too restrictive | Low | Low | Use standard DNS-1123 label pattern from Kubernetes |

## Implementation Checklist

- [ ] Update `SubGroup` struct with kubebuilder validation patterns
- [ ] Add `validateSubGroupName()` function in webhook
- [ ] Update `validateSubGroups()` to call name validation
- [ ] Add unit tests for name validation
- [ ] Add integration tests for webhook rejection
- [ ] Update CRD documentation with naming requirements
- [ ] Add release notes entry
- [ ] Run `make generate` to update generated code

## References

- [Kubernetes DNS-1123 Label Names](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names)
- [Issue #12: Validate subgroup names are lowercase](https://github.com/omer-dayan/KAI-Scheduler/issues/12)
- [Existing SubGroup validation in podgroup_webhook.go](https://github.com/omer-dayan/KAI-Scheduler/blob/main/pkg/apis/scheduling/v2alpha2/podgroup_webhook.go)

---
🤖 Design by [KAI Design Agent](https://github.com/romanbaron/KAI-Agents)

// Copyright 2025 NVIDIA CORPORATION
// SPDX-License-Identifier: Apache-2.0

package v2alpha2

import (
	"errors"
	"testing"

	"k8s.io/utils/ptr"
)

func TestValidateSubGroupName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		// Valid names
		{
			name:    "simple lowercase name",
			input:   "mysubgroup",
			wantErr: false,
		},
		{
			name:    "lowercase with numbers",
			input:   "subgroup123",
			wantErr: false,
		},
		{
			name:    "lowercase with hyphens",
			input:   "my-sub-group",
			wantErr: false,
		},
		{
			name:    "single character",
			input:   "a",
			wantErr: false,
		},
		{
			name:    "single digit",
			input:   "1",
			wantErr: false,
		},
		{
			name:    "starts with number",
			input:   "1abc",
			wantErr: false,
		},
		{
			name:    "ends with number",
			input:   "abc1",
			wantErr: false,
		},
		// Invalid names - uppercase
		{
			name:    "uppercase letters",
			input:   "MySubGroup",
			wantErr: true,
			errMsg:  `subgroup name "MySubGroup" is invalid: must consist of lowercase alphanumeric characters or '-', and must start and end with an alphanumeric character`,
		},
		{
			name:    "all uppercase",
			input:   "SUBGROUP",
			wantErr: true,
			errMsg:  `subgroup name "SUBGROUP" is invalid: must consist of lowercase alphanumeric characters or '-', and must start and end with an alphanumeric character`,
		},
		// Invalid names - special characters
		{
			name:    "underscore",
			input:   "my_subgroup",
			wantErr: true,
			errMsg:  `subgroup name "my_subgroup" is invalid: must consist of lowercase alphanumeric characters or '-', and must start and end with an alphanumeric character`,
		},
		{
			name:    "dot",
			input:   "my.subgroup",
			wantErr: true,
			errMsg:  `subgroup name "my.subgroup" is invalid: must consist of lowercase alphanumeric characters or '-', and must start and end with an alphanumeric character`,
		},
		{
			name:    "space",
			input:   "my subgroup",
			wantErr: true,
			errMsg:  `subgroup name "my subgroup" is invalid: must consist of lowercase alphanumeric characters or '-', and must start and end with an alphanumeric character`,
		},
		// Invalid names - hyphen placement
		{
			name:    "starts with hyphen",
			input:   "-subgroup",
			wantErr: true,
			errMsg:  `subgroup name "-subgroup" is invalid: must consist of lowercase alphanumeric characters or '-', and must start and end with an alphanumeric character`,
		},
		{
			name:    "ends with hyphen",
			input:   "subgroup-",
			wantErr: true,
			errMsg:  `subgroup name "subgroup-" is invalid: must consist of lowercase alphanumeric characters or '-', and must start and end with an alphanumeric character`,
		},
		{
			name:    "only hyphen",
			input:   "-",
			wantErr: true,
			errMsg:  `subgroup name "-" is invalid: must consist of lowercase alphanumeric characters or '-', and must start and end with an alphanumeric character`,
		},
		// Invalid names - empty and length
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
			errMsg:  "subgroup name cannot be empty",
		},
		{
			name:    "exceeds max length",
			input:   "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz01234", // 65 chars
			wantErr: true,
			errMsg:  `subgroup name "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz01234" exceeds maximum length of 63 characters`,
		},
		{
			name:    "exactly max length",
			input:   "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0", // 63 chars
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSubGroupName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Fatalf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateSubGroups(t *testing.T) {
	tests := []struct {
		name      string
		subGroups []SubGroup
		wantErr   error
	}{
		{
			name: "Valid DAG single root",
			subGroups: []SubGroup{
				{Name: "a", MinMember: 1},
				{Name: "b", Parent: ptr.To("a"), MinMember: 1},
				{Name: "c", Parent: ptr.To("b"), MinMember: 1},
			},
			wantErr: nil,
		},
		{
			name: "Valid DAG multiple roots",
			subGroups: []SubGroup{
				{Name: "a", MinMember: 1},
				{Name: "b", MinMember: 1},
				{Name: "c", Parent: ptr.To("a"), MinMember: 1},
				{Name: "d", Parent: ptr.To("b"), MinMember: 1},
			},
			wantErr: nil,
		},
		{
			name: "Valid lowercase with hyphens and numbers",
			subGroups: []SubGroup{
				{Name: "group-1", MinMember: 1},
				{Name: "sub-group-2", Parent: ptr.To("group-1"), MinMember: 1},
			},
			wantErr: nil,
		},
		{
			name: "Missing parent",
			subGroups: []SubGroup{
				{Name: "a", MinMember: 1},
				{Name: "b", Parent: ptr.To("x"), MinMember: 1}, // parent x does not exist
			},
			wantErr: errors.New("parent x of b was not found"),
		},
		{
			name:      "Empty list",
			subGroups: []SubGroup{},
			wantErr:   nil,
		},
		{
			name: "Duplicate subgroup names",
			subGroups: []SubGroup{
				{Name: "a", MinMember: 1},
				{Name: "a", MinMember: 1}, // duplicate
			},
			wantErr: errors.New("duplicate subgroup name a"),
		},
		{
			name: "Cycle in graph (A -> B -> C -> A) - duplicate subgroup name",
			subGroups: []SubGroup{
				{Name: "a", MinMember: 1},
				{Name: "b", Parent: ptr.To("a"), MinMember: 1},
				{Name: "c", Parent: ptr.To("b"), MinMember: 1},
				{Name: "a", Parent: ptr.To("c"), MinMember: 1}, // creates a cycle
			},
			wantErr: errors.New("duplicate subgroup name a"), // duplicate is caught before cycle
		},
		{
			name: "Self-parent subgroup (cycle of length 1)",
			subGroups: []SubGroup{
				{Name: "a", Parent: ptr.To("a"), MinMember: 1},
			},
			wantErr: errors.New("cycle detected in subgroups"),
		},
		{
			name: "Cycle in graph (A -> B -> C -> A)",
			subGroups: []SubGroup{
				{Name: "a", Parent: ptr.To("c"), MinMember: 1},
				{Name: "b", Parent: ptr.To("a"), MinMember: 1},
				{Name: "c", Parent: ptr.To("b"), MinMember: 1}, // creates a cycle
			},
			wantErr: errors.New("cycle detected in subgroups"),
		},
		{
			name: "Multiple disjoint cycles",
			subGroups: []SubGroup{
				{Name: "a", Parent: ptr.To("b"), MinMember: 1},
				{Name: "b", Parent: ptr.To("a"), MinMember: 1}, // cycle a <-> b
				{Name: "c", Parent: ptr.To("d"), MinMember: 1},
				{Name: "d", Parent: ptr.To("c"), MinMember: 1}, // cycle c <-> d
			},
			wantErr: errors.New("cycle detected in subgroups"),
		},
		// New tests for subgroup name validation
		{
			name: "Invalid uppercase subgroup name",
			subGroups: []SubGroup{
				{Name: "MySubGroup", MinMember: 1},
			},
			wantErr: errors.New(`subgroup name "MySubGroup" is invalid: must consist of lowercase alphanumeric characters or '-', and must start and end with an alphanumeric character`),
		},
		{
			name: "Invalid uppercase parent reference",
			subGroups: []SubGroup{
				{Name: "child", Parent: ptr.To("ParentGroup"), MinMember: 1},
			},
			wantErr: errors.New(`invalid parent reference: subgroup name "ParentGroup" is invalid: must consist of lowercase alphanumeric characters or '-', and must start and end with an alphanumeric character`),
		},
		{
			name: "Invalid subgroup name with underscore",
			subGroups: []SubGroup{
				{Name: "my_subgroup", MinMember: 1},
			},
			wantErr: errors.New(`subgroup name "my_subgroup" is invalid: must consist of lowercase alphanumeric characters or '-', and must start and end with an alphanumeric character`),
		},
		{
			name: "Invalid subgroup name starts with hyphen",
			subGroups: []SubGroup{
				{Name: "-invalid", MinMember: 1},
			},
			wantErr: errors.New(`subgroup name "-invalid" is invalid: must consist of lowercase alphanumeric characters or '-', and must start and end with an alphanumeric character`),
		},
		{
			name: "Invalid subgroup name ends with hyphen",
			subGroups: []SubGroup{
				{Name: "invalid-", MinMember: 1},
			},
			wantErr: errors.New(`subgroup name "invalid-" is invalid: must consist of lowercase alphanumeric characters or '-', and must start and end with an alphanumeric character`),
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

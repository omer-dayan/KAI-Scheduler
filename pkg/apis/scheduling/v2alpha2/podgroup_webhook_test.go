// Copyright 2025 NVIDIA CORPORATION
// SPDX-License-Identifier: Apache-2.0

package v2alpha2

import (
	"errors"
	"strings"
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
		{
			name:    "valid lowercase",
			input:   "workers",
			wantErr: false,
		},
		{
			name:    "valid with hyphen",
			input:   "decode-workers",
			wantErr: false,
		},
		{
			name:    "valid with numbers",
			input:   "worker1",
			wantErr: false,
		},
		{
			name:    "valid single char",
			input:   "a",
			wantErr: false,
		},
		{
			name:    "valid number only",
			input:   "1",
			wantErr: false,
		},
		{
			name:    "valid starts with number",
			input:   "1worker",
			wantErr: false,
		},
		{
			name:    "valid complex",
			input:   "decode-workers-v2",
			wantErr: false,
		},
		{
			name:    "invalid uppercase",
			input:   "Workers",
			wantErr: true,
			errMsg:  "must consist of lowercase alphanumeric characters",
		},
		{
			name:    "invalid mixed case",
			input:   "decodeWorkers",
			wantErr: true,
			errMsg:  "must consist of lowercase alphanumeric characters",
		},
		{
			name:    "invalid underscore",
			input:   "decode_workers",
			wantErr: true,
			errMsg:  "must consist of lowercase alphanumeric characters",
		},
		{
			name:    "invalid starts with hyphen",
			input:   "-workers",
			wantErr: true,
			errMsg:  "must start and end with an alphanumeric character",
		},
		{
			name:    "invalid ends with hyphen",
			input:   "workers-",
			wantErr: true,
			errMsg:  "must start and end with an alphanumeric character",
		},
		{
			name:    "invalid empty",
			input:   "",
			wantErr: true,
			errMsg:  "name cannot be empty",
		},
		{
			name:    "invalid too long",
			input:   strings.Repeat("a", 64),
			wantErr: true,
			errMsg:  "name must be no more than 63 characters",
		},
		{
			name:    "valid max length",
			input:   strings.Repeat("a", 63),
			wantErr: false,
		},
		{
			name:    "invalid dot",
			input:   "decode.workers",
			wantErr: true,
			errMsg:  "must consist of lowercase alphanumeric characters",
		},
		{
			name:    "invalid space",
			input:   "decode workers",
			wantErr: true,
			errMsg:  "must consist of lowercase alphanumeric characters",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSubGroupName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSubGroupName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" && err != nil && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("validateSubGroupName(%q) error = %v, want error containing %q", tt.input, err, tt.errMsg)
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
			name: "Valid lowercase with hyphens",
			subGroups: []SubGroup{
				{Name: "decode-workers", MinMember: 4},
				{Name: "prefill-workers", MinMember: 2, Parent: ptr.To("decode-workers")},
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
			name: "Cycle in graph (a -> b -> c -> a) - duplicate subgroup name",
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
			name: "Cycle in graph (a -> b -> c -> a)",
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
		// Lowercase validation test cases
		{
			name: "Invalid uppercase subgroup name",
			subGroups: []SubGroup{
				{Name: "Workers", MinMember: 4},
			},
			wantErr: errors.New("invalid subgroup name \"Workers\": must consist of lowercase alphanumeric characters or '-', must start and end with an alphanumeric character"),
		},
		{
			name: "Invalid mixed case subgroup name",
			subGroups: []SubGroup{
				{Name: "decodeWorkers", MinMember: 4},
			},
			wantErr: errors.New("invalid subgroup name \"decodeWorkers\": must consist of lowercase alphanumeric characters or '-', must start and end with an alphanumeric character"),
		},
		{
			name: "Invalid uppercase parent reference",
			subGroups: []SubGroup{
				{Name: "workers", MinMember: 4},
				{Name: "leaders", MinMember: 1, Parent: ptr.To("Workers")},
			},
			wantErr: errors.New("invalid parent name \"Workers\" for subgroup \"leaders\": must consist of lowercase alphanumeric characters or '-', must start and end with an alphanumeric character"),
		},
		{
			name: "Invalid underscore in name",
			subGroups: []SubGroup{
				{Name: "decode_workers", MinMember: 4},
			},
			wantErr: errors.New("invalid subgroup name \"decode_workers\": must consist of lowercase alphanumeric characters or '-', must start and end with an alphanumeric character"),
		},
		{
			name: "Invalid name starts with hyphen",
			subGroups: []SubGroup{
				{Name: "-workers", MinMember: 4},
			},
			wantErr: errors.New("invalid subgroup name \"-workers\": must consist of lowercase alphanumeric characters or '-', must start and end with an alphanumeric character"),
		},
		{
			name: "Invalid name ends with hyphen",
			subGroups: []SubGroup{
				{Name: "workers-", MinMember: 4},
			},
			wantErr: errors.New("invalid subgroup name \"workers-\": must consist of lowercase alphanumeric characters or '-', must start and end with an alphanumeric character"),
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

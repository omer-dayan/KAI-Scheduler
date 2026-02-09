// Copyright 2025 NVIDIA CORPORATION
// SPDX-License-Identifier: Apache-2.0

package v2alpha2

import (
	"errors"
	"testing"

	"k8s.io/utils/ptr"
)

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
			name: "Valid lowercase name with hyphens",
			subGroups: []SubGroup{
				{Name: "my-subgroup-1", MinMember: 1},
			},
			wantErr: nil,
		},
		{
			name: "Valid lowercase name with numbers",
			subGroups: []SubGroup{
				{Name: "sub1group2", MinMember: 1},
			},
			wantErr: nil,
		},
		{
			name: "Valid single character name",
			subGroups: []SubGroup{
				{Name: "a", MinMember: 1},
			},
			wantErr: nil,
		},
		{
			name: "Valid single digit name",
			subGroups: []SubGroup{
				{Name: "1", MinMember: 1},
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
		// Subgroup name validation tests
		{
			name: "Invalid uppercase subgroup name",
			subGroups: []SubGroup{
				{Name: "MySubgroup", MinMember: 1},
			},
			wantErr: errors.New(`subgroup name "MySubgroup" must be a lowercase DNS-1123 label: lowercase alphanumeric characters or '-', starting and ending with an alphanumeric character`),
		},
		{
			name: "Invalid mixed case subgroup name",
			subGroups: []SubGroup{
				{Name: "mySubGroup", MinMember: 1},
			},
			wantErr: errors.New(`subgroup name "mySubGroup" must be a lowercase DNS-1123 label: lowercase alphanumeric characters or '-', starting and ending with an alphanumeric character`),
		},
		{
			name: "Invalid subgroup name with underscore",
			subGroups: []SubGroup{
				{Name: "my_subgroup", MinMember: 1},
			},
			wantErr: errors.New(`subgroup name "my_subgroup" must be a lowercase DNS-1123 label: lowercase alphanumeric characters or '-', starting and ending with an alphanumeric character`),
		},
		{
			name: "Invalid subgroup name starting with hyphen",
			subGroups: []SubGroup{
				{Name: "-subgroup", MinMember: 1},
			},
			wantErr: errors.New(`subgroup name "-subgroup" must be a lowercase DNS-1123 label: lowercase alphanumeric characters or '-', starting and ending with an alphanumeric character`),
		},
		{
			name: "Invalid subgroup name ending with hyphen",
			subGroups: []SubGroup{
				{Name: "subgroup-", MinMember: 1},
			},
			wantErr: errors.New(`subgroup name "subgroup-" must be a lowercase DNS-1123 label: lowercase alphanumeric characters or '-', starting and ending with an alphanumeric character`),
		},
		{
			name: "Invalid subgroup name with special characters",
			subGroups: []SubGroup{
				{Name: "sub@group", MinMember: 1},
			},
			wantErr: errors.New(`subgroup name "sub@group" must be a lowercase DNS-1123 label: lowercase alphanumeric characters or '-', starting and ending with an alphanumeric character`),
		},
		{
			name: "Invalid empty subgroup name",
			subGroups: []SubGroup{
				{Name: "", MinMember: 1},
			},
			wantErr: errors.New("subgroup name cannot be empty"),
		},
		{
			name: "Invalid uppercase parent reference",
			subGroups: []SubGroup{
				{Name: "parent", MinMember: 1},
				{Name: "child", Parent: ptr.To("Parent"), MinMember: 1},
			},
			wantErr: errors.New(`invalid parent reference: subgroup name "Parent" must be a lowercase DNS-1123 label: lowercase alphanumeric characters or '-', starting and ending with an alphanumeric character`),
		},
		{
			name: "Invalid parent reference with underscore",
			subGroups: []SubGroup{
				{Name: "parent-group", MinMember: 1},
				{Name: "child", Parent: ptr.To("parent_group"), MinMember: 1},
			},
			wantErr: errors.New(`invalid parent reference: subgroup name "parent_group" must be a lowercase DNS-1123 label: lowercase alphanumeric characters or '-', starting and ending with an alphanumeric character`),
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

func TestValidateSubGroupName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid lowercase", "mysubgroup", false},
		{"valid with hyphens", "my-sub-group", false},
		{"valid with numbers", "sub1group2", false},
		{"valid single char", "a", false},
		{"valid single digit", "1", false},
		{"valid alphanumeric mix", "a1b2c3", false},
		{"invalid uppercase", "MySubGroup", true},
		{"invalid mixed case", "mySubgroup", true},
		{"invalid underscore", "my_subgroup", true},
		{"invalid starting hyphen", "-subgroup", true},
		{"invalid ending hyphen", "subgroup-", true},
		{"invalid special char", "sub@group", true},
		{"invalid space", "sub group", true},
		{"invalid empty", "", true},
		{"invalid dot", "sub.group", true},
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

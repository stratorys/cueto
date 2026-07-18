// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"reflect"
	"testing"
)

// normalizeFlags permits the documented `command positional -C dir` form even
// though Go's flag package otherwise stops parsing at the first positional arg.
func TestNormalizeFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "no flags",
			args: []string{"catalog"},
			want: []string{"catalog"},
		},
		{
			name: "flag before positional",
			args: []string{"-C", "../cue", "vet"},
			want: []string{"-C", "../cue", "vet"},
		},
		{
			name: "flag after positional",
			args: []string{"describe", "-C", "../cue"},
			want: []string{"-C", "../cue", "describe"},
		},
		{
			name: "--input with stdin marker",
			args: []string{"eval", "myeval", "--input", "-"},
			want: []string{"--input", "-", "eval", "myeval"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := normalizeFlags(c.args)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("normalizeFlags(%v) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}

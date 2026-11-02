// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file tests validation of zone segment layouts.

package main

import (
	"testing"

	openrgb "github.com/sb0-0dev/openrgb-golang/client"
)

// TestValidateSegmentLayout tests the validation of zone segment layouts.
func TestValidateSegmentLayout(t *testing.T) {
	tests := []struct {
		name     string
		segments []openrgb.Segment
		wantErr  bool
	}{
		{
			name: "complete segments in any order",
			segments: []openrgb.Segment{
				{Name: "right", StartIdx: 4, NumLeds: 4},
				{Name: "left", StartIdx: 0, NumLeds: 4},
			},
		},
		{
			name: "overlap",
			segments: []openrgb.Segment{
				{Name: "first", StartIdx: 0, NumLeds: 5},
				{Name: "second", StartIdx: 4, NumLeds: 4},
			},
			wantErr: true,
		},
		{
			name: "gap",
			segments: []openrgb.Segment{
				{Name: "first", StartIdx: 0, NumLeds: 3},
				{Name: "second", StartIdx: 4, NumLeds: 4},
			},
			wantErr: true,
		},
		{
			name: "incomplete coverage",
			segments: []openrgb.Segment{
				{Name: "first", StartIdx: 0, NumLeds: 7},
			},
			wantErr: true,
		},
		{
			name: "blank name",
			segments: []openrgb.Segment{
				{Name: "  ", StartIdx: 0, NumLeds: 8},
			},
			wantErr: true,
		},
		{
			name: "out of bounds",
			segments: []openrgb.Segment{
				{Name: "first", StartIdx: 0, NumLeds: 9},
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSegmentLayout(test.segments, 8)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateSegmentLayout() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

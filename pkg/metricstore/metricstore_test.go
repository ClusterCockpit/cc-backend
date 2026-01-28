// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

func TestAssignAggregationStrategy(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected AggregationStrategy
		wantErr  bool
	}{
		{"empty string", "", NoAggregation, false},
		{"sum", "sum", SumAggregation, false},
		{"avg", "avg", AvgAggregation, false},
		{"invalid", "invalid", NoAggregation, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AssignAggregationStrategy(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("AssignAggregationStrategy(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("AssignAggregationStrategy(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBufferWrite(t *testing.T) {
	b := newBuffer(100, 10)

	// Test writing value
	nb, err := b.write(100, schema.Float(42.0))
	if err != nil {
		t.Errorf("buffer.write() error = %v", err)
	}
	if nb != b {
		t.Error("buffer.write() created new buffer unexpectedly")
	}
	if len(b.data) != 1 {
		t.Errorf("buffer.write() len(data) = %d, want 1", len(b.data))
	}
	if b.data[0] != schema.Float(42.0) {
		t.Errorf("buffer.write() data[0] = %v, want 42.0", b.data[0])
	}

	// Test writing value from past (should error)
	_, err = b.write(50, schema.Float(10.0))
	if err == nil {
		t.Error("buffer.write() expected error for past timestamp")
	}
}

func TestBufferRead(t *testing.T) {
	b := newBuffer(100, 10)

	// Write some test data
	b.write(100, schema.Float(1.0))
	b.write(110, schema.Float(2.0))
	b.write(120, schema.Float(3.0))

	// Read data
	data := make([]schema.Float, 3)
	result, from, to, err := b.read(100, 130, data)
	if err != nil {
		t.Errorf("buffer.read() error = %v", err)
	}
	// Buffer read should return from as firstWrite (start + freq/2)
	if from != 100 {
		t.Errorf("buffer.read() from = %d, want 100", from)
	}
	if to != 130 {
		t.Errorf("buffer.read() to = %d, want 130", to)
	}
	if len(result) != 3 {
		t.Errorf("buffer.read() len(result) = %d, want 3", len(result))
	}
}

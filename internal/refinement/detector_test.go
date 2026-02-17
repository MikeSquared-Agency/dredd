package refinement

import (
	"math"
	"testing"
)

func TestParsePgVector(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []float64
		hasError bool
	}{
		{
			name:     "valid vector",
			input:    "[0.1,0.2,0.3]",
			expected: []float64{0.1, 0.2, 0.3},
			hasError: false,
		},
		{
			name:     "empty vector",
			input:    "[]",
			expected: []float64{},
			hasError: false,
		},
		{
			name:     "single element",
			input:    "[1.0]",
			expected: []float64{1.0},
			hasError: false,
		},
		{
			name:     "negative values",
			input:    "[-0.1,0.2,-0.3]",
			expected: []float64{-0.1, 0.2, -0.3},
			hasError: false,
		},
		{
			name:     "spaces around values",
			input:    "[ 0.1 , 0.2 , 0.3 ]",
			expected: []float64{0.1, 0.2, 0.3},
			hasError: false,
		},
		{
			name:     "invalid format - no brackets",
			input:    "0.1,0.2,0.3",
			expected: nil,
			hasError: true,
		},
		{
			name:     "invalid format - missing closing bracket",
			input:    "[0.1,0.2,0.3",
			expected: nil,
			hasError: true,
		},
		{
			name:     "invalid float value",
			input:    "[0.1,invalid,0.3]",
			expected: nil,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePgVector(tt.input)
			
			if tt.hasError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			
			if len(result) != len(tt.expected) {
				t.Errorf("length mismatch: got %d, want %d", len(result), len(tt.expected))
				return
			}
			
			for i, val := range result {
				if math.Abs(val-tt.expected[i]) > 1e-9 {
					t.Errorf("value mismatch at index %d: got %f, want %f", i, val, tt.expected[i])
				}
			}
		})
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float64
		b        []float64
		expected float64
	}{
		{
			name:     "identical vectors",
			a:        []float64{1.0, 2.0, 3.0},
			b:        []float64{1.0, 2.0, 3.0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float64{1.0, 0.0},
			b:        []float64{0.0, 1.0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float64{1.0, 2.0},
			b:        []float64{-1.0, -2.0},
			expected: -1.0,
		},
		{
			name:     "different lengths",
			a:        []float64{1.0, 2.0},
			b:        []float64{1.0, 2.0, 3.0},
			expected: 0.0,
		},
		{
			name:     "zero vector",
			a:        []float64{0.0, 0.0},
			b:        []float64{1.0, 2.0},
			expected: 0.0,
		},
		{
			name:     "empty vectors",
			a:        []float64{},
			b:        []float64{},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			if math.Abs(result-tt.expected) > 1e-9 {
				t.Errorf("cosineSimilarity(%v, %v) = %f, want %f", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

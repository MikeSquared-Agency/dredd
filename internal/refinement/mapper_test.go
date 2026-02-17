package refinement

import (
	"reflect"
	"testing"
)

func TestMapper_MapPatternToSections(t *testing.T) {
	mapper := NewMapper()
	
	tests := []struct {
		name        string
		patternType string
		expected    []string
	}{
		{
			name:        "correction pattern",
			patternType: "correction",
			expected:    []string{"thinking_mode", "anti_patterns"},
		},
		{
			name:        "pushback pattern", 
			patternType: "pushback",
			expected:    []string{"anti_patterns"},
		},
		{
			name:        "philosophy pattern",
			patternType: "philosophy", 
			expected:    []string{"philosophy"},
		},
		{
			name:        "reframing pattern",
			patternType: "reframing",
			expected:    []string{"thinking_mode"},
		},
		{
			name:        "direction pattern",
			patternType: "direction",
			expected:    []string{"interaction_modes"},
		},
		{
			name:        "unknown pattern",
			patternType: "unknown",
			expected:    []string{},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapper.MapPatternToSections(tt.patternType)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("MapPatternToSections(%q) = %v, want %v", tt.patternType, result, tt.expected)
			}
		})
	}
}

func TestMapper_MapPatternToSections_ImmutableReturn(t *testing.T) {
	mapper := NewMapper()
	
	// Get sections for correction pattern
	sections1 := mapper.MapPatternToSections("correction")
	sections2 := mapper.MapPatternToSections("correction")
	
	// Modify first result
	sections1[0] = "modified"
	
	// Verify second result is not affected
	expected := []string{"thinking_mode", "anti_patterns"}
	if !reflect.DeepEqual(sections2, expected) {
		t.Errorf("MapPatternToSections should return immutable copies, got %v", sections2)
	}
}

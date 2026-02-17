package refinement

// Mapper maps pattern types to SOUL sections for refinement targeting
type Mapper struct {
	mapping map[string][]string
}

// NewMapper creates a new SOUL section mapper
func NewMapper() *Mapper {
	return &Mapper{
		mapping: map[string][]string{
			"correction": {"thinking_mode", "anti_patterns"},
			"pushback":   {"anti_patterns"},
			"philosophy": {"philosophy"},
			"reframing":  {"thinking_mode"},
			"direction":  {"interaction_modes"},
		},
	}
}

// MapPatternToSections returns the SOUL sections that should be refined for a given pattern type
func (m *Mapper) MapPatternToSections(patternType string) []string {
	sections, exists := m.mapping[patternType]
	if !exists {
		return []string{}
	}
	// Return a copy to avoid external modification
	result := make([]string, len(sections))
	copy(result, sections)
	return result
}

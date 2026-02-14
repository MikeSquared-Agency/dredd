package trust

import (
	"math"
	"testing"
)

func TestSignalWeight(t *testing.T) {
	tests := []struct {
		name     string
		severity string
		want     float64
	}{
		{"routine", "routine", 0.01},
		{"significant", "significant", 0.03},
		{"critical", "critical", 0.05},
		{"unknown defaults to routine", "banana", 0.01},
		{"empty defaults to routine", "", 0.01},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SignalWeight(tt.severity)
			if got != tt.want {
				t.Errorf("SignalWeight(%q) = %f, want %f", tt.severity, got, tt.want)
			}
		})
	}
}

func TestUpdateScore_Correct(t *testing.T) {
	tests := []struct {
		name     string
		current  float64
		severity string
		want     float64
	}{
		{"routine correct from zero", 0.0, "routine", 0.01},
		{"significant correct from zero", 0.0, "significant", 0.03},
		{"critical correct from zero", 0.0, "critical", 0.05},
		{"routine correct from 0.5", 0.5, "routine", 0.51},
		{"clamped at 1.0", 0.99, "significant", 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UpdateScore(tt.current, tt.severity, true)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("UpdateScore(%f, %q, true) = %f, want %f", tt.current, tt.severity, got, tt.want)
			}
		})
	}
}

func TestUpdateScore_Wrong(t *testing.T) {
	tests := []struct {
		name     string
		current  float64
		severity string
		want     float64
	}{
		{"routine wrong — 2x degradation", 0.5, "routine", 0.48},
		{"significant wrong — 2x degradation", 0.5, "significant", 0.44},
		{"critical wrong — 2x degradation", 0.5, "critical", 0.40},
		{"clamped at 0.0", 0.01, "significant", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UpdateScore(tt.current, tt.severity, false)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("UpdateScore(%f, %q, false) = %f, want %f", tt.current, tt.severity, got, tt.want)
			}
		})
	}
}

func TestUpdateScore_Asymmetry(t *testing.T) {
	// Core property: wrong decisions degrade trust 2x faster than correct ones build it
	score := 0.5
	gain := UpdateScore(score, "routine", true) - score
	loss := score - UpdateScore(score, "routine", false)

	if math.Abs(loss-gain*2) > 0.001 {
		t.Errorf("expected loss (%f) to be 2x gain (%f)", loss, gain)
	}
}

func TestCriticalFailureDrop(t *testing.T) {
	tests := []struct {
		name    string
		current float64
		want    float64
	}{
		{"drop from 0.8", 0.8, 0.5},
		{"drop from 0.5", 0.5, 0.2},
		{"clamped at 0.0 from 0.2", 0.2, 0.0},
		{"clamped at 0.0 from 0.1", 0.1, 0.0},
		{"already zero", 0.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CriticalFailureDrop(tt.current)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("CriticalFailureDrop(%f) = %f, want %f", tt.current, got, tt.want)
			}
		})
	}
}

func TestDecayScore(t *testing.T) {
	tests := []struct {
		name      string
		current   float64
		decayRate float64
		days      int
		want      float64
	}{
		{"no decay at 0 days", 0.8, 0.01, 0, 0.8},
		{"1 day decay", 1.0, 0.01, 1, 0.99},
		{"7 days decay", 1.0, 0.01, 7, 0.9321}, // 1.0 * (0.99)^7
		{"30 days decay", 1.0, 0.01, 30, 0.7397},
		{"zero score stays zero", 0.0, 0.01, 30, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecayScore(tt.current, tt.decayRate, tt.days)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("DecayScore(%f, %f, %d) = %f, want %f", tt.current, tt.decayRate, tt.days, got, tt.want)
			}
		})
	}
}

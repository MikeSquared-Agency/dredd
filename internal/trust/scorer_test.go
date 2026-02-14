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

func TestSentimentModifier(t *testing.T) {
	tests := []struct {
		name      string
		sentiment string
		want      float64
	}{
		{"flow", "flow", 1.0},
		{"stressed", "stressed", 0.7},
		{"frustrated", "frustrated", 0.5},
		{"empty defaults to 1.0", "", 1.0},
		{"unknown defaults to 1.0", "happy", 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SentimentModifier(tt.sentiment)
			if got != tt.want {
				t.Errorf("SentimentModifier(%q) = %f, want %f", tt.sentiment, got, tt.want)
			}
		})
	}
}

func TestUpdateScoreWithSentiment_Flow(t *testing.T) {
	// Flow sentiment (1.0) should behave identically to UpdateScore.
	tests := []struct {
		name     string
		current  float64
		severity string
		correct  bool
		want     float64
	}{
		{"flow correct routine", 0.5, "routine", true, 0.51},
		{"flow wrong routine", 0.5, "routine", false, 0.48},
		{"flow correct critical", 0.0, "critical", true, 0.05},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UpdateScoreWithSentiment(tt.current, tt.severity, tt.correct, "flow")
			base := UpdateScore(tt.current, tt.severity, tt.correct)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("UpdateScoreWithSentiment(%f, %q, %v, \"flow\") = %f, want %f", tt.current, tt.severity, tt.correct, got, tt.want)
			}
			if math.Abs(got-base) > 0.001 {
				t.Errorf("flow sentiment should match UpdateScore: got %f, base %f", got, base)
			}
		})
	}
}

func TestUpdateScoreWithSentiment_Stressed(t *testing.T) {
	// Stressed sentiment (0.7) scales the weight down.
	tests := []struct {
		name     string
		current  float64
		severity string
		correct  bool
		want     float64
	}{
		// routine weight=0.01, stressed=0.7: effective=0.007
		{"stressed correct routine", 0.5, "routine", true, 0.507},
		// routine weight=0.01, stressed=0.7: effective=0.007, 2x degradation=0.014
		{"stressed wrong routine", 0.5, "routine", false, 0.486},
		// significant weight=0.03, stressed=0.7: effective=0.021
		{"stressed correct significant", 0.5, "significant", true, 0.521},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UpdateScoreWithSentiment(tt.current, tt.severity, tt.correct, "stressed")
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("UpdateScoreWithSentiment(%f, %q, %v, \"stressed\") = %f, want %f", tt.current, tt.severity, tt.correct, got, tt.want)
			}
		})
	}
}

func TestUpdateScoreWithSentiment_Frustrated(t *testing.T) {
	// Frustrated sentiment (0.5) halves the weight.
	tests := []struct {
		name     string
		current  float64
		severity string
		correct  bool
		want     float64
	}{
		// routine weight=0.01, frustrated=0.5: effective=0.005
		{"frustrated correct routine", 0.5, "routine", true, 0.505},
		// routine weight=0.01, frustrated=0.5: effective=0.005, 2x degradation=0.01
		{"frustrated wrong routine", 0.5, "routine", false, 0.49},
		// critical weight=0.05, frustrated=0.5: effective=0.025
		{"frustrated correct critical", 0.0, "critical", true, 0.025},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UpdateScoreWithSentiment(tt.current, tt.severity, tt.correct, "frustrated")
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("UpdateScoreWithSentiment(%f, %q, %v, \"frustrated\") = %f, want %f", tt.current, tt.severity, tt.correct, got, tt.want)
			}
		})
	}
}

func TestUpdateScoreWithSentiment_EmptyDefaultsToFlow(t *testing.T) {
	// Empty sentiment should behave the same as "flow" (modifier=1.0).
	got := UpdateScoreWithSentiment(0.5, "routine", true, "")
	base := UpdateScore(0.5, "routine", true)
	if math.Abs(got-base) > 0.001 {
		t.Errorf("empty sentiment should match UpdateScore: got %f, base %f", got, base)
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

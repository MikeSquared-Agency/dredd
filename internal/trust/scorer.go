package trust

// SignalWeight returns the trust score increment for a given severity.
func SignalWeight(severity string) float64 {
	switch severity {
	case "routine":
		return 0.01
	case "significant":
		return 0.03
	case "critical":
		return 0.05
	default:
		return 0.01
	}
}

// SentimentModifier returns the scaling factor for a given sentiment state.
// Spec: flow=1.0, stressed=0.7, frustrated=0.5, unknown/default=1.0.
func SentimentModifier(sentiment string) float64 {
	switch sentiment {
	case "flow":
		return 1.0
	case "stressed":
		return 0.7
	case "frustrated":
		return 0.5
	default:
		return 1.0
	}
}

// UpdateScore calculates the new trust score after a signal.
//
// direction: +1 for correct, -1 for wrong
// Degradation is asymmetric: wrong decisions count 2x.
func UpdateScore(currentScore float64, severity string, correct bool) float64 {
	return UpdateScoreWithSentiment(currentScore, severity, correct, "")
}

// UpdateScoreWithSentiment calculates the new trust score after a signal,
// scaling the weight by the sentiment modifier.
//
// Formula: new_score = old_score + (signal_weight x sentiment_modifier x direction)
// Degradation is asymmetric: wrong decisions count 2x.
func UpdateScoreWithSentiment(currentScore float64, severity string, correct bool, sentiment string) float64 {
	weight := SignalWeight(severity) * SentimentModifier(sentiment)

	if correct {
		return clamp(currentScore + weight)
	}
	// Wrong decisions degrade trust 2x faster
	return clamp(currentScore - weight*2.0)
}

// CriticalFailureDrop applies a cliff drop for critical failures.
func CriticalFailureDrop(currentScore float64) float64 {
	score := currentScore - 0.3
	if score < 0.0 {
		return 0.0
	}
	return score
}

// DecayScore applies daily decay for stale trust scores.
// decayRate is typically 0.01, days is the number of days since last signal.
func DecayScore(currentScore float64, decayRate float64, days int) float64 {
	score := currentScore
	for i := 0; i < days; i++ {
		score *= (1.0 - decayRate)
	}
	return clamp(score)
}

func clamp(score float64) float64 {
	if score < 0.0 {
		return 0.0
	}
	if score > 1.0 {
		return 1.0
	}
	return score
}

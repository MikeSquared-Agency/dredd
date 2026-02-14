package backfill

import (
	"time"
)

// dedupWindow is the tolerance for matching timestamps across CC/Gateway files.
const dedupWindow = 1 * time.Second

// overlapThreshold is the fraction of timestamps that must match to consider files duplicates.
const overlapThreshold = 0.8

// fileFingerprint holds timing + content info for deduplication.
type fileFingerprint struct {
	Path       string
	Source     FileSource
	Timestamps []time.Time
	Previews   []string // first 3 message texts (trimmed)
}

// BuildFingerprint creates a fingerprint from parsed conversation messages.
func BuildFingerprint(path string, source FileSource, msgs []ConversationMessage) fileFingerprint {
	fp := fileFingerprint{
		Path:   path,
		Source: source,
	}

	for _, m := range msgs {
		if !m.Timestamp.IsZero() {
			fp.Timestamps = append(fp.Timestamps, m.Timestamp)
		}
	}

	// Keep first 3 message texts for preview.
	for i, m := range msgs {
		if i >= 3 {
			break
		}
		text := m.Text
		if len(text) > 100 {
			text = text[:100]
		}
		fp.Previews = append(fp.Previews, text)
	}

	return fp
}

// FindDuplicates takes CC and Gateway fingerprints and returns gateway file paths
// that overlap with CC files (CC is preferred source).
func FindDuplicates(ccFPs, gwFPs []fileFingerprint) map[string]bool {
	duplicates := make(map[string]bool)

	for _, gw := range gwFPs {
		if len(gw.Timestamps) == 0 {
			continue
		}
		for _, cc := range ccFPs {
			if isOverlapping(cc, gw) {
				duplicates[gw.Path] = true
				break
			}
		}
	}

	return duplicates
}

// isOverlapping checks if >80% of one file's timestamps appear in the other
// within the dedupWindow.
func isOverlapping(a, b fileFingerprint) bool {
	if len(b.Timestamps) == 0 {
		return false
	}

	matches := 0
	for _, bt := range b.Timestamps {
		for _, at := range a.Timestamps {
			diff := bt.Sub(at)
			if diff < 0 {
				diff = -diff
			}
			if diff <= dedupWindow {
				matches++
				break
			}
		}
	}

	return float64(matches)/float64(len(b.Timestamps)) >= overlapThreshold
}

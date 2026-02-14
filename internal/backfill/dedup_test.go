package backfill

import (
	"testing"
	"time"
)

func TestFindDuplicates_OverlappingSessions(t *testing.T) {
	base := time.Date(2026, 2, 11, 10, 0, 0, 0, time.UTC)

	ccFP := fileFingerprint{
		Path:   "cc/session1.jsonl",
		Source: SourceCC,
		Timestamps: []time.Time{
			base,
			base.Add(1 * time.Second),
			base.Add(2 * time.Second),
			base.Add(3 * time.Second),
			base.Add(4 * time.Second),
		},
	}

	// Gateway has same timestamps (within window) — should be detected as duplicate.
	gwFP := fileFingerprint{
		Path:   "gw/session1.jsonl",
		Source: SourceGateway,
		Timestamps: []time.Time{
			base,
			base.Add(1 * time.Second),
			base.Add(2 * time.Second),
			base.Add(3 * time.Second),
			base.Add(4 * time.Second),
		},
	}

	dups := FindDuplicates([]fileFingerprint{ccFP}, []fileFingerprint{gwFP})
	if !dups["gw/session1.jsonl"] {
		t.Error("expected gateway file to be marked as duplicate")
	}
}

func TestFindDuplicates_NoOverlap(t *testing.T) {
	base := time.Date(2026, 2, 11, 10, 0, 0, 0, time.UTC)

	ccFP := fileFingerprint{
		Path:   "cc/session1.jsonl",
		Source: SourceCC,
		Timestamps: []time.Time{
			base,
			base.Add(1 * time.Second),
		},
	}

	// Gateway timestamps are completely different.
	gwFP := fileFingerprint{
		Path:   "gw/session2.jsonl",
		Source: SourceGateway,
		Timestamps: []time.Time{
			base.Add(1 * time.Hour),
			base.Add(1*time.Hour + time.Second),
		},
	}

	dups := FindDuplicates([]fileFingerprint{ccFP}, []fileFingerprint{gwFP})
	if dups["gw/session2.jsonl"] {
		t.Error("expected gateway file NOT to be marked as duplicate")
	}
}

func TestFindDuplicates_PartialOverlapBelowThreshold(t *testing.T) {
	base := time.Date(2026, 2, 11, 10, 0, 0, 0, time.UTC)

	ccFP := fileFingerprint{
		Path:   "cc/session1.jsonl",
		Source: SourceCC,
		Timestamps: []time.Time{
			base,
			base.Add(1 * time.Second),
		},
	}

	// Only 2 out of 5 match — 40% < 80% threshold.
	gwFP := fileFingerprint{
		Path:   "gw/session3.jsonl",
		Source: SourceGateway,
		Timestamps: []time.Time{
			base,
			base.Add(1 * time.Second),
			base.Add(1 * time.Hour),
			base.Add(2 * time.Hour),
			base.Add(3 * time.Hour),
		},
	}

	dups := FindDuplicates([]fileFingerprint{ccFP}, []fileFingerprint{gwFP})
	if dups["gw/session3.jsonl"] {
		t.Error("40% overlap should NOT be marked as duplicate")
	}
}

func TestFindDuplicates_EmptyGateway(t *testing.T) {
	ccFP := fileFingerprint{
		Path:       "cc/session1.jsonl",
		Source:     SourceCC,
		Timestamps: []time.Time{time.Now()},
	}

	dups := FindDuplicates([]fileFingerprint{ccFP}, nil)
	if len(dups) != 0 {
		t.Error("expected no duplicates with empty gateway list")
	}
}

func TestFindDuplicates_WithinTimestampWindow(t *testing.T) {
	base := time.Date(2026, 2, 11, 10, 0, 0, 0, time.UTC)

	ccFP := fileFingerprint{
		Path:   "cc/session1.jsonl",
		Source: SourceCC,
		Timestamps: []time.Time{
			base,
			base.Add(10 * time.Second),
		},
	}

	// Timestamps within 1s window — should match.
	gwFP := fileFingerprint{
		Path:   "gw/session1.jsonl",
		Source: SourceGateway,
		Timestamps: []time.Time{
			base.Add(500 * time.Millisecond),
			base.Add(10*time.Second + 800*time.Millisecond),
		},
	}

	dups := FindDuplicates([]fileFingerprint{ccFP}, []fileFingerprint{gwFP})
	if !dups["gw/session1.jsonl"] {
		t.Error("timestamps within 1s window should be detected as duplicate")
	}
}

func TestBuildFingerprint(t *testing.T) {
	msgs := []ConversationMessage{
		{Role: "user", Text: "First message", Timestamp: time.Now()},
		{Role: "assistant", Text: "Second message", Timestamp: time.Now()},
		{Role: "user", Text: "Third message", Timestamp: time.Now()},
		{Role: "assistant", Text: "Fourth message should not be in preview", Timestamp: time.Now()},
	}

	fp := BuildFingerprint("test.jsonl", SourceCC, msgs)

	if fp.Path != "test.jsonl" {
		t.Errorf("Path = %q", fp.Path)
	}
	if fp.Source != SourceCC {
		t.Errorf("Source = %d", fp.Source)
	}
	if len(fp.Timestamps) != 4 {
		t.Errorf("expected 4 timestamps, got %d", len(fp.Timestamps))
	}
	if len(fp.Previews) != 3 {
		t.Errorf("expected 3 previews, got %d", len(fp.Previews))
	}
}

func TestBuildFingerprint_LongTextTruncated(t *testing.T) {
	longText := ""
	for i := 0; i < 200; i++ {
		longText += "x"
	}

	msgs := []ConversationMessage{
		{Role: "user", Text: longText, Timestamp: time.Now()},
	}

	fp := BuildFingerprint("test.jsonl", SourceCC, msgs)

	if len(fp.Previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(fp.Previews))
	}
	if len(fp.Previews[0]) != 100 {
		t.Errorf("expected preview truncated to 100, got %d", len(fp.Previews[0]))
	}
}

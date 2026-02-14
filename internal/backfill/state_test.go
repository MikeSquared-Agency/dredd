package backfill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackfillState_NewAndSave(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	// Override the default state path for testing.
	s := &BackfillState{path: statePath}
	s.MarkProcessed("file1.jsonl")
	s.MarkProcessed("file2.jsonl")
	s.DecisionsFound = 5
	s.PatternsFound = 3
	s.ChunksProcessed = 10

	if err := s.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file was created.
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	// Reload and verify.
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("state file is empty")
	}
}

func TestBackfillState_IsProcessed(t *testing.T) {
	s := &BackfillState{}

	if s.IsProcessed("file1.jsonl") {
		t.Error("file1 should not be processed yet")
	}

	s.MarkProcessed("file1.jsonl")

	if !s.IsProcessed("file1.jsonl") {
		t.Error("file1 should be processed")
	}
	if s.IsProcessed("file2.jsonl") {
		t.Error("file2 should not be processed")
	}
}

func TestBackfillState_AddError(t *testing.T) {
	s := &BackfillState{}
	s.AddError("something went wrong")
	s.AddError("another error")

	if len(s.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(s.Errors))
	}
	if s.Errors[0] != "something went wrong" {
		t.Errorf("error[0] = %q", s.Errors[0])
	}
}

func TestBackfillState_SaveCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "nested", "dir", "state.json")

	s := &BackfillState{path: statePath}
	if err := s.Save(); err != nil {
		t.Fatalf("Save with nested dir failed: %v", err)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state file not created in nested dir: %v", err)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	got := expandHome("~/test/path")
	want := filepath.Join(home, "test/path")
	if got != want {
		t.Errorf("expandHome(~/test/path) = %q, want %q", got, want)
	}

	// Non-tilde paths should pass through.
	got = expandHome("/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("expandHome(/absolute/path) = %q", got)
	}
}

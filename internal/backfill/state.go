package backfill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const defaultStatePath = "~/.openclaw/workspace/dredd-backfill-state.json"

// BackfillState tracks progress for resumable backfill runs.
type BackfillState struct {
	StartedAt       time.Time `json:"started_at"`
	LastProcessedAt time.Time `json:"last_processed_at"`
	FilesProcessed  []string  `json:"files_processed"`
	FilesRemaining  int       `json:"files_remaining"`
	ChunksProcessed int       `json:"chunks_processed"`
	DecisionsFound  int       `json:"decisions_found"`
	PatternsFound   int       `json:"patterns_found"`
	Errors          []string  `json:"errors"`

	path string // not serialized
}

// LoadState loads the backfill state from disk, or creates a new one.
func LoadState() (*BackfillState, error) {
	p := expandHome(defaultStatePath)

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &BackfillState{
				StartedAt: time.Now().UTC(),
				path:      p,
			}, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}

	var s BackfillState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	s.path = p
	return &s, nil
}

// Save persists the state to disk.
func (s *BackfillState) Save() error {
	s.LastProcessedAt = time.Now().UTC()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	return os.WriteFile(s.path, data, 0o644)
}

// IsProcessed returns true if the given file has already been processed.
func (s *BackfillState) IsProcessed(path string) bool {
	for _, f := range s.FilesProcessed {
		if f == path {
			return true
		}
	}
	return false
}

// MarkProcessed records a file as processed.
func (s *BackfillState) MarkProcessed(path string) {
	s.FilesProcessed = append(s.FilesProcessed, path)
}

// AddError records a processing error.
func (s *BackfillState) AddError(msg string) {
	s.Errors = append(s.Errors, msg)
}

func expandHome(path string) string {
	if len(path) > 1 && path[0] == '~' && path[1] == '/' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

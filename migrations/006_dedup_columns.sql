-- 006_dedup_columns.sql
-- Add deduplication columns to reasoning_patterns and decisions tables.

ALTER TABLE reasoning_patterns
  ADD COLUMN IF NOT EXISTS deduped_at timestamptz,
  ADD COLUMN IF NOT EXISTS dedup_survivor_id uuid REFERENCES reasoning_patterns(id);

ALTER TABLE decisions
  ADD COLUMN IF NOT EXISTS deduped_at timestamptz,
  ADD COLUMN IF NOT EXISTS dedup_survivor_id uuid REFERENCES decisions(id);

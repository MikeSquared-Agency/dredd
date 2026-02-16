ALTER TABLE decisions ADD COLUMN IF NOT EXISTS model_id TEXT;
ALTER TABLE decisions ADD COLUMN IF NOT EXISTS model_tier TEXT;
CREATE INDEX IF NOT EXISTS idx_decisions_model ON decisions(model_id);
CREATE INDEX IF NOT EXISTS idx_decisions_model_tier ON decisions(model_tier);

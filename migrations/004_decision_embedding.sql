-- 004_decision_embedding.sql
-- Add optional embedding column to decisions table.

alter table decisions add column if not exists embedding vector(1536);

-- 001_reasoning_patterns.sql
-- Type 2 knowledge: reasoning patterns extracted from conversation transcripts.

create table reasoning_patterns (
  id uuid primary key default gen_random_uuid(),
  owner_uuid uuid not null,
  session_ref text not null,
  pattern_type text not null,          -- reframing | correction | philosophy | direction | pushback
  summary text not null,
  conversation_arc text not null,      -- the relevant portion of transcript
  arc_embedding vector(1536),
  tags text[] default '{}',
  dredd_confidence float not null,
  review_status text default 'pending', -- pending | confirmed | rejected | skipped
  review_note text,
  reviewed_at timestamptz,
  created_at timestamptz default now()
);

create index idx_patterns_owner on reasoning_patterns(owner_uuid);
create index idx_patterns_embedding on reasoning_patterns using ivfflat (arc_embedding vector_cosine_ops) with (lists = 100);
create index idx_patterns_review on reasoning_patterns(review_status);
create index idx_patterns_type on reasoning_patterns(pattern_type);

-- RLS
alter table reasoning_patterns enable row level security;

create policy "Service role full access" on reasoning_patterns
  for all using (auth.role() = 'service_role');

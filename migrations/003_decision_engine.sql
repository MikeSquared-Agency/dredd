-- 003_decision_engine.sql
-- Full Decision Engine tables for Type 1 knowledge.
-- These may already exist — run only if not already deployed.

create table if not exists decisions (
  id uuid primary key default gen_random_uuid(),
  domain text not null,
  category text not null,
  severity text not null default 'routine',
  source text not null default 'dredd',
  decided_by uuid not null,
  summary text not null,
  session_ref text,
  review_status text default 'pending',
  review_note text,
  reviewed_at timestamptz,
  created_at timestamptz default now()
);

create index if not exists idx_decisions_decided_by on decisions(decided_by);
create index if not exists idx_decisions_category on decisions(category);
create index if not exists idx_decisions_review on decisions(review_status);
create index if not exists idx_decisions_session on decisions(session_ref);

create table if not exists decision_context (
  id uuid primary key default gen_random_uuid(),
  decision_id uuid not null references decisions(id) on delete cascade,
  situation_text text not null,
  situation_embedding vector(1536),
  entities jsonb default '[]',
  metrics jsonb default '{}',
  created_at timestamptz default now()
);

create index if not exists idx_context_decision on decision_context(decision_id);

create table if not exists decision_options (
  id uuid primary key default gen_random_uuid(),
  decision_id uuid not null references decisions(id) on delete cascade,
  option_key text not null,
  pro_signals text[] default '{}',
  con_signals text[] default '{}',
  was_chosen boolean not null default false,
  created_at timestamptz default now()
);

create index if not exists idx_options_decision on decision_options(decision_id);

create table if not exists decision_reasoning (
  id uuid primary key default gen_random_uuid(),
  decision_id uuid not null references decisions(id) on delete cascade,
  factors text[] default '{}',
  tradeoffs text[] default '{}',
  reasoning_text text not null,
  reasoning_embedding vector(1536),
  created_at timestamptz default now()
);

create index if not exists idx_reasoning_decision on decision_reasoning(decision_id);

create table if not exists decision_tags (
  id uuid primary key default gen_random_uuid(),
  decision_id uuid not null references decisions(id) on delete cascade,
  tag text not null,
  created_at timestamptz default now()
);

create index if not exists idx_tags_decision on decision_tags(decision_id);
create index if not exists idx_tags_tag on decision_tags(tag);

create table if not exists decision_outcomes (
  id uuid primary key default gen_random_uuid(),
  decision_id uuid not null references decisions(id) on delete cascade,
  outcome_text text not null,
  outcome_quality text, -- positive | negative | neutral
  measured_at timestamptz,
  created_at timestamptz default now()
);

create index if not exists idx_outcomes_decision on decision_outcomes(decision_id);

-- RLS for all decision engine tables
alter table decisions enable row level security;
alter table decision_context enable row level security;
alter table decision_options enable row level security;
alter table decision_reasoning enable row level security;
alter table decision_tags enable row level security;
alter table decision_outcomes enable row level security;

create policy "Service role full access" on decisions for all using (auth.role() = 'service_role');
create policy "Service role full access" on decision_context for all using (auth.role() = 'service_role');
create policy "Service role full access" on decision_options for all using (auth.role() = 'service_role');
create policy "Service role full access" on decision_reasoning for all using (auth.role() = 'service_role');
create policy "Service role full access" on decision_tags for all using (auth.role() = 'service_role');
create policy "Service role full access" on decision_outcomes for all using (auth.role() = 'service_role');

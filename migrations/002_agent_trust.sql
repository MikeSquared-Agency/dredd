-- 002_agent_trust.sql
-- Continuous trust scores per agent/category/severity.
-- Replaces shu_ha_ri_trust. Simpler — continuous scores, no phase labels.

create table agent_trust (
  id uuid primary key default gen_random_uuid(),
  agent_id text not null,
  category text not null,              -- gate_approval | pr_review | architecture | infrastructure | ui | security | ...
  severity text not null,              -- routine | significant | critical

  trust_score float not null default 0.0,  -- 0.0 to 1.0, continuous
  total_decisions integer not null default 0,
  correct_decisions integer not null default 0,
  critical_failures integer not null default 0,

  -- DECAY
  last_signal_at timestamptz,
  decay_rate float not null default 0.01,  -- score decays over time without new signals

  updated_at timestamptz not null default now(),

  unique(agent_id, category, severity)
);

create index idx_trust_agent on agent_trust(agent_id);
create index idx_trust_score on agent_trust(trust_score);

-- RLS
alter table agent_trust enable row level security;

create policy "Service role full access" on agent_trust
  for all using (auth.role() = 'service_role');

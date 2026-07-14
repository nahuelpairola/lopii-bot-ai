-- +goose Up
create table llm_calls (
  id                            bigint generated always as identity primary key,
  created_at                    timestamptz not null default now(),
  trace_id                      text not null,
  call_type                     text not null,
  model                         text not null,
  prompt_tokens                 int,
  completion_tokens             int,
  total_tokens                  int,
  latency_ms                    int not null,
  http_status                   int,
  attempts                      int,
  error                         text,
  ratelimit_remaining_requests  int,
  ratelimit_remaining_tokens    int
);
create index idx_llm_calls_created_at on llm_calls (created_at);
create index idx_llm_calls_trace_id   on llm_calls (trace_id);

create table request_traces (
  id           bigint generated always as identity primary key,
  created_at   timestamptz not null default now(),
  trace_id     text not null unique,
  user_id      bigint references users(id),
  update_type  text not null,
  received_at  timestamptz not null,
  latency_ms   int not null,
  error        text
);
create index idx_request_traces_created_at on request_traces (created_at);

alter table intent_events add column trace_id text;

-- +goose Down
alter table intent_events drop column trace_id;
drop table request_traces;
drop table llm_calls;

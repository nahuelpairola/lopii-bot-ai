-- +goose Up
create table intent_events (
  id                 bigint generated always as identity primary key,
  created_at         timestamptz not null default now(),
  user_id            bigint not null references users(id),
  raw_message        text not null,
  intent             text not null,
  needs_confirmation boolean not null default false,
  outcome            text not null,
  resolved_at        timestamptz,
  was_correct        boolean
);

-- +goose Down
drop table intent_events;

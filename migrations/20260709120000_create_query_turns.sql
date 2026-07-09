-- +goose Up
create table query_turns (
  id         bigint generated always as identity primary key,
  user_id    bigint not null references users(id),
  question   text not null,
  answer     text not null,
  created_at timestamptz not null default now()
);
create index query_turns_user_created_idx on query_turns (user_id, created_at desc);

-- +goose Down
drop table query_turns;

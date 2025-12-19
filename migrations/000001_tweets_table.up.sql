create table tweets
(
    id              uuid      not null default gen_random_uuid(),
    text            text      not null,
    created_at      timestamp not null default now(),
    updated_at      timestamp not null default now(),
    user_id         uuid      not null,
    primary key (id)
);
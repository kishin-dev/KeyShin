-- KeyShin database setup.
--
-- Run this once in Supabase: Dashboard → SQL Editor → New query → paste → Run.
-- It's safe to run again; it only creates what doesn't exist yet.
--
-- Security model:
--   * Every table has Row Level Security turned on with no policies, and the
--     public roles (anon, authenticated) have no privileges at all. Nobody can
--     read or write these tables with the publishable key.
--   * Only the KeyShin server, using the secret key (service_role), can
--     access them.

create extension if not exists pgcrypto;

-- ------------------------------------------------------------------ admins
-- Which Supabase Auth users may sign in to the dashboard, and their username.
-- Being a Supabase user is not enough: you also need a row here.
create table if not exists public.admins (
  user_id    uuid primary key references auth.users (id) on delete cascade,
  username   text not null check (username ~ '^[a-z0-9][a-z0-9_.-]{2,31}$'),
  created_at timestamptz not null default now()
);
create unique index if not exists admins_username_key on public.admins (username);

-- ---------------------------------------------------------------- products
-- Anything you sell access to: a plugin, an app, a paid tier.
create table if not exists public.products (
  id          uuid primary key default gen_random_uuid(),
  slug        text not null unique check (slug ~ '^[a-z0-9][a-z0-9-]{1,63}$'),
  name        text not null check (length(name) between 1 and 120),
  description text,
  key_prefix  text not null default 'KSHN' check (key_prefix ~ '^[A-Z0-9]{2,8}$'),
  created_at  timestamptz not null default now()
);

-- ---------------------------------------------------------------- licenses
create table if not exists public.licenses (
  id              uuid primary key default gen_random_uuid(),
  product_id      uuid not null references public.products (id) on delete restrict,
  key             text not null unique,
  status          text not null default 'active' check (status in ('active', 'revoked')),
  customer_name   text,
  customer_email  text,
  note            text,
  max_activations integer not null default 1 check (max_activations > 0),
  expires_at      timestamptz,
  created_at      timestamptz not null default now(),
  revoked_at      timestamptz
);
create index if not exists licenses_product_id_idx on public.licenses (product_id);
create index if not exists licenses_customer_email_idx on public.licenses (lower(customer_email));

-- ------------------------------------------------------------- activations
-- The machines or servers a license is being used on.
create table if not exists public.activations (
  id           uuid primary key default gen_random_uuid(),
  license_id   uuid not null references public.licenses (id) on delete cascade,
  fingerprint  text not null,
  label        text,
  created_at   timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  unique (license_id, fingerprint)
);

-- ------------------------------------------------------- lock tables down
alter table public.admins      enable row level security;
alter table public.products    enable row level security;
alter table public.licenses    enable row level security;
alter table public.activations enable row level security;

revoke all on public.admins, public.products, public.licenses, public.activations
  from anon, authenticated;

-- ---------------------------------------------------------------- functions
-- Looks up the email for a dashboard username, so admins can sign in with a
-- username even though Supabase Auth signs in by email.
create or replace function public.keyshin_admin_email(p_username text)
returns text
language sql
stable
security definer
set search_path = ''
as $$
  select u.email
  from public.admins a
  join auth.users u on u.id = a.user_id
  where a.username = lower(trim(p_username))
$$;

-- Totals for the dashboard overview.
create or replace function public.keyshin_stats()
returns json
language sql
stable
set search_path = ''
as $$
  select json_build_object(
    'products', (select count(*) from public.products),
    'active',   (select count(*) from public.licenses
                 where status = 'active' and (expires_at is null or expires_at > now())),
    'revoked',  (select count(*) from public.licenses where status = 'revoked')
  )
$$;

revoke all on function public.keyshin_admin_email(text) from public, anon, authenticated;
revoke all on function public.keyshin_stats()          from public, anon, authenticated;
grant execute on function public.keyshin_admin_email(text) to service_role;
grant execute on function public.keyshin_stats()          to service_role;

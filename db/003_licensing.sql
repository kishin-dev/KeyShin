-- KeyShin: products, licenses and license validation.
--
-- Run this in Supabase after 001_init.sql: SQL Editor → New query → paste → Run.
-- It's safe to run again; it replaces the functions with the latest version.
--
-- All functions are only callable by the KeyShin server (service_role).

-- ------------------------------------------------------------ products list
-- Every product with how many licenses it has.
create or replace function public.keyshin_products()
returns json
language sql
stable
set search_path = ''
as $$
  select coalesce(json_agg(json_build_object(
    'id', t.id,
    'slug', t.slug,
    'name', t.name,
    'description', t.description,
    'keyPrefix', t.key_prefix,
    'createdAt', t.created_at,
    'activeLicenses', t.active_licenses,
    'totalLicenses', t.total_licenses
  ) order by t.created_at desc), '[]'::json)
  from (
    select
      p.id, p.slug, p.name, p.description, p.key_prefix, p.created_at,
      count(l.id) filter (
        where l.status = 'active' and (l.expires_at is null or l.expires_at > now())
      ) as active_licenses,
      count(l.id) as total_licenses
    from public.products p
    left join public.licenses l on l.product_id = p.id
    group by p.id
  ) t
$$;

-- ---------------------------------------------------------- license search
-- Searches licenses by key, customer name or customer email.
--   p_state: 'active', 'expired', 'revoked' or null for all.
-- Returns {"items": [...], "total": n}.
create or replace function public.keyshin_search_licenses(
  p_query   text default null,
  p_product uuid default null,
  p_state   text default null,
  p_limit   integer default 50,
  p_offset  integer default 0
)
returns json
language sql
stable
set search_path = ''
as $$
  with base as (
    select
      l.*,
      case
        when l.status = 'revoked' then 'revoked'
        when l.expires_at is not null and l.expires_at <= now() then 'expired'
        else 'active'
      end as state,
      p.name as product_name,
      p.slug as product_slug,
      (select count(*) from public.activations a where a.license_id = l.id) as activations
    from public.licenses l
    join public.products p on p.id = l.product_id
  ),
  filtered as (
    select * from base
    where (p_product is null or product_id = p_product)
      and (p_state is null or state = p_state)
      and (
        coalesce(trim(p_query), '') = ''
        or strpos(lower(key), lower(trim(p_query))) > 0
        or strpos(lower(coalesce(customer_name, '')), lower(trim(p_query))) > 0
        or strpos(lower(coalesce(customer_email, '')), lower(trim(p_query))) > 0
      )
  )
  select json_build_object(
    'total', (select count(*) from filtered),
    'items', coalesce((
      select json_agg(json_build_object(
        'id', id,
        'key', key,
        'state', state,
        'customerName', customer_name,
        'customerEmail', customer_email,
        'maxActivations', max_activations,
        'activations', activations,
        'expiresAt', expires_at,
        'createdAt', created_at,
        'product', json_build_object('id', product_id, 'name', product_name, 'slug', product_slug)
      ) order by created_at desc)
      from (
        select * from filtered
        order by created_at desc
        limit least(greatest(coalesce(p_limit, 50), 1), 100)
        offset greatest(coalesce(p_offset, 0), 0)
      ) page
    ), '[]'::json)
  )
$$;

-- ---------------------------------------------------------- license detail
-- One license with its product and activations, or null if it doesn't exist.
create or replace function public.keyshin_license(p_id uuid)
returns json
language sql
stable
set search_path = ''
as $$
  select json_build_object(
    'id', l.id,
    'key', l.key,
    'status', l.status,
    'state', case
      when l.status = 'revoked' then 'revoked'
      when l.expires_at is not null and l.expires_at <= now() then 'expired'
      else 'active'
    end,
    'customerName', l.customer_name,
    'customerEmail', l.customer_email,
    'note', l.note,
    'maxActivations', l.max_activations,
    'expiresAt', l.expires_at,
    'createdAt', l.created_at,
    'revokedAt', l.revoked_at,
    'product', json_build_object('id', p.id, 'name', p.name, 'slug', p.slug),
    'activations', coalesce((
      select json_agg(json_build_object(
        'id', a.id,
        'fingerprint', a.fingerprint,
        'label', a.label,
        'createdAt', a.created_at,
        'lastSeenAt', a.last_seen_at
      ) order by a.last_seen_at desc)
      from public.activations a where a.license_id = l.id
    ), '[]'::json)
  )
  from public.licenses l
  join public.products p on p.id = l.product_id
  where l.id = p_id
$$;

-- -------------------------------------------------------------- validation
-- Checks a license key for a product, and records the machine using it.
--
--   p_key          the license key the customer entered
--   p_product      the product's slug, sent by your plugin
--   p_fingerprint  an ID for the machine/server (optional; without it the
--                  key is checked but no activation is recorded)
--   p_label        a readable name for the machine, e.g. a hostname (optional)
--
-- Returns {"valid": true|false, "code": "...", ...}. The row lock makes
-- simultaneous activations safe: a license can't go over its limit even if
-- several machines activate at the same moment.
create or replace function public.keyshin_validate(
  p_key         text,
  p_product     text,
  p_fingerprint text default null,
  p_label       text default null
)
returns json
language plpgsql
set search_path = ''
as $$
declare
  lic        public.licenses%rowtype;
  prod_slug  text;
  used       integer;
  fp         text := nullif(trim(p_fingerprint), '');
  is_new     boolean := false;
begin
  select l.* into lic
  from public.licenses l
  where l.key = upper(trim(p_key))
  for update;

  if not found then
    return json_build_object('valid', false, 'code', 'not_found');
  end if;

  select slug into prod_slug from public.products where id = lic.product_id;
  if prod_slug <> lower(trim(p_product)) then
    return json_build_object('valid', false, 'code', 'wrong_product');
  end if;

  if lic.status = 'revoked' then
    return json_build_object('valid', false, 'code', 'revoked');
  end if;

  if lic.expires_at is not null and lic.expires_at <= now() then
    return json_build_object('valid', false, 'code', 'expired', 'expiresAt', lic.expires_at);
  end if;

  if fp is not null then
    update public.activations
       set last_seen_at = now(),
           label = coalesce(nullif(trim(p_label), ''), label)
     where license_id = lic.id and fingerprint = fp;

    if not found then
      select count(*) into used from public.activations where license_id = lic.id;
      if used >= lic.max_activations then
        return json_build_object(
          'valid', false, 'code', 'activation_limit',
          'maxActivations', lic.max_activations, 'activations', used
        );
      end if;
      insert into public.activations (license_id, fingerprint, label)
      values (lic.id, fp, nullif(trim(p_label), ''));
      is_new := true;
    end if;
  end if;

  select count(*) into used from public.activations where license_id = lic.id;

  return json_build_object(
    'valid', true,
    'code', 'valid',
    'product', prod_slug,
    'expiresAt', lic.expires_at,
    'maxActivations', lic.max_activations,
    'activations', used,
    'newActivation', is_new
  );
end
$$;

-- ------------------------------------------------------------ permissions
revoke all on function public.keyshin_products()                                     from public, anon, authenticated;
revoke all on function public.keyshin_search_licenses(text, uuid, text, integer, integer) from public, anon, authenticated;
revoke all on function public.keyshin_license(uuid)                                  from public, anon, authenticated;
revoke all on function public.keyshin_validate(text, text, text, text)               from public, anon, authenticated;

grant execute on function public.keyshin_products()                                     to service_role;
grant execute on function public.keyshin_search_licenses(text, uuid, text, integer, integer) to service_role;
grant execute on function public.keyshin_license(uuid)                                  to service_role;
grant execute on function public.keyshin_validate(text, text, text, text)               to service_role;

-- Make a Supabase Auth user a KeyShin admin.
--
-- 1. Supabase Dashboard → Authentication → Users → Add user → Create new user.
--    Enter the email and password, and tick "Auto Confirm User".
-- 2. Change the email and username below, then run this in the SQL Editor.
--    The username is what you type on the KeyShin login page
--    (3–32 characters: lowercase letters, numbers, . _ -).

insert into public.admins (user_id, username)
select id, 'bartol'
from auth.users
where email = 'you@example.com'
on conflict (user_id) do update set username = excluded.username;

-- Check it worked (should return one row):
select a.username, u.email
from public.admins a join auth.users u on u.id = a.user_id;

# KeyShin

License key management for [Kishin](https://github.com/kishin-dev) projects: issue keys to customers, check them from your plugins and services, and revoke them when needed.

> **Status:** early. Admin sign-in and the dashboard overview work; products, licenses and the validation API are next.

## Stack

- **Go** (standard library only) for the API, deployed as Vercel serverless functions in Frankfurt
- **Supabase** for the Postgres database and admin authentication
- **HTML, Tailwind and vanilla JS** for the admin dashboard (Tailwind is compiled at build time)
- Live at **licenses.kishin.lol**

## Project layout

```
api/            Vercel functions, one per endpoint
lib/auth/       Admin sign-in with Supabase Auth, session cookies
lib/supa/       Small Supabase client (Auth + database REST API)
lib/store/      Database queries
lib/httpx/      JSON/HTTP helpers shared by the handlers
lib/supafake/   In-memory fake Supabase for tests and local demos
db/             SQL to run in the Supabase SQL Editor
public/         Static site: / is the login page, /dashboard is the dashboard
web/            Tailwind source, compiled into public/css/tailwind.css
cmd/dev/        Local dev server that mimics Vercel (not deployed)
```

Each file in `api/` is deployed by Vercel as its own function, so shared code lives in `lib/` and gets imported.

## Setup

### 1. Supabase

1. **SQL Editor → New query**, paste `db/001_init.sql`, then **Run**. This creates the tables and locks them down.
2. **Authentication → Users → Add user → Create new user.** Enter your email and a strong password, and tick **Auto Confirm User**.
3. Open `db/002_add_admin.sql`, put in your email and the username you want to log in with, and run it in the SQL Editor.
4. **Authentication → Sign In / Providers:** turn off **Allow new users to sign up**. Admins are only ever added by you.
5. **Project Settings → API Keys:** copy the publishable key and the secret key for the next step.

### 2. Vercel

Add these under **Settings → Environment Variables** (all environments), then redeploy:

| Name                       | Value                                      |
| -------------------------- | ------------------------------------------ |
| `SUPABASE_URL`             | `https://<project-ref>.supabase.co`        |
| `SUPABASE_PUBLISHABLE_KEY` | `sb_publishable_...` (or legacy anon key)  |
| `SUPABASE_SECRET_KEY`      | `sb_secret_...` (or legacy service_role key) |

`vercel.json` already sets the build command, the output folder and the Frankfurt region.

## Running locally

Requires Go 1.22+ and Node 18+.

```bash
npm install
npm run build              # or `npm run watch` in another terminal while editing
go run ./cmd/dev -fake     # fake Supabase, sign in with admin / password
```

To use your real Supabase project instead, `cp .env.example .env`, fill it in and run `go run ./cmd/dev`.

Run the tests with `go test ./...`.

## API

| Method | Path           | Auth  | Description                                     |
| ------ | -------------- | ----- | ----------------------------------------------- |
| POST   | `/api/login`   | –     | `{"username","password"}` → sets session cookies |
| POST   | `/api/logout`  | –     | Ends the session                                |
| GET    | `/api/session` | admin | The signed-in admin                             |
| GET    | `/api/stats`   | admin | Totals for the overview                         |

## Security

- **Sign-in** goes through Supabase Auth (hashed passwords, rate limiting). Admins type a username; the server looks up the matching Supabase user, and **only users listed in the `admins` table can sign in**, even if someone manages to create a Supabase account.
- **Sessions** are Supabase's access and refresh tokens, kept in `HttpOnly`, `Secure`, `SameSite=Strict`, `__Host-` cookies that browser JavaScript can't read. Access tokens are checked with Supabase on every request, so logging out or deleting an admin takes effect immediately. Sessions end after 12 hours of inactivity.
- **The database** has Row Level Security on every table and no privileges for the public roles, so the publishable key can't read anything. Only the server, using the secret key, can.
- The dashboard HTML is public, like any static file, but holds no data. All data comes from API endpoints that check the session.

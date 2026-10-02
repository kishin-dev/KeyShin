# KeyShin

License key management for [Kishin](https://github.com/kishin-dev) projects: issue keys to customers, check them from your plugins and services, and revoke them when needed.

- **Products**: anything you sell access to, each with its own key prefix (`DBOT-…`, `MCP-…`)
- **Licenses**: random keys like `DBOT-VNHG-QX4Z-LEXA-ZJ74`, with an optional customer, expiry date and machine limit
- **Activations**: each license tracks the machines using it, so one key can't be shared beyond its limit
- **Validation API**: one public endpoint your plugins call to check a key

## Stack

- **Go** (standard library only) for the API, deployed as Vercel serverless functions in Frankfurt
- **Supabase** for the Postgres database and admin authentication
- **HTML, Tailwind and vanilla JS** for the admin dashboard (Tailwind is compiled at build time)
- Live at **licenses.kishin.lol**

## Project layout

```
api/            Admin endpoints, one Vercel function per file
api/v1/         Public API your plugins call
lib/auth/       Admin sign-in with Supabase Auth, session cookies
lib/supa/       Small Supabase client (Auth + database REST API)
lib/store/      Products, licenses, key generation and validation
lib/httpx/      JSON/HTTP helpers shared by the handlers
lib/supafake/   In-memory fake Supabase Auth for tests
db/             SQL to run in the Supabase SQL Editor, in order
public/         Dashboard pages: / (login), /dashboard, /licenses, /products
web/            Tailwind source, compiled into public/css/tailwind.css
cmd/dev/        Local dev server that mimics Vercel (not deployed)
```

Each file in `api/` is deployed by Vercel as its own function, so shared code lives in `lib/` and gets imported.

## Setup

### 1. Supabase

1. **SQL Editor → New query**: paste `db/001_init.sql` and **Run**. Then do the same with `db/003_licensing.sql`. Both are safe to run again.
2. **Authentication → Users → Add user → Create new user.** Enter your email and a strong password, and tick **Auto Confirm User**.
3. Open `db/002_add_admin.sql`, put in your email and the username you want to log in with, and run it in the SQL Editor.
4. **Authentication → Sign In / Providers:** turn off **Allow new users to sign up**. Admins are only ever added by you.
5. **Project Settings → API Keys:** copy the publishable key and the secret key for the next step.

### 2. Vercel

Add these under **Settings → Environment Variables** (all environments), then redeploy:

| Name                       | Value                                        |
| -------------------------- | -------------------------------------------- |
| `SUPABASE_URL`             | `https://<project-ref>.supabase.co`          |
| `SUPABASE_PUBLISHABLE_KEY` | `sb_publishable_...` (or legacy anon key)    |
| `SUPABASE_SECRET_KEY`      | `sb_secret_...` (or legacy service_role key) |

`vercel.json` already sets the build command, the output folder and the Frankfurt region.

## Using it

1. **Products → New product.** The **slug** (e.g. `discord-bot-pro`) is what your plugin sends to say which product it is, so it can't be changed later. The **key prefix** starts every key for this product.
2. **Licenses → Issue license.** Pick the product, optionally add the customer, how many machines may use the key, and an expiry date. Copy the key and send it to the customer.
3. In your plugin, check the key with the validation API (below) when it starts.
4. To stop a key working, open it and click **Revoke license**. You can restore it later. To free a slot, remove an old machine from the license.

## Checking a license from your plugin

```
POST https://licenses.kishin.lol/api/v1/validate
Content-Type: application/json

{
  "key": "DBOT-VNHG-QX4Z-LEXA-ZJ74",
  "product": "discord-bot-pro",
  "fingerprint": "a-stable-id-for-this-machine",
  "label": "bot-server-1"
}
```

| Field         | Required | Meaning                                                                                                                                  |
| ------------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `key`         | yes      | The key the customer entered. Case and surrounding spaces don't matter.                                                                  |
| `product`     | yes      | The product's slug.                                                                                                                      |
| `fingerprint` | no       | Something that stays the same on one machine (e.g. a hash of the machine ID). With it, the machine takes one of the license's slots. Without it, the key is checked but no machine is recorded. |
| `label`       | no       | A readable name shown in the dashboard, like a hostname.                                                                                 |

The answer is always HTTP 200 when the check itself worked:

```json
{
  "valid": true,
  "code": "valid",
  "message": "License is valid.",
  "product": "discord-bot-pro",
  "expiresAt": null,
  "maxActivations": 2,
  "activations": 1,
  "newActivation": true
}
```

```json
{ "valid": false, "code": "activation_limit", "message": "This license is already active on the maximum number of machines. …", "maxActivations": 2, "activations": 2 }
```

| `code`             | Meaning                                                            |
| ------------------ | ------------------------------------------------------------------ |
| `valid`            | The key works for this product on this machine.                    |
| `not_found`        | No such key.                                                       |
| `wrong_product`    | The key exists, but for a different product.                       |
| `revoked`          | You revoked it.                                                    |
| `expired`          | Past its expiry date.                                              |
| `activation_limit` | All machine slots are taken by other machines.                     |

`message` is written for customers, so your plugin can show it directly. Other HTTP statuses mean the request itself was wrong (`400`, with an `error`) or KeyShin couldn't reach the database (`502`). Treat those as "couldn't check" rather than "invalid".

**Example (JavaScript / Node 18+):**

```js
async function checkLicense(key) {
  const res = await fetch('https://licenses.kishin.lol/api/v1/validate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ key, product: 'discord-bot-pro', fingerprint: machineId(), label: os.hostname() }),
    signal: AbortSignal.timeout(8000),
  });
  if (!res.ok) throw new Error(`License server error ${res.status}`);
  return res.json(); // { valid, code, message, ... }
}
```

**Tip:** remember the last successful check for a while (for example 24 hours) and only refuse to run when the server *says* the key is invalid. That way a short outage of KeyShin, or the customer's internet, doesn't stop your plugin.

## Running locally

Requires Go 1.22+ and Node 18+.

```bash
cp .env.example .env       # fill in your Supabase URL and keys
npm install
npm run build              # or `npm run watch` in another terminal while editing
go run ./cmd/dev           # http://localhost:3000
```

## Tests

```bash
go test ./...
```

The database tests in `lib/store` run against a real Postgres with PostgREST (the API layer Supabase uses) and are skipped unless you point them at one:

```bash
KEYSHIN_TEST_POSTGREST_URL=http://127.0.0.1:3001 \
KEYSHIN_TEST_SERVICE_JWT=<JWT with {"role":"service_role"}> \
go test ./lib/store/
```

Use a throwaway database with `db/001_init.sql` and `db/003_licensing.sql` applied. Never point the tests at production.

## Admin API

All of these need an admin session (the dashboard's cookies).

| Method | Path                                         | Description                                |
| ------ | -------------------------------------------- | ------------------------------------------ |
| POST   | `/api/login`                                 | `{"username","password"}`, starts a session |
| POST   | `/api/logout`                                | Ends the session                           |
| GET    | `/api/session`                               | The signed-in admin                        |
| GET    | `/api/stats`                                 | Totals for the overview                    |
| GET    | `/api/products`                              | List products                              |
| POST   | `/api/products`                              | Create a product                           |
| PATCH  | `/api/products?id=…`                         | Edit name, description or key prefix       |
| GET    | `/api/licenses?q=&product=&state=&limit=&offset=` | Search licenses                       |
| GET    | `/api/licenses?id=…`                         | One license with its machines              |
| POST   | `/api/licenses`                              | Issue a license                            |
| PATCH  | `/api/licenses?id=…`                         | Edit, revoke (`"status":"revoked"`) or restore (`"status":"active"`) |
| DELETE | `/api/activations?id=…`                      | Remove a machine from its license          |

## Security

- **Sign-in** goes through Supabase Auth (hashed passwords, rate limiting). Admins type a username; the server looks up the matching Supabase user, and **only users listed in the `admins` table can sign in**, even if someone manages to create a Supabase account.
- **Sessions** are Supabase's access and refresh tokens, kept in `HttpOnly`, `Secure`, `SameSite=Strict`, `__Host-` cookies that browser JavaScript can't read. Access tokens are checked with Supabase on every request, so logging out or deleting an admin takes effect immediately. Sessions end after 12 hours of inactivity.
- **The database** has Row Level Security on every table and no privileges for the public roles, so the publishable key can't read anything. Only the server, using the secret key, can.
- **License keys** carry 80 random bits, far too many to guess. Activation limits are enforced inside the database with a row lock, so they hold even when many machines activate at the same moment.
- The dashboard HTML is public, like any static file, but holds no data. All data comes from API endpoints that check the session.

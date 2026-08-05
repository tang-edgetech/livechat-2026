# LiveChat Project — Overview & Plan

Status: **Planning draft** — living document, updated as decisions are made.
Last updated: 2026-07-29

> **Resuming in a new session:** this file is self-contained — a fresh session can read it and start building directly from §4 (schema) and §5 (setup wizard) onward. §9 lists what's been decided; **§12 (always the last section) lists what's still open** and should be resolved (or explicitly deferred) before the part it touches gets built — as items get resolved they move up into §9/the relevant section, and §12 stays last. **§11 is the build roadmap** — follow it phase by phase rather than building everything at once.

---

## 1. Product Summary

A customer-care / livechat support platform, multi-tenant across **Merchants (Brands)**, used by a support organization to run livechat for their brands' customers ("Visitors"). Target scale: thousands of concurrent real-time users at launch, architected to scale to millions.

### 1.1 Roles

| Role | Who | Scope |
|---|---|---|
| **Super Admin** | Dev/platform team | Full system access, sees & manages every account (incl. Admins), runs initial setup, only role that can delete Audit Logs |
| **Admin** | Customer's CEO/GM-level stakeholder | Manages their own Merchant's Agents, Settings, sees Overview stats for their merchant(s) |
| **Agent** | Customer's support team member (formerly "Operator") | Handles chats assigned/available to them |

> Naming decision: the support-rep role is called **Agent** in the UI (industry-standard term, e.g. Zendesk/Intercom). Internally the DB role slug is `agent`.

### 1.2 Confirmed Tech Stack

| Layer | Choice | Notes |
|---|---|---|
| Frontend | **Next.js** + **Tailwind CSS** + **Ant Design** | SPA-like behavior via AJAX (fetch/XHR) for all actions — no full page reloads |
| Backend API | **Go** (recommend **Gin** framework) | REST endpoints for all AJAX actions |
| Real-time transport | **WebSocket** (Go `gorilla/websocket` or `nhooyr/websocket`) for the live chat stream only | See §2 |
| Database | **MySQL** | Single shared database; merchant scoping via a `merchant_id` column on single-owner tables and dedicated pivot tables where an Admin/Agent/content item can span several merchants (see §3, §4) |
| Cache / Pub-Sub | **Redis** (confirmed addition) | Needed once you run more than one Go instance — see §2 |
| Local dev | XAMPP (Apache + MySQL) for DB/local services | Next.js dev server and Go binary run alongside, not through Apache/PHP |
| File storage | Local disk initially (dev on XAMPP, prod on self-hosted/cPanel), abstracted behind a storage interface so it can move to S3-compatible object storage later without touching business logic | See §7 |

---

## 2. Real-Time Architecture (important — read this)

Your requirement "**every execution runs via AJAX so the user stays on the same page**" is the right call for all admin/panel actions (save, delete, filter, status change, bulk edit, etc.) — plain fetch calls, update the view in place, no navigation.

However, the **live chat message stream itself should not be AJAX polling**. Polling thousands of open chats every few seconds does not scale to "millions of users" — it multiplies DB load and adds latency. The standard pattern (used by every livechat SaaS) is a **hybrid**:

- **AJAX** — all actions: send/save/delete/filter/settings/etc. Confirmed, stays within your requirement.
- **WebSocket** — one persistent connection per logged-in user (Agent, Admin, or Visitor widget), used **only** to push new messages / status changes / typing indicators in real time. A message sent via AJAX POST is broadcast to the other party over WebSocket — the sender's own UI updates optimistically from the AJAX response, no round-trip needed.

**Scaling note:** once you run more than one Go backend instance (needed to reach "millions" scale) or move to Cloudflare, a single instance no longer holds every WebSocket connection for a given chat. Add **Redis Pub/Sub**: each Go instance subscribes to Redis channels per chat/merchant, so any instance can broadcast a message to whichever instance is holding the recipient's socket. This is a small addition now that avoids a rewrite later — recommend building the WebSocket hub with this abstraction from day one even if you launch on a single server.

**cPanel / production hosting flag:** persistent WebSocket connections and long-running Go binaries generally do **not** run on shared cPanel hosting (which is built around PHP-FPM/CGI request-response, not long-lived processes). If cPanel is a hard requirement, you need a **VPS with cPanel** (not shared hosting) so you can run the Go binary as a service, or put the Go app behind Cloudflare on a VPS/dedicated box with cPanel used only for DNS/mail/file management. Worth deciding before committing to a hosting provider.

---

## 3. Roles × DB Multi-Tenancy Model

Confirmed approach: **single shared database**, kept as simple as possible. `Merchant` is a dimension on the relevant tables, not a separate schema or database per merchant. This keeps migrations, cross-merchant reporting (Super Admin/Admin Overview stats), and operations simple, and scales to millions of rows with normal indexing.

- **Super Admin** → belongs to no merchant, sees/manages everything.
- **Admin** and **Agent** → can each hold **multiple merchants** (many-to-many), via the `user_merchant` pivot table (§4).
- **Chat** → still a single `merchant_id` FK — one chat always belongs to exactly one brand.
- **Canned Message, Automation Rule, Bot Flow, Integration** → can each apply to **multiple merchants**, via their own dedicated pivot tables (§4), or be marked global.

**Assignment rules (enforced server-side, not just UI):**
- **Super Admin** creates any account, with any role, and assigns any merchant(s) to it.
- **Admin** can grant/revoke merchant access for their own Agents, but only from among the merchants the Admin itself already has — an Admin can never hand out access to a merchant they don't hold.
- **Agent** cannot modify their own merchant assignment at all — that control simply isn't exposed to the Agent role, in the UI or the API.

Table naming convention: **singular, simple, one word where possible** (`user`, `chat`, `message`), no prefixes like `tbl_` or `lc_` — see rationale in chat.

---

## 4. Database Schema (v1 draft)

### Core

**`role`** — lookup table
| column | type | notes |
|---|---|---|
| id | tinyint PK | |
| slug | varchar(30) unique | `super_admin`, `admin`, `agent` |
| name | varchar(50) | display label |

**`merchant`** — a brand/customer account
| column | type | notes |
|---|---|---|
| id | bigint PK | |
| uuid | char(36) unique | external-facing id |
| name | varchar(120) | |
| code | varchar(50) unique | slug used in widget embed / API |
| status | enum(active, suspended) | |
| routing_mode | enum(manual, round_robin), default `manual` | how new chats get assigned to an Agent — see §6.9 |
| last_routed_agent_id | bigint FK → user.id, nullable | round-robin cursor: last Agent a chat was auto-assigned to for this merchant |
| widget_config | JSON, nullable | accent color, logo/avatar URL, corner position, default language — see §10.4 |
| inactivity_timeout_minutes | smallint, default `30` | auto-close an idle chat after this many minutes of no Visitor activity — Admin-configurable, see §10.5 |
| created_at, updated_at | datetime | |

**`user`** — Super Admin / Admin / Agent accounts (internal system users)
| column | type | notes |
|---|---|---|
| id | bigint PK | |
| uuid | char(36) unique | |
| role_id | tinyint FK | |
| display_name | varchar(80) | editable by user (Profile) |
| username | varchar(50) unique | |
| email | varchar(120) unique | |
| password_hash | varchar(255) | |
| status | enum(active, inactive, suspended) | Admin/Super Admin can change |
| items_per_page | smallint, nullable | per-user override; falls back to `setting.items_per_page_default` |
| last_login_at | datetime, nullable | |
| created_at, updated_at | datetime | |

**`user_merchant`** — pivot: which merchant(s) an Admin/Agent can access (Super Admin has no rows here — needs none)
| column | type | notes |
|---|---|---|
| id | bigint PK | |
| user_id | bigint FK | |
| merchant_id | bigint FK | |
| assigned_by | bigint FK → user.id | who granted this access (audit trail) |
| created_at | datetime | |

unique index on (`user_id`, `merchant_id`) to prevent duplicate grants.

**`visitor`** — the customer on the other end of a chat (not a system `user`)
| column | type | notes |
|---|---|---|
| id | bigint PK | |
| uuid | char(36) unique | this is what the widget stores client-side (cookie/localStorage) and sends on every request/WebSocket connect — the raw `id` never reaches the visitor's browser, same rule as §4.1 |
| merchant_id | bigint FK | |
| display_name | varchar(80), default `Visitor` | from the pre-chat form's Full Name field |
| phone | varchar(20), nullable | normalized incl. country code (e.g. `60123456789`, `6589001001`) — the **primary** identity key, see below |
| email | varchar(120), nullable | secondary identity key, kept "for record" |
| fingerprint | varchar(100) | browser/device identifier; last resort only, once phone or email is known those take priority |
| merged_into_id | bigint FK → visitor.id, nullable | set when this row has been merged into another visitor record (§10.3) — a retired record, kept for history/audit, excluded from future lookups |
| created_at, updated_at | datetime | |

Intended uniqueness: one active (`merged_into_id IS NULL`) visitor per `(merchant_id, phone)` and per `(merchant_id, email)`. MySQL unique indexes can't be conditioned on another column, so this is enforced at the application layer (check-then-insert against non-merged rows) rather than a DB constraint.

**Identity resolution at chat start** (pre-chat form or logged-in passthrough): 1) normalize the submitted phone and look up a non-merged `visitor` row with that phone for this merchant — **phone is the primary match key**; 2) if no phone match, fall back to matching by email; 3) if neither matches, create a new `visitor` row; 4) if phone matches one existing row and email matches a *different* existing row (conflict — e.g. someone reused an old email on a new number), do **not** auto-merge — proceed with the phone-matched row, and raise it as a flagged possible-duplicate for Admin to resolve via the manual merge tool (§10.3), logged in `audit_log`.

### Chat

**`chat`**
| column | type | notes |
|---|---|---|
| id | bigint PK | |
| uuid | char(36) unique | chat id shown in UI |
| merchant_id | bigint FK | |
| visitor_id | bigint FK | |
| agent_id | bigint FK, nullable | current PIC |
| status | enum(active, pending, closed, bot) | drives the red/green dot |
| started_at, closed_at, last_message_at | datetime | |

**`message`**
| column | type | notes |
|---|---|---|
| id | bigint PK | |
| chat_id | bigint FK | |
| sender_type | enum(visitor, agent, bot, system) | |
| sender_id | bigint, nullable | meaning depends on `sender_type`: `visitor`→`visitor.id`, `agent`→`user.id`, `bot`→`bot_flow.id`, `system`→`NULL`. No single FK constraint possible since it's polymorphic; enforce the mapping in application code. |
| body | text | plain text (label/value) — even for a `quick_reply` message this holds the question text; for a visitor's button click this holds the chosen option's value, identical in shape to a free-typed reply |
| type | enum(text, file, system, quick_reply) | `quick_reply` is a bot message rendered with clickable options — see `metadata` |
| metadata | JSON, nullable | only populated for `type = quick_reply`: `{ options: [ {label, value}, ... ] }`. A visitor's click just posts a normal `type = text` message with that option's value as `body` — the bot flow's `ask_question` node evaluates it exactly like free-typed input, no special-case handling needed downstream |
| created_at | datetime | index for ordering |

### Files

**`file`**
| column | type | notes |
|---|---|---|
| id | bigint PK | |
| uuid | char(36) unique | required — file download links must never expose the raw incrementing id (enumeration risk) |
| merchant_id | bigint FK | |
| chat_id | bigint FK, nullable | set when uploaded during a chat (customer upload) |
| uploader_type | enum(visitor, user, system) | |
| uploader_id | bigint, nullable | |
| purpose | enum(chat, automation, bot, branding) | `chat`/`branding` cover the two upload categories from §6.8 plus merchant widget assets; `branding` = logo/avatar uploaded for `merchant.widget_config` (§10.4) — routed through the same Files system so format/permission rules in §6.8 still apply, rather than bypassing it |
| disk_path | varchar(255) | relative path under the storage driver root |
| original_name | varchar(255) | |
| mime_type | varchar(100) | |
| size_bytes | int unsigned | |
| created_at | datetime | |

### Automation / Bot

These four can each apply to **one, several, or all** merchants — so instead of a single `merchant_id` column, each has an `is_global` flag plus its own pivot table. `is_global = true` → applies to every merchant under that company, ignore the pivot rows. `is_global = false` → applies only to the merchants listed in the pivot.

**`canned_message`**
| id, title, body, is_global, created_by, created_at |

**`canned_message_merchant`** — pivot: id, canned_message_id, merchant_id

**`automation_rule`** — initial prompt + auto-response
| id, name, trigger_type, condition (JSON), message, is_global, is_active, created_at |

**`automation_rule_merchant`** — pivot: id, automation_rule_id, merchant_id

**`condition` JSON shape** (flat list, not a node graph — this is the "Simple rule list" tier from §6.3, deliberately simpler than `bot_flow`'s trigger+flow graph in §6.4 since Automation only ever fires one message, no branching):
```
{ logic: and | or, rules: [ {field, operator, value}, ... ] }
```
e.g. `{ logic: "and", rules: [ {field: "page_url", operator: "contains", value: "/pricing"}, {field: "time_of_day", operator: "between", value: ["09:00","18:00"]} ] }`.

**`bot_flow`**
| id, name, trigger (JSON), flow (JSON), integration_id (nullable FK), is_global, is_active, created_at |

**`bot_flow_merchant`** — pivot: id, bot_flow_id, merchant_id

**`flow` JSON shape** (node graph, not a flat rule list — see §6.4):
```
trigger:  { type: chat_start | keyword | condition, conditions: [ {field, operator, value, logic} ] }
nodes:    [ { id, type, config, next }  or  { id, type: "condition", config, branches: { true: nodeId, false: nodeId } } ]
entry:    "<first node id>"
```
Node `type` = the "tools/actions" you chain from a Trigger: `send_message`, `ask_question` (capture visitor input into a variable — config may include `options: [{label, value}, ...]` to render as quick-reply buttons via `message.type = quick_reply`, or omit `options` for a free-text question), `condition` (branch), `call_integration` (the outbound bot/AI REST call, §6.4), `set_variable`, `delay`, `handoff_to_agent`, `close_chat`. Because it's a graph of nodes + edges (`next`/`branches`), it can be rendered as a flow-chart diagram straight from the JSON for the "preview flow" button — no separate diagram format to maintain.

### Integration

**`integration`** — REST API keys, B2B auto-login
| column | type | notes |
|---|---|---|
| id | bigint PK | |
| type | enum(api_key, auto_login, webhook, widget_identity) | `api_key` = inbound (external system authenticates to us); `webhook` = outbound (we call an external endpoint — covers both notification webhooks and a Bot flow's AI/third-party call, §6.4); `auto_login` = B2B deep-link into the admin/Agent panel; `widget_identity` = the merchant's secret for signing logged-in-visitor passthrough tokens (§10.2) — deliberately a **separate** secret from `auto_login` since it identifies a Visitor (low privilege) rather than granting panel access (high privilege) |
| config | JSON | endpoint URLs, scopes, etc. |
| secret_hash | varchar(255) | hashed API key / auto-login hash secret |
| is_global | boolean | |
| created_at | datetime | |

**`integration_merchant`** — pivot: id, integration_id, merchant_id

> Each pivot is a dedicated 2-3 column table rather than one shared polymorphic table, so MySQL can enforce real foreign-key constraints on every relationship — a handful of small pivot tables is still simple, and it keeps referential integrity that a generic polymorphic pivot would give up.

### Audit Log

**`audit_log`** — high volume, index-heavy
| column | type | notes |
|---|---|---|
| id | bigint PK | |
| merchant_id | bigint, nullable | |
| user_id | bigint, nullable | |
| category | varchar(50) | indexed |
| message | varchar(255) | |
| status_code | smallint | indexed |
| status_message | varchar(100) | |
| source | varchar(50) | e.g. `web`, `api`, `system` |
| ip_address | varchar(45) | |
| created_at | datetime | indexed; candidate for monthly partitioning once volume grows |

### Settings

**`setting`** — key/value, avoids a wide sparse table
| id, merchant_id (nullable=global), `key`, value (JSON/text), updated_at, updated_by |

### Sessions / Auth

**`session`**
| id, user_id, token_hash, ip, user_agent, last_activity_at, expires_at, created_at |

> **Decision:** default auth mode for v1 is **server-side session** — every request validates `token_hash` against this table. Build the auth layer behind an interface so it can switch to **JWT + refresh token** later, toggled by Super Admin via a `setting` flag (`auth_mode` = `session` | `jwt`), without touching call sites. `last_activity_at` is bumped on authenticated requests (throttled, e.g. at most once/minute, to avoid write amplification) and drives the idle-logout in §6.

### 4.1 ID Strategy: internal integer vs external UUID

- Internal DB foreign keys and joins always use the fast `bigint` PK.
- Any ID that leaves the trust boundary — shown in a URL, returned by/accepted from the REST API, used in a file download link, or part of the auto-login flow — uses the `uuid` column instead, never the raw incrementing id.
- No 2FA is required for this system (confirmed). Login is username/email + password only.

---

## 5. Setup Wizard (Super Admin first run)

Triggered on first access when no `setting.key = 'setup_complete'` row exists.

1. **Environment checklist** (AJAX-checked, live pass/fail per item)
   - Node.js version (for Next.js build/runtime)
   - Go binary present / correct version
   - MySQL reachable
   - Redis reachable (optional but recommended — warn if missing, don't block)
   - Write permission on the uploads directory
   - *(Correction from your original note: PHP is not part of this stack since the backend is Go — XAMPP is only used locally to supply Apache/MySQL. No PHP runtime check is needed unless you plan a PHP component.)*
2. **Database configuration** — host, port, db name, db user, db password. Pre-fills sensible defaults (e.g. db name `livechat`, user `livechat_user`, generated password) but every field is editable. "Test Connection" (AJAX) before allowing "Run Migration."
3. **Site configuration** — Site title, timezone, base URL, local storage path, and (per your dev setup) app port / WebSocket port fields since you're running under XAMPP locally. These values are written to `.env` and read at runtime — never hardcoded anywhere in the codebase — so moving from XAMPP localhost today to a live server/domain later is purely a config change (new `.env` on the new box, or re-running this wizard there), not a code change.
4. **Super Admin account** — username, email, password, confirm password.
5. **Finish** — writes `.env`, runs migrations, sets `setup_complete`, redirects to login. Wizard cannot be re-run once complete (would need a deliberate "reset" super-admin-only tool later, not part of v1).

---

## 6. UI/UX Conventions

### 6.0 Design Principle: Built for Non-Technical Users

**Assume Admin and Agent have zero IT background.** Every screen must be usable without a manual — plain language, no jargon, no raw config/JSON exposed, generous tooltips/helper text, sensible defaults everywhere. This is a hard constraint on every section below, not a nice-to-have:

- Anywhere this document describes a JSON shape (`condition`, `trigger`, `flow`, `widget_config`, etc.) — that's the **storage format only**. The Admin/Agent-facing UI is a guided builder (dropdowns, plain-English sentences, toggles, templates) that reads/writes that JSON; they never see or edit it directly. Treat "here's the JSON shape" and "here's the UI" as two separate design problems from now on — this doc has mostly only specified the former so far and needs the latter added alongside it, screen by screen.
- Words like "JSON", "node", "flow graph", "webhook", "API", "integration", "condition/operator" are **implementation vocabulary**, not UI copy. The UI uses plain terms instead (e.g. "step", "rule", "connect to your bot", "when this happens...").
- Prefer a handful of ready-made templates/presets an Admin can pick and tweak over a blank canvas they have to build from scratch.
- The **only** place this constraint relaxes is where the *actual user* is technical by definition: the Setup Wizard (§5, Super Admin/dev team only) and the raw side of Settings > Integration (API keys/webhook URLs, typically handled by the merchant's own IT contact or your dev team, not the day-to-day Admin). Even there, keep it as guided and copy-paste-friendly as possible (generated snippets, a "Test Connection" button) rather than assuming protocol knowledge.

- **Confirmation popups (modal)** required before: Logout, Set Status, Save Changes, Create, Delete, Bulk Edit apply, Export.
- **Idle session timeout**: 2 hours of inactivity force-logs-out the user. Shown via a custom app modal (not a native browser `alert`/`confirm`) with a single "OK" button; clicking it logs out and redirects to the login page. Driven by `session.last_activity_at` server-side (so it holds even if a client-side timer is tampered with) plus a client-side idle timer that resets on user interaction.
- **Back button** never uses browser history — every page defines its parent route explicitly (breadcrumb/parent map), and the in-app back control navigates there via router push.
- **Sidebar**: sticky top-left, logo at top, nav items `flex: 1` + scrollable with a minimal/thin custom scrollbar, Logout pinned to the bottom.
  - Menu items: **Chats**, **Overview** (Admin/Super Admin only), **Settings**, **Profile**.
- **Login → Landing**: after login, land on a dashboard showing: Online Agents, Entries, Records, Traffic, Merchants/Brands, Active Chats, Bot Chats. (Admin/Super Admin only — Agents land directly on Chat List, per §8.)

### 6.1 Settings tabs
As of Phase 10, a categorized left-nav (not flat tabs), regrouped from
research into Zendesk/Zoho Desk/Tidio/Tawk.to's admin IA (§12): **General**
(General, Files, Embed, System) · **Conversations** (Canned Messages) ·
**Automation** (Greeting Rules, Flows — the Bot flow builder, relabeled per
industry-standard vocabulary while the engine underneath stays the same) ·
**Team** (Users) · **Merchants** · **Customers** (Visitors) ·
**Integrations & Logs** (Integration, Audit Logs — Admin/Super Admin only).
Every item still has its own standalone route (§6.6-style "not a modal, not
just a tab") — this is a left-nav reorganization, not a URL change.

### 6.2 Profile
- Any user: change Display Name, Password.
- Admin: force-change Agent passwords, set Agent account status.
- Super Admin: same, for every account across every merchant.

### 6.3 Automation
- Initial prompt message shown to a visitor on opening the widget, with conditional rules (e.g. by page, time, merchant).
- Auto-response rules.
- Canned messages, each optionally targeted to a specific merchant.
- **UI (per §6.0)**: a plain sentence builder, not a form full of "field/operator/value" fields — e.g. *"If visitor's page **contains** [ __ ], and the time is **between** [ __ ] and [ __ ], show this message: [ __ ]"* — dropdowns for the field and comparison, plain-language options only (no exposed operator codes like `contains`/`between`), reads/writes the `condition` JSON in §4 underneath.

### 6.4 Bot
- **Create Trigger** as the entry point (e.g. chat starts, keyword seen, condition matched — deeper conditions than Automation's, can combine multiple fields with AND/OR).
- From the trigger, chain multiple **tools/actions**, each proceeding to the next: send message, ask a question and capture the answer, branch on a condition, call an external integration, hand off to an Agent, close the chat, etc. — see the `flow` node-graph shape under `bot_flow` in §4.
- A **"Preview Flow"** button renders the current flow as a flow-chart diagram straight from its node graph, so anyone building it can visually verify the trigger → branches → actions before activating it — no separate diagram to hand-maintain, it's generated from the same JSON that runs the flow.
- The "AI"/third-party step is one of those action types (`call_integration`) — a **generic outbound REST call**, not a hardcoded vendor SDK. `bot_flow` holds an `integration_id` (nullable FK → `integration`, type `webhook`) that supplies the endpoint URL + auth secret + optional custom headers (a property of the connection itself, set once); when that node runs, the Go backend POSTs `{chatUuid, visitorUuid, visitor: {displayName, phone, email}, variables}` to the endpoint. The step's own config optionally names a variable to save the response into (`saveResponseAs`, extracted via an optional dot/index `responsePath` like `data.answer` — defaults to the response's top-level `message` field if unset) and whether to also post it back into the chat as a `bot` message (`sendAsMessage`, default yes). Saving into a variable also sets `<variable>_ok` to `true`/`false` off the HTTP outcome, so a flow branches on success/failure using the **existing** `condition` step rather than any new branching mechanic. Swapping or adding a provider later is just adding a new `integration` row, no flow-engine changes.
- **Phase 10 depth** (scoped from studying Live Helper Chat's trigger/condition/AI-integration docs, see §12 for what wasn't adopted): `send_message` and `ask_question` message text supports `{{variable}}` interpolation with a fallback (`{{name || "there"}}`). The `condition` step upgraded from a single inline `{field, operator, value}` to the same `{logic, rules: [...]}` multi-rule AND/OR shape Automation's own page-URL/time-of-day conditions already use — evaluated against every captured variable plus two always-available built-ins, `visitor_tier` (§6.9.1) and `chat_duration_seconds`. `ask_question` gained optional regex validation (offered as plain-language presets — Numbers only/Email/Phone/Custom — never a bare regex field by default, per this section's own UI principle) with a retry limit and a "give up, skip to step..." target, instead of always accepting whatever the visitor typed. `call_integration` gained an opt-in "log request & response to Audit Logs" checkbox for debugging a connection — the single UX trick the Live Helper Chat research flagged as doing the most for flow-debugging there.
- **Considered and explicitly not built in Phase 10**: a keyword/text-match condition evaluated against "the message that started the chat" — Live Helper Chat supports this, but in this engine a bot flow's trigger is evaluated at chat-*creation* time, before the visitor has sent any message at all (the pre-chat form/passthrough/API-v1 paths never carry an initial free-text message), so there is no "starting message" to match against without first building deferred/lazy trigger evaluation (re-checking a flow's trigger against the visitor's actual first message once it arrives) — a real architecture change, not a config addition. Revisit only if a concrete flow needs it.
- This is a **structured step-by-step builder that emits a node graph**, not a free-form drag-and-drop canvas — a canvas-style editor is a much larger build and can be a later upgrade on top of the same underlying `flow` JSON if you want it down the line.
- **UI (per §6.0 — this is the section most at risk of feeling technical, so it's called out explicitly):**
  - Primary editing surface is a **linear, plain-language step list** ("Step 1: When a chat starts → Step 2: Send a message: '...' → Step 3: Ask a question: '...' → Step 4: If the answer is '...', go to Step 6, otherwise Step 5"), built by picking from a small set of big, clearly-labelled step buttons ("Send a message", "Ask a question", "Check something", "Connect to another system", "Hand over to a real person", "End the chat") — never a "type" dropdown of raw node names.
  - The flow-chart preview is a **read-only, secondary view** for double-checking the shape at a glance (useful once a flow has branches) — it is not where anyone builds or edits the flow, so it can use friendly step labels/icons instead of technical node IDs.
  - Ship a few **ready-made templates** ("Ask for name & email before connecting to an Agent", "Answer FAQs then hand off") an Admin can start from and tweak, instead of a blank flow.
  - "Connect to another system" (the `call_integration` step) is the one point where real technical setup (an endpoint URL, a secret) is unavoidable — keep that specific step's form minimal (URL + secret + a "Test Connection" button) and let the Super Admin/dev team set it up once per merchant; the Admin building the flow just picks from already-configured connections by name, never touches the URL/secret themselves.

### 6.5 Integration
- REST API — inbound (external systems call in) and outbound (webhooks out); planned to grow over time.
- B2B auto-login — dev team issues a merchant a unique hash key; their system can deep-link a user into the widget/panel pre-authenticated without re-entering credentials.
- **UI note (per §6.0)**: this tab is the one place raw technical fields (API keys, endpoint URLs, secrets) are genuinely unavoidable, and is expected to be set up by the merchant's own IT contact or your dev team, not the everyday Admin. Still keep it as friction-free as possible for that technical-but-not-your-team user: auto-generate copy-paste-ready values/snippets, a "Test Connection" button with a plain pass/fail result, and short plain-language descriptions of what each key/webhook actually does — not just a bare field name.

### 6.6 Users
- Admin manages Agents for their own merchant (Super Admin not visible to Admins).
- Super Admin sees and manages every account, every merchant.
- Create/Edit happens on a single dedicated page per user (not a modal).

### 6.7 Audit Logs
- Expected to be the largest table in the system — schema above indexes `category`, `status_code`, `user_id`, `created_at`.
- Filters: date range, category, user id, source, status code; plus keyword search and sorting.
- Row click → quick-view popup of the full log record.
- CSV export.
- Delete action on a row: **Super Admin only**.

### 6.8 Files
- Configurable allowed/disallowed formats, permissions.
- Two categories, matching the `file.purpose`/`chat_id` split in §4:
  1. Customer-uploaded during a chat — scoped exclusively to that chat.
  2. Agent/Admin-uploaded — reusable in Automation/Bot content.

### 6.9 Chat Assignment / Routing
- Per-merchant **`routing_mode`** setting (`manual` default, or `round_robin`) — System-level, Admin-configurable per their own merchant(s).
- **Manual** (default): a new chat lands as `pending`, `agent_id = NULL`, visible to every Agent assigned to that merchant in the shared Chat List; any of them can claim it, flipping it to `active` with themselves as PIC.
- **Round-robin**: the system assigns the chat immediately to the next Agent in rotation among that merchant's currently *online* Agents (tracked via the WebSocket presence used for the Online Agents stat), advancing `merchant.last_routed_agent_id` each time. Chat goes straight to `active`.
- Regardless of mode, an Admin (or Super Admin) can always manually reassign a chat's PIC — routing decides the *default* assignment, not a hard restriction.

### 6.9.1 Visitor Tiers (Normal/VIP)
- `visitor.tier` (`normal` default, or `vip`) is a property of the customer, not the chat — it persists across every future conversation with that identity.
- **Routing effect**: a **Normal** visitor keeps today's behavior unchanged (bot first if a matching flow exists, else `routing_mode`-based assignment). A **VIP** visitor skips the bot entirely and is routed directly to an Agent flagged `handles_vip` for that merchant (`user_merchant.handles_vip`) — regardless of the merchant's own `routing_mode` setting. If no VIP-flagged Agent is currently online, it falls back to the normal agent pool (§6.9) rather than leaving the VIP client waiting specifically for a dedicated Agent — they're still served promptly, just not by a VIP-specialist this time.
- **Setting the tier** — two trusted channels only, never a raw unsigned field a visitor could self-report:
  1. **Manual staff tagging** (always available): Admin/Agent marks a visitor VIP/Normal from Settings > Visitors.
  2. **The merchant's own site** ("the parked site"), via whichever trusted channel it's already using to talk to us: a `"tier":"vip"` claim inside the same HMAC-signed passthrough payload used for a logged-in visitor (§10.2), or a `tier` field on an API-key-authenticated `POST /api/v1/chats` call (§6.5). The standard: send `"vip"` for a VIP account, omit the field entirely for a normal one. Anything else is treated as no signal and never touches an existing tier — a same-session anonymous call can't silently downgrade a VIP a human staffer already set.
- **Visibility**: a gold "VIP" tag shows next to the customer's name in the Chat List and the conversation header — this is how staff see *why* a chat routed the way it did. No separate queue-priority/reordering mechanism exists (or is needed) since VIP already bypasses the shared queue via direct-to-agent routing.

### 6.10 Merchants
- **Super Admin**: creates/suspends Merchant records (`name`, `code`), and grants the first Admin(s) access to it via `user_merchant`. This is also where a brand-new customer gets onboarded — create the `merchant` row, create (or pick) its Admin, assign it.
- No automated invite email for v1 (consistent with the in-app-only notification decision, §9) — Super Admin sets the new Admin's initial password directly at creation time and communicates it out-of-band; the Admin can change it themselves afterward (§6.2), or Admin/Super Admin can force a change later.
- **Admin**: sees this tab scoped to only the merchant(s) they already hold, and can edit that merchant's own operational config — `routing_mode`, `widget_config` (branding, §10.4), `inactivity_timeout_minutes` (§10.5) — but cannot create a new Merchant or grant themselves access to one they don't have.

---

## 7. File Storage Strategy

Given the dev-on-XAMPP-now / self-hosted-or-Cloudflare-later path: build a small **storage driver interface** in Go (`Put`, `Get`, `Delete`, `URL`) with a **local-disk implementation** first (works identically on XAMPP and on a cPanel/self-hosted box). This keeps the door open to swap in an S3-compatible driver (MinIO or AWS S3) later purely as a config change, with zero changes to chat/automation/file business logic. No object storage needs to be stood up now.

---

## 8. Chat List (Agent's landing view)

Agents land directly on the Chat List (no dashboard).

- **Layout**: needs to read as neat and dense without feeling cramped — the list should fit within the viewport without page-level vertical scroll wherever practical, minimal wasted white space. Any horizontal overflow (e.g. a wide table on a narrow screen) scrolls **within its own container** (`overflow-x: auto` on that container), never causing the whole page to scroll sideways.
- Filter, keyword search, sorting required.
- Bulk select + bulk edit.
- Items-per-page: global default (Admin-configurable in Settings, suggested default 20) with a per-user override remembered on their account (`user.items_per_page`).
- Columns:
  1. Status dot (red/green) + Chat ID
  2. Customer display name (default "Visitor", falls back to visitor-provided name if given)
  3. Timestamp
  4. PIC — Agent display name + email
  5. Merchant/Brand
  6. Status — Active / Closed / Pending / Bot / etc.

---

## 9. Decisions Log

| Topic | Decision |
|---|---|
| Notifications | **In-app only** for v1 (new chat assigned, chat waiting too long, etc.) — delivered over the existing WebSocket connection, no email/SMTP infra needed yet. |
| Data retention | **Flexible, default 1 year.** Retention window per data type (`audit_log`, `message`, `file`) stored in `setting`, Super-Admin-adjustable shorter or longer, plus a manual "purge now" action. A scheduled job auto-purges anything past its window; nothing is silently kept forever by default. |
| Bot / AI | Generic outbound REST call via `integration`, not a fixed vendor SDK — see §6.4. |
| Auth mechanism | **Server-side session for v1**, built behind an interface so Super Admin can toggle to JWT + refresh token later via a `setting` flag — see §4 `session`. |
| Redis | **Confirmed** as part of the stack (WebSocket pub/sub across Go instances; also backs whichever auth mode is active). |
| Hosting | **VPS** (dev on local XAMPP, production on a VPS managed by your internal server team), **cPanel optional** — since it's self-hosted/internal, the Go binary runs as a persistent service either way; cPanel (if used) stays limited to DNS/mail/file-manager convenience, never fronting the WebSocket/Go process. |
| Bot flow complexity | **Structured step-by-step builder emitting a node graph** (Trigger → chained tools/actions, with branching/conditions), plus a generated flow-chart preview — see §6.4. Not a free-form drag-and-drop canvas for v1; that remains a possible later upgrade on the same `flow` JSON. |
| Chat routing | **Configurable per merchant** — `manual` claim-from-queue by default, `round_robin` auto-assign available as a toggle; Admin/Super Admin can always manually override the PIC regardless of mode. See §6.9. |
| Visitor identity | **Phone is the primary identity key**, email secondary, fingerprint last resort — see the identity-resolution note under `visitor` in §4 and the pre-chat form / manual merge tool in §10.3. |
| Password policy | **NIST-aligned**: minimum 10 characters, no forced composition rules (no mandatory uppercase/symbol), no periodic forced rotation, checked against a common/breached-password blocklist at signup/change. Admin's existing "force change password" action (§6.2) still covers ad-hoc resets. |
| Retention auto-purge mechanism | **In-process scheduler** — a daily ticker/cron goroutine inside the Go binary (e.g. via `robfig/cron`) sweeps `audit_log`/`message`/`file` rows past their `setting`-defined retention window (§9). No separate worker process or OS-level cron needed for v1; revisit only if purge volume ever becomes heavy enough to want isolating from the main API process. |
| CAPTCHA / stronger abuse protection | **Deferred by design**, not unresolved — rate limiting is the v1 baseline (§10.6); add CAPTCHA (e.g. Cloudflare Turnstile) only if real abuse is observed in production. |
| Phone-number-recycling edge case | **Accepted limitation for v1** — if a telco reassigns a retired/merged phone number to a genuinely different person, they'd resolve into the old (merged) visitor's identity chain. Standard CRM-merge behavior; revisit only if it actually surfaces. |
| Phase discipline | **Strict phase-by-phase order, no jumping ahead** — a later phase cannot start before the current one is complete. Revisiting an already-built phase for an enhancement or bug fix is always allowed and is not "jumping." See §11. |
| Port & domain flexibility | **Environment-driven config, not hardcoded** — app port, WebSocket port, and base URL/domain are read from `.env` at runtime (set via the Setup Wizard, §5). Same codebase runs unchanged on XAMPP localhost during dev and on the live server/domain later — switching environments is a config change only. |
| Merchant management UI | **Dedicated "Merchants" Settings tab** — Super Admin creates/suspends Merchant records and grants the first Admin(s) access; an Admin edits their own held merchant(s)' operational config (routing, branding, timeout) from the same tab. See §6.10. |
| Non-technical UX | **Design principle, applies everywhere**: assume Admin/Agent have zero IT background — plain language, guided builders/templates over raw config or JSON, no exposed jargon. Only the Setup Wizard (§5) and the raw side of Integration (§6.5) get to stay technical, since those users are inherently technical. See §6.0. |
| Bot message richness | **Quick-reply/structured content supported** — `message` gained a `type = quick_reply` value and a `metadata` JSON column for button options; a visitor's click posts back as an ordinary text message so the flow engine's `ask_question` node doesn't need special-case handling. See §4 `message` and §6.4. |
| Dashboard metric definitions | **Entries** = new chats started within the selected time range; **Records** = total known Visitor records, all-time; **Traffic** = total messages exchanged (Visitor+Agent+Bot) within the time range. See §9.1/§11 Phase 2. |

### 9.1 Remaining small design defaults (implementation detail, flag if you want it different)

- **Overview dashboard refresh**: "Online Agents" and "Active Chats" ride the existing WebSocket presence channel (already live, no extra cost). "Entries", "Records", "Traffic", "Bot Chats" are historical aggregates — refreshed via AJAX poll every ~60s rather than push, since they don't need sub-second accuracy.
- **REST API (Settings > Integration)**: versioned under `/api/v1/`, Bearer API-key auth for server-to-server calls; outbound webhook payloads get an HMAC signature so the receiving system can verify authenticity.
- **B2B auto-login token**: short-lived (~5 min), single-use, HMAC-signed token carrying `merchant_id` + external user reference + expiry + nonce; the nonce is recorded on first use to block replay. Consuming the link establishes a normal session — the token itself is never a long-lived credential.

## 10. Customer-Facing Chat Widget

### 10.1 Delivery & Embed
- A small `<script>` snippet placed on the merchant's website injects an **iframe** pointing at the widget app — isolates the widget's CSS/JS completely from the host page (the standard pattern: Intercom, Zendesk, Drift all do this).
- The widget is its own Next.js route (not a separate frontend stack) for v1 — reuses the existing team skillset and build pipeline. If load-time on the merchant's page ever becomes a concern, this can be swapped for a leaner dedicated bundle later without changing the backend contract.

### 10.2 Visitor Identity & Passthrough Auth
- **Anonymous visitor** → must complete the pre-chat form (§10.3) before starting a chat.
- **Logged-in visitor** (on the merchant's own site) → the merchant's backend signs a payload (name/email/phone/external id) using their `integration` (`type = widget_identity`) secret; the embed script carries that signed token; our backend verifies the signature server-side before trusting any of it. Never trust an unsigned passthrough payload — it's attacker-controlled JS on someone else's page.
- **Resuming a conversation**: both `StartChatHandler` and `CreateChatV1Handler` check for an existing chat (`status IN ('pending','active','bot')`) for the identity `visitor.Resolve` returns *before* inserting a new one — a `closed` chat still starts fresh, since that conversation is genuinely done. In practice this means: an **anonymous** visitor can only resume on the same browser/device, since there's no stable identity to resolve against elsewhere — the widget's `localStorage` session key is what makes same-device resumption work today, and that's an accepted limitation, not a bug. A **logged-in** visitor resumes anywhere (new device, cleared storage, different browser) because the merchant's own backend can always reissue a valid signed passthrough token for that same identity, and the widget then reconnects to their existing open chat instead of spawning a blank one.

### 10.3 Pre-chat Form & Visitor Merge
- Fields: **Phone number** (primary — normalized with country code), **Full Name** (→ `display_name`), **Email** (secondary, "for record").
- Identity resolution order and conflict handling: see the `visitor` table note in §4.
- **Admin can force-change a Visitor's email** (typo correction, etc.) — a normal edit action, logged in `audit_log`.
- **Manual merge tool** (Admin/Super Admin): handles the "visitor got a new phone number" case the automatic resolution can't — Admin picks a source and target Visitor record, confirms via the standard confirmation modal, and the system: reassigns every `chat.visitor_id` (and any `file.uploader_id` where `uploader_type = visitor`) from source → target, sets `visitor.merged_into_id` on the source row (kept, not deleted, for history), and writes an `audit_log` entry (`category = visitor_merge`). All of that visitor's old chats now show up under the one current identity.

### 10.4 Branding
- `merchant.widget_config` (JSON): accent color, logo/avatar (a `file.uuid` reference, `purpose = branding` — uploaded and validated through the same Files system as §6.8, not a raw pasted URL), corner position (bottom-left/right), default language. Covers the "make it look like our brand" need without a full theme editor.

### 10.5 Inactivity Timeout
- `merchant.inactivity_timeout_minutes` (default 30, Admin-configurable) — auto-closes a chat after this many minutes of no Visitor activity. Distinct from the 2-hour Agent idle-logout in §6, which is about the Agent's own panel session, not the chat itself.

### 10.6 Abuse Protection
- **v1 baseline: rate limiting only** — per-IP and per-visitor (phone/fingerprint) limits on chat-start and message-send at the Go layer. No CAPTCHA for v1 (kept frictionless for legitimate visitors); revisit per §12 if abuse actually shows up.

## 11. Build Phases / Roadmap

Build in this order — each phase ends in something genuinely demoable/testable before the next one starts, rather than one massive build. Phases 0–3 are strictly sequential (each depends on the previous one existing). Once Phase 3 is stable, Phases 4–6 don't depend on each other — with more than one developer, they can run in parallel; with one, do them in the order below. Phase 7 comes last regardless, since it hardens whatever's already built.

> **Hard rule:** work proceeds phase by phase, in order — never jump ahead to a later phase before the current one is complete. The only exception is going back to an **already-built** phase for an enhancement or bug fix; that's always allowed and doesn't count as "jumping."

| Phase | Focus | Key deliverables | Milestone — what "done" looks like |
|---|---|---|---|
| **0 — Foundations** | Plumbing, no user-visible features yet | Repo scaffold (Next.js+Tailwind+AntD, Go+Gin, MySQL, Redis wired up). Setup Wizard (§5). Core tables: `role`, `user`, `session`, `setting`, `merchant`, `user_merchant`, and `audit_log` **+ a logging helper called from everywhere else** (build this now — retrofitting audit logging into every later phase is much more painful than baking it in from day one). Auth (login, server-side session, 2-hour idle timeout modal, logout confirm modal). Global UI shell: sidebar, confirmation-modal component, the "back = parent route" convention, the AJAX fetch-wrapper pattern every later phase reuses. | Super Admin completes setup, logs in, sees an empty panel shell, idle-timeout/logout both work correctly. |
| **1 — Accounts, Merchants & Permissions** | The org structure everything else scopes to | Settings > Users (create/edit Admin & Agent accounts, force password reset, role + `user_merchant` assignment per the rules in §3). Settings > Merchants (§6.10) — Super Admin creates/suspends merchants and assigns the first Admin; full branding/routing config UI comes later once those features exist (Phase 3/4), this phase just needs the record + assignment working. Profile tab (self-service name/password). **RBAC enforced on every route from here on**, not bolted on later. | Super Admin onboards a new merchant + its first Admin; that Admin creates an Agent; permission boundaries (who sees/edits what) are provably correct. Still no chat. |
| **2 — Core Chat Engine (internal MVP)** | The product's core loop, before any public entry point exists | Tables: `chat`, `message`, `visitor` (full identity-resolution logic from §4, but fed by an internal test harness for now — the real pre-chat form comes in Phase 3). WebSocket hub, built with the Redis pub/sub abstraction from §2 from day one even though only one Go instance runs so far. Chat List (§8: filters, search, sort, bulk edit, items-per-page). Chat conversation view (AJAX+WebSocket send/receive, file attachments). Chat routing (§6.9: manual claim + round-robin). Overview dashboard (§6/§9.1) — Online Agents/Active Chats live via WebSocket presence, Entries/Records/Traffic/Bot Chats as periodic aggregates once §12's metric definitions are confirmed. | An Agent can pick up and hold a full real-time conversation with a manually-created test visitor, and it shows up correctly on the Admin's dashboard. |
| **3 — Customer-Facing Widget (§10)** | First point the product is live for real external users | Embed script + iframe delivery. Real pre-chat form wired to identity resolution (replaces Phase 2's test harness). Passthrough auth (`widget_identity`). Branding (`widget_config` — fills in the Merchants-tab config UI stubbed in Phase 1). Inactivity timeout. Rate limiting. Manual visitor-merge tool. | A real visitor opens the widget on a merchant's site and chats with an Agent, end to end. |
| **4 — Automation & Bot** | Configurable, no-code-for-the-Admin behavior | Automation rules with the plain-language sentence builder (§6.0/§6.3). Bot flow engine — plain-language step-list builder + read-only flow-chart preview, quick-reply messages (§6.0/§6.4). `integration` table + `call_integration` action for outbound bot/AI calls. | Admin configures a greeting/auto-response and builds a working bot flow (including handoff-to-Agent) without touching any code or JSON. |
| **5 — Compliance & Operational Depth** | The "boring but necessary" phase | Audit Logs UI (filters, keyword search, quick-view, CSV export, Super-Admin-only delete — logging itself started back in Phase 0). Files settings screen (format/permission rules). General/System settings tabs. Retention auto-purge job (§9). | The system is auditable, governable, and self-maintaining (old data ages out on its own). |
| **6 — Integrations & B2B** | Opening the platform up to external systems | REST API v1 (inbound, `api_key`). Outbound webhooks. B2B auto-login (`auto_login` token, panel deep-link). | A third-party/B2B system can integrate with the platform. |
| **7 — Scale-Out & Hardening** | Proving the "millions of users" target, not just functional completeness | Activate multi-instance Go behind the Redis pub/sub abstraction built in Phase 2 (actually run >1 instance). Load/perf testing. CAPTCHA/stronger abuse protection if real abuse has been observed (§12). Hosting migration (VPS/cPanel/Cloudflare) validated under real load. | The platform is load-tested at scale, not just working on one dev box. |
| **8 — UX Refinement & Embed Flexibility** | Post-roadmap polish requested directly, not part of the original phased build | Profile page restructure (2-column: Basic Information + Change Password). Settings' now-11-tab flat bar regrouped into a categorized left-nav. Files gets per-merchant rule overrides plus an actual file library/browser (previously rules-only). Embed snippet generation gains a Widget-vs-Page type toggle and a per-merchant allowed-origins list. postMessage-based runtime theming so a host site can re-skin the widget live. | Day-to-day screens scale better with the amount of settings/data the earlier phases produced, and a merchant's own dev team can embed and re-theme the widget without needing a redeploy from us. |
| **9 — Visitor Tiers, Deeper Bot Integrations & Visual Redesign** | Another post-roadmap round requested directly | Normal/VIP visitor tiering with direct-to-VIP-agent routing that bypasses the bot (§6.9.1). Bot flow `call_integration` gains a richer outgoing payload (visitor identity), per-connection custom headers, and response→variable capture with path extraction so a flow can branch on a call's outcome using the existing `condition` step (§6.4/§12). A bold, colorful `ConfigProvider` theme across the whole app (previously unstyled default AntD). | VIP customers reach a dedicated agent immediately instead of the bot queue; a bot flow can call a real API, read its answer, and branch on success/failure; the app has an actual visual identity instead of stock AntD. |
| **10 — Grand Redesign, Theme Switcher, Settings IA & Bot Depth** | The client wasn't satisfied with Phase 9's colorful redesign; another post-roadmap round | A craft-first, Vercel-benchmarked design system replacing Phase 9's theme (neutral monochrome + one restrained accent, same information density). A 3-preset (Light/Dark/Violet) Appearance picker in Profile, account-persisted (`user.theme_preference`). Settings regrouped per Zendesk/Zoho/Tidio/Tawk.to research — Automation promoted to its own top-level group, Team/Merchants/Customers split apart (no URL changes). Bot flow depth from Live Helper Chat research: `{{variable}}` templating in messages, multi-rule AND/OR conditions, built-in `visitor_tier`/`chat_duration_seconds` condition fields, regex-validated `ask_question` with a retry limit, a keyword-based flow-entry condition, and optional audit-log debug visibility on `call_integration`. | The panel reads as a premium, deliberately-designed product rather than stock AntD with a color swapped in; Settings' structure matches what an admin coming from Zendesk/Zoho/Tidio would expect; a bot flow can validate input, branch on richer conditions, and speak in templated sentences without a code change. |

## 12. Open Items — to discuss next

- **CAPTCHA / stronger abuse protection** — still deferred per the Decisions Log (§9): rate limiting (now Redis-backed as of Phase 7) is the v1 baseline, add CAPTCHA (e.g. Cloudflare Turnstile) only once real abuse is actually observed in production. Not built yet, not an oversight.
- **Real VPS / multi-machine load validation** — Phase 7 proved the multi-instance mechanics (WS delivery, presence, rate-limit sharing) correctly with two Go processes on one dev box sharing one MySQL/Redis, and a load test on that same box sustained ~280 req/s with zero errors. What hasn't been proven: real capacity on separate hardware behind a real reverse proxy/load balancer, under real geographically-distributed traffic. Needs an actual VPS (or two) to validate — out of reach from a local dev sandbox.
- **Bot flow `call_integration` — features deliberately not adopted from Live Helper Chat's REST API** (researched in Phase 9): SSE/streaming responses (for token-by-token AI replies), poll-until-match (for a slow async job on the other end), `{foreach}`-style response templating, per-file-type upload content routing, and message-content redaction (`sensitive_` prefix). All are real, legitimate features in that product, but none have current usage pressure here and each would meaningfully grow the (currently synchronous, no-worker-queue) bot engine's complexity. Revisit if a real flow actually needs one of these rather than building ahead of demand.
- **Bot flow engine — architectural-investment items from the same Live Helper Chat research, still deferred** (Phase 10): streaming and background/async `call_integration` execution (both require a job queue + a persistent push channel, incompatible with today's synchronous in-request interpreter), a function/tool-calling loop (needs the interpreter to pause mid-flow and resume with accumulated context, not a single linear pass), bot composition/includes (needs flow-referencing-flow semantics and invalidation/versioning), and RAG/vector-store retrieval as a flow primitive (even Live Helper Chat treats this as bespoke per-provider wiring, not a platform primitive). Phase 10 took only the "cheap wins that fit the current architecture" half of that research (templating, multi-rule conditions, validated retry-limited questions, keyword entry conditions, audit-log debug visibility) — see §11's Phase 10 row.
- **Settings IA — patterns researched (Zendesk/Zoho Desk/Tidio/Tawk.to) but deliberately not built in Phase 10**: Business Hours/Availability schedules, SLA policies, a dedicated Assignment/Routing-rules surface (distinct from Greeting Rules/Bot), a Knowledge Base/Help Center for end-customer self-service, custom roles/permission sets (ours stays a fixed Super Admin/Admin/Agent 3-tier), a settings-wide search bar, and a Settings landing/dashboard page. All are real patterns every one of those four competitors has in some form; none are in scope until there's concrete demand.

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

LiveChat is a multi-tenant customer-care/livechat platform (Merchants/Brands → Admins → Agents, serving anonymous or identified "Visitors" through an embeddable widget). The full product spec — roles, real-time architecture, DB schema, UI/UX conventions, decisions log, and the phased build roadmap — lives in **`overview.md`** at the repo root. That file is the source of truth for *why* things are built the way they are; read it before making product/architecture decisions, not just this file. It is a living document: §9 is decisions already made, §11 is the phase-by-phase roadmap, §12 (always last) is open questions.

**Phase discipline is a hard rule** (overview.md §11): work proceeds phase by phase in the order listed there. Never jump ahead to a later phase's features before the current one is complete. Going back to an already-built phase for a bug fix or enhancement is always fine. If unsure what phase the repo is currently on, check what tables/handlers/pages already exist against the §11 table rather than guessing from a commit message.

Repo layout: `frontend/` (Next.js) and `backend/` (Go) are independent modules in one repo — no shared package/build step between them, they talk over HTTP only.

## Commands

### Frontend (`frontend/`)
```
npm install       # first time / after pulling dependency changes
npm run dev       # Next.js dev server, http://localhost:3000
npm run build
npm run start
npm run lint      # eslint (eslint-config-next core-web-vitals + typescript)
```
There is no frontend test runner configured yet.

### Backend (`backend/`)
```
go run ./cmd/server      # starts the API on :8080 (APP_PORT)
go build ./...
go vet ./...
```
There are no `_test.go` files yet — if you add backend logic that needs coverage, this is a green field (no existing test conventions to match).

Backend config is entirely env-driven (`backend/internal/config/config.go`), loaded via `godotenv` from a `.env` at the repo root the Setup Wizard writes (or process env / defaults if absent — a fresh checkout with no `.env` must still boot far enough to serve the wizard). Never hardcode a port, host, or path anywhere else in the codebase; add a new `Config` field instead.

### Running the app locally
Both processes run side by side (no Docker/Makefile in the repo): MySQL + Redis reachable (XAMPP locally per overview.md), `go run ./cmd/server` for the API, `npm run dev` in `frontend/` for the UI. On first run, hitting the frontend redirects into the Setup Wizard (`/setup`), which drives `/api/setup/*` to test the DB, run migrations (`backend/migrations/*.sql`, applied in filename order), and create the first Super Admin — see `backend/internal/handlers/setup.go`.

## Architecture

### Request flow: same-origin proxy, not CORS
The browser only ever talks to the Next.js origin. `frontend/next.config.ts` rewrites `/api/*` to the Go backend (`BACKEND_URL` env var, default `http://localhost:8080`). This is deliberate: it avoids `SameSite=None`/cross-origin cookie handling for the session cookie, and mirrors how production sits behind one reverse-proxy domain. `frontend/src/lib/api.ts` (`apiFetch`/`apiGet`/`apiPost`/`apiPatch`/`apiDelete`) is the single fetch wrapper every screen uses — plain JSON in/out, `credentials: 'include'`, no other place should call `fetch` directly for app data.

### Auth & RBAC (backend)
- Server-side session, not JWT (overview.md §4/§9 — a `setting` flag can toggle to JWT later, but build behind the current interface, don't preempt it).
- `backend/internal/session/session.go`: opaque random token, only its SHA-256 hash (`token_hash`) is ever persisted; cookie name is `session_token` (`backend/internal/middleware/auth.go`).
- The 2-hour idle timeout (overview.md §6.0) is enforced **server-side** in `session.Validate`, not just by the client-side timer — `last_activity_at` slides forward on activity, throttled to once/minute to avoid write amplification. The frontend's `AuthContext` heartbeats `/api/auth/me` every 60s purely so the `IdleTimeoutModal` shows up promptly; it isn't what actually logs anyone out.
- `middleware.RequireAuth` sets `user_id`/`role` on the Gin context; `middleware.RequireRole(...)` must run after it. RBAC is enforced per-route in `cmd/server/main.go`, never left to the frontend to hide a button and call it done.
- Password policy is NIST-aligned: minimum 10 chars, no forced composition/rotation (checked in every handler that sets a password — see the repeated `len(password) < 10` checks rather than a shared validator; if you add another password-setting endpoint, match that inline check, don't invent a different rule).

### appstate: DB connection can flip from nil to live mid-process
`backend/internal/appstate/appstate.go` holds the `*sql.DB` behind a `sync.RWMutex`. Before the Setup Wizard finishes, `state.DB()` is `nil` and `requireDB` middleware (in `main.go`) 503s any route that needs it. `handlers.FinishHandler` calls `state.SetDB(conn)` after running migrations, so the API works immediately post-setup with **no process restart**. Any new handler that touches the DB must go through `state.DB()` at call time, not capture the connection once at startup.

### Multi-tenancy model
Single shared MySQL database; `merchant_id` scopes single-owner rows, and many-to-many cases (which Admin/Agent can access which merchants) go through the `user_merchant` pivot table rather than a JSON list or separate schema-per-tenant. `backend/internal/handlers/users.go`'s `resolveTarget`/`actorMerchantIDs`/`merchantsByUserID` helpers are where the scoping rule ("an Admin can only see/manage Agents sharing one of the Admin's own merchants, and can never grant a merchant it doesn't hold itself") is centralized — every user-mutating endpoint funnels through `resolveTarget` so that rule lives in one place. Extend those helpers rather than re-deriving the check inline in a new handler.

### ID strategy: internal bigint vs external uuid
Every table has an internal `bigint` PK used for joins/FKs, plus a `uuid CHAR(36)` used anywhere the ID crosses a trust boundary (URLs, API responses/requests, file links). Handlers accept/return `uuid` in JSON and resolve to internal `id` via a `SELECT ... WHERE uuid = ?` before touching FKs (see `merchant.go`, `users.go`). Never expose the raw incrementing `id` externally — this is a stated anti-enumeration rule, not a style preference.

### Migrations
`backend/migrations/*.sql`, run in filename order by `db.RunMigrations`. Every statement must be idempotent (`CREATE TABLE IF NOT EXISTS`, `INSERT ... ON DUPLICATE KEY UPDATE`) because the Setup Wizard's "Run Migration" step can be retried after a partial failure, and `RunMigrations` has no per-file "already applied" tracking of its own. When adding a new migration file, name it so it sorts after the existing ones (`0002_...`, `0003_...`) and keep it re-run-safe.

### Audit logging
`backend/internal/audit.Log` writes one `audit_log` row and only logs-to-stderr on failure — it must never bubble an error up and fail the action that triggered it. Every state-changing handler calls this already; match that pattern (actor, category, human-readable message, status code, source, IP) when adding a new mutation rather than skipping it, since audit coverage was intentionally built in from Phase 0 rather than retrofitted later.

### Frontend conventions
- **Back button never uses browser history.** `frontend/src/lib/routes.ts` maps every route to an explicit parent (exact match, then longest-prefix match); add an entry here whenever a new page is introduced, or its back button silently falls back to `/dashboard`.
- **Confirmation modal**: `confirmAction` in `frontend/src/components/modals/confirm.tsx` is the one place `Modal.confirm` is configured — required before logout/save/delete/bulk-apply/export (overview.md §6.0). Don't hand-rolled a `Modal.confirm` call elsewhere.
- **Idle-timeout modal** (`IdleTimeoutModal.tsx`) is a custom AntD modal, never a native `alert`/`confirm`, single "OK" button, not closable/dismissable — it's mounted once in the root layout and driven by `AuthContext`'s `sessionExpired` flag.
- Route groups: `frontend/src/app/(app)/*` is the authenticated shell (`layout.tsx` there redirects to `/login` if `useAuth()` has no user); `/login` and `/setup` sit outside it.
- Settings is tab-based (`(app)/settings/page.tsx` + one component per tab in `components/settings/`) rather than separate routes per tab; create/edit pages for Users/Merchants are dedicated full pages under `settings/users/[uuid]`, `settings/users/new`, etc. (overview.md §6.6: "not a modal"), not modals.
- Stack: Next.js App Router + TypeScript, Tailwind for layout/utility styling, Ant Design for components. `@/*` resolves to `frontend/src/*`.

### Real-time (not yet built)
overview.md §2 specifies AJAX for all actions plus a single WebSocket connection per logged-in user/visitor for live message push, with a Redis pub/sub abstraction from day one so it survives a move to multiple Go instances. None of this exists in the codebase yet (still Phase 0/1 per the roadmap) — when it's built, follow that abstraction rather than adding polling or a single-instance-only hub.

## Other things in the repo
- `scripts/telegram/` — standalone Node scripts (send/poll) for a Telegram notification bridge used during development sessions; unrelated to the LiveChat product itself, no shared code with `frontend`/`backend`.
- `frontend/CLAUDE.md` imports `frontend/AGENTS.md` via `@AGENTS.md`.

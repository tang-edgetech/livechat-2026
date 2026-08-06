-- Manual availability toggle (Sidebar "Online"/"Offline") — a self-set
-- preference independent of whether the user actually has a live
-- WebSocket connection (ws/hub.go + internal/presence still track that
-- separately); routing.go intersects both before treating an Agent as
-- reachable. Default 'online' so existing/new accounts behave exactly as
-- before this column existed.
ALTER TABLE user ADD COLUMN IF NOT EXISTS manual_status ENUM('online','offline') NOT NULL DEFAULT 'online';

-- 'enquiry' is a brand-new chat that landed with nobody able to answer it
-- (no Agent reachable, no Bot flow configured) — the widget shows a
-- lead-capture form instead of a live "waiting for an agent" screen, and
-- an operator later promotes it to 'active' via the Invite action
-- (see ClaimChatHandler's sibling, InviteChatHandler, in chat.go).
ALTER TABLE chat MODIFY COLUMN status ENUM('active','pending','closed','bot','enquiry') NOT NULL DEFAULT 'pending';

export type ChatSummary = {
  uuid: string;
  visitor_name: string;
  visitor_uuid: string;
  visitor_tier: "normal" | "vip";
  merchant_name: string;
  merchant_uuid: string;
  agent_name: string | null;
  agent_email: string | null;
  status: "active" | "pending" | "closed" | "bot";
  started_at: string;
  last_message_at: string | null;
};

export type ChatMessage = {
  id: number;
  uuid?: string;
  chat_uuid?: string;
  sender_type: "visitor" | "agent" | "bot" | "system";
  body: string;
  type: "text" | "file" | "system" | "quick_reply";
  metadata?: string | null;
  created_at: string;
};

export const STATUS_COLOR: Record<ChatSummary["status"], string> = {
  active: "success",
  pending: "warning",
  closed: "default",
  bot: "processing",
};

// Messages arrive from two independent channels — the sender's own POST
// response (optimistic append) and the WebSocket push to everyone else —
// with no ordering guarantee between them. That's most visible right
// after a bot flow's ask_question: the engine can publish the *next*
// bot message over WS before the visitor's own POST response for their
// answer has resolved, so a plain append can land it out of order. Sort
// by id (a real DB insertion order) and dedupe every time, rather than
// trusting arrival order.
export function appendMessage(prev: ChatMessage[], next: ChatMessage): ChatMessage[] {
  if (prev.some((m) => m.id === next.id)) return prev;
  return [...prev, next].sort((a, b) => a.id - b.id);
}

// A Canned Message / Automation greeting inserted with "Insert as HTML"
// carries that flag through into the sent message's metadata (same
// convention already used for quick_reply options and file attachments)
// so a plain-text message still renders exactly as it always has.
export function messageIsHtml(message: ChatMessage): boolean {
  if (!message.metadata) return false;
  try {
    return Boolean(JSON.parse(message.metadata).isHtml);
  } catch {
    return false;
  }
}

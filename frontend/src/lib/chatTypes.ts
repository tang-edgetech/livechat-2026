export type ChatSummary = {
  uuid: string;
  visitor_name: string;
  visitor_uuid: string;
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

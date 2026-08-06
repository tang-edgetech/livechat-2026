export type AutomationRule = {
  id: number;
  name: string;
  condition: string | null;
  message: string;
  is_global: boolean;
  is_active: boolean;
  is_html: boolean;
  merchant_uuid: string | null;
};

export type CannedMessage = {
  id: number;
  title: string;
  body: string;
  is_global: boolean;
  is_html: boolean;
  merchant_uuid: string | null;
};

export type WebhookIntegration = {
  id: number;
  name: string;
  events?: string[];
};

export type WebhookIntegrationDetail = {
  id: number;
  name: string;
  url: string;
  isGlobal: boolean;
  merchantUuid: string | null;
  events?: string[];
  headers?: Record<string, string>;
};

export type ApiKey = {
  id: number;
  name: string;
  merchant_name: string;
  created_at: string;
};

export type BotFlow = {
  id: number;
  name: string;
  trigger: string;
  flow: string;
  integration_id: number | null;
  is_global: boolean;
  is_active: boolean;
  merchant_uuid: string | null;
};

// Storage shapes (overview.md §4) — the UI never shows this JSON
// directly, only the plain-language builder that reads/writes it.
export type ConditionRule = { field: string; operator: string; value: unknown };
export type ConditionSet = { logic: "and" | "or"; rules: ConditionRule[] };

export type FlowNode = {
  id: string;
  type: "send_message" | "ask_question" | "condition" | "call_integration" | "set_variable" | "delay" | "handoff_to_agent" | "close_chat";
  config: Record<string, unknown>;
  next?: string;
  branches?: { true?: string; false?: string };
  // Canvas layout only — the engine (botengine/engine.go) never reads
  // this, it just walks next/branches. Optional so a flow authored
  // before the visual canvas editor still loads (falls back to an
  // auto-layout position).
  position?: { x: number; y: number };
};

// mode "steps" (default/omitted) runs the node graph below via nodes/
// entry; "ai_passthrough" ignores them entirely and forwards every
// visitor message to `passthrough.integrationId` instead (item 2c).
export type PassthroughConfig = { integrationId?: number; greeting?: string; logToAuditLog?: boolean };
export type FlowDef = { nodes: FlowNode[]; entry: string; mode?: "steps" | "ai_passthrough"; passthrough?: PassthroughConfig };
// audience gates which visitor tier this flow is even eligible for
// (overview.md item 3) — omitted/"normal" is the default so existing
// flows keep never firing for a VIP visitor unless deliberately opted in.
export type TriggerDef = { type: "chat_start"; conditions: ConditionSet; audience?: "normal" | "vip" | "both" };

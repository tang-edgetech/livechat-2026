export type AutomationRule = {
  id: number;
  name: string;
  condition: string | null;
  message: string;
  is_global: boolean;
  is_active: boolean;
  merchant_uuid: string | null;
};

export type CannedMessage = {
  id: number;
  title: string;
  body: string;
  is_global: boolean;
  merchant_uuid: string | null;
};

export type WebhookIntegration = {
  id: number;
  name: string;
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
};

export type FlowDef = { nodes: FlowNode[]; entry: string };
export type TriggerDef = { type: "chat_start"; conditions: ConditionSet };

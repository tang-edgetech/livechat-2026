export type StaffUser = {
  uuid: string;
  display_name: string;
  email: string;
  role: "super_admin" | "admin" | "agent";
  status: "active" | "inactive" | "suspended";
  created_at: string;
  created_by_name: string | null;
  merchants: { uuid: string; name: string; handles_vip: boolean }[];
};

export type Merchant = {
  uuid: string;
  name: string;
  code: string;
  status: "active" | "suspended";
};

export type WidgetConfig = {
  accentColor?: string;
  corner?: "bottom-left" | "bottom-right";
  logoFileUuid?: string;
  language?: string;
  // Domains allowed to postMessage a runtime theme override into the
  // widget iframe (overview.md §6.5) — also the same trust boundary a
  // "page" iframe embed is expected to be hosted under.
  allowedOrigins?: string[];
};

export type MerchantDetail = {
  uuid: string;
  name: string;
  code: string;
  status: "active" | "suspended";
  routing_mode: "manual" | "round_robin";
  widget_config: string | null;
  inactivity_timeout_minutes: number;
  has_widget_identity: boolean;
  has_auto_login: boolean;
};

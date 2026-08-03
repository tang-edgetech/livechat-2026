export type StaffUser = {
  uuid: string;
  display_name: string;
  username: string;
  email: string;
  role: "super_admin" | "admin" | "agent";
  status: "active" | "inactive" | "suspended";
  merchants: { uuid: string; name: string }[];
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

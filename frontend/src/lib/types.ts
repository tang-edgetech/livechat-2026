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

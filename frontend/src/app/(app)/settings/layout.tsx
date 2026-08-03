"use client";

import { Card, Col, Menu, Row } from "antd";
import type { MenuProps } from "antd";
import { usePathname, useRouter } from "next/navigation";

import { useAuth } from "@/context/AuthContext";
import { SETTINGS_GROUPS } from "@/components/settings/settingsSections";

// Persistent left-nav shared by every /settings/* page — real standalone
// routes per section (e.g. /settings/files) instead of a client-only tab
// switch, so a section is linkable/bookmarkable and the browser's own
// forward/back works within Settings.
export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { user } = useAuth();
  const isStaff = user?.role === "admin" || user?.role === "super_admin";

  const visibleGroups = SETTINGS_GROUPS.map((g) => ({ ...g, items: g.items.filter((i) => !i.staffOnly || isStaff) })).filter(
    (g) => g.items.length > 0,
  );

  // /settings/files -> "files"; /settings/merchants/<uuid> -> "merchants"
  // (prefix match highlights the right nav item even from a deep page).
  const activeKey = pathname.split("/")[2] ?? "general";

  const menuItems: MenuProps["items"] = visibleGroups.map((g) => ({
    key: g.key,
    label: g.label,
    type: "group",
    children: g.items.map((i) => ({ key: i.key, label: i.label })),
  }));

  return (
    <Row gutter={24}>
      <Col xs={24} md={6}>
        <Card size="small" styles={{ body: { padding: 0 } }}>
          <Menu
            mode="inline"
            selectedKeys={[activeKey]}
            onClick={({ key }) => router.push(`/settings/${key}`)}
            items={menuItems}
            style={{ border: "none" }}
          />
        </Card>
      </Col>
      <Col xs={24} md={18}>
        {children}
      </Col>
    </Row>
  );
}

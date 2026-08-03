"use client";

import { Card, Tabs } from "antd";

import { useAuth } from "@/context/AuthContext";
import { UsersTab } from "@/components/settings/UsersTab";
import { MerchantsTab } from "@/components/settings/MerchantsTab";
import { VisitorsTab } from "@/components/settings/VisitorsTab";
import { AutomationTab } from "@/components/settings/AutomationTab";
import { BotTab } from "@/components/settings/BotTab";
import { IntegrationTab } from "@/components/settings/IntegrationTab";
import { CannedMessagesTab } from "@/components/settings/CannedMessagesTab";
import { AuditLogsTab } from "@/components/settings/AuditLogsTab";
import { GeneralTab } from "@/components/settings/GeneralTab";
import { SystemTab } from "@/components/settings/SystemTab";
import { FilesTab } from "@/components/settings/FilesTab";

export default function SettingsPage() {
  const { user } = useAuth();
  const isStaff = user?.role === "admin" || user?.role === "super_admin";

  return (
    <Card>
      <Tabs
        items={[
          { key: "general", label: "General", children: <GeneralTab /> },
          { key: "canned-messages", label: "Canned Messages", children: <CannedMessagesTab /> },
          ...(isStaff
            ? [
                { key: "automation", label: "Automation", children: <AutomationTab /> },
                { key: "bot", label: "Bot", children: <BotTab /> },
                { key: "integration", label: "Integration", children: <IntegrationTab /> },
                { key: "users", label: "Users", children: <UsersTab /> },
                { key: "merchants", label: "Merchants", children: <MerchantsTab /> },
                { key: "visitors", label: "Visitors", children: <VisitorsTab /> },
                { key: "audit", label: "Audit Logs", children: <AuditLogsTab /> },
                { key: "system", label: "System", children: <SystemTab /> },
              ]
            : []),
          { key: "files", label: "Files", children: <FilesTab /> },
        ]}
      />
    </Card>
  );
}

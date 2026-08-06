import type { ComponentType, ReactNode } from "react";
import {
  ApiOutlined,
  CodeOutlined,
  ContactsOutlined,
  FileSearchOutlined,
  FolderOutlined,
  MessageOutlined,
  NotificationOutlined,
  PartitionOutlined,
  SettingOutlined,
  ShopOutlined,
  TeamOutlined,
  ThunderboltOutlined,
  ToolOutlined,
} from "@ant-design/icons";

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
import { EmbedTab } from "@/components/settings/EmbedTab";

export type SettingsItem = {
  key: string;
  label: string;
  description: string;
  icon: ReactNode;
  Component: ComponentType;
  staffOnly?: boolean;
  // Most sections are tables/builders that want the full content width;
  // the handful of plain forms (General, System) stay narrow so they
  // don't float lost in a football-field-wide panel.
  narrow?: boolean;
};
export type SettingsGroup = { key: string; label: string; icon: ReactNode; items: SettingsItem[] };

// Single source of truth for both the left-nav (settings/layout.tsx) and
// the standalone per-section routes (settings/[section]/page.tsx) — every
// item here is reachable at its own URL, e.g. /settings/files, not just a
// client-side tab switch.
export const SETTINGS_GROUPS: SettingsGroup[] = [
  {
    key: "general",
    label: "General",
    icon: <SettingOutlined />,
    items: [
      {
        key: "general",
        label: "General",
        description: "Site-wide name, timezone, and list defaults.",
        icon: <SettingOutlined />,
        Component: GeneralTab,
        narrow: true,
      },
      {
        key: "files",
        label: "Files",
        description: "Upload rules and the shared file library.",
        icon: <FolderOutlined />,
        Component: FilesTab,
      },
      {
        key: "embed",
        label: "Embed",
        description: "Get the snippet to put this chat on a website, and preview it live.",
        icon: <CodeOutlined />,
        Component: EmbedTab,
        staffOnly: true,
      },
      {
        key: "system",
        label: "System",
        description: "Retention windows for old data, and a manual purge.",
        icon: <ToolOutlined />,
        Component: SystemTab,
        staffOnly: true,
        narrow: true,
      },
    ],
  },
  {
    key: "conversations",
    label: "Conversations",
    icon: <MessageOutlined />,
    items: [
      {
        key: "canned-messages",
        label: "Canned Messages",
        description: "Ready-made replies your team can insert with one click while chatting.",
        icon: <MessageOutlined />,
        Component: CannedMessagesTab,
      },
    ],
  },
  {
    key: "automation-group",
    label: "Automation",
    icon: <ThunderboltOutlined />,
    items: [
      {
        key: "automation",
        label: "Greeting Rules",
        description:
          "Automatic messages: a greeting when a chat opens (targeted by page or time), or an instant auto-reply when a visitor's message contains a keyword.",
        icon: <NotificationOutlined />,
        Component: AutomationTab,
        staffOnly: true,
      },
      {
        key: "bot",
        label: "Flows",
        description: "Build a step-by-step bot that greets visitors, asks a few questions, and hands off to an Agent — no code.",
        icon: <PartitionOutlined />,
        Component: BotTab,
        staffOnly: true,
      },
    ],
  },
  {
    key: "team",
    label: "Team",
    icon: <TeamOutlined />,
    items: [
      {
        key: "users",
        label: "Users",
        description: "Create and manage Admin/Agent accounts and which merchants they can access.",
        icon: <TeamOutlined />,
        Component: UsersTab,
        staffOnly: true,
      },
    ],
  },
  {
    key: "merchants",
    label: "Merchants",
    icon: <ShopOutlined />,
    items: [
      {
        key: "merchants",
        label: "Merchants",
        description: "Manage the brands/businesses this platform serves.",
        icon: <ShopOutlined />,
        Component: MerchantsTab,
        staffOnly: true,
      },
    ],
  },
  {
    key: "customers",
    label: "Customers",
    icon: <ContactsOutlined />,
    items: [
      {
        key: "visitors",
        label: "Visitors",
        description:
          "Search for a visitor by name, phone, or email to correct their email, or merge two records that turned out to be the same person.",
        icon: <ContactsOutlined />,
        Component: VisitorsTab,
        staffOnly: true,
      },
    ],
  },
  {
    key: "integrations",
    label: "Integrations & Logs",
    icon: <ApiOutlined />,
    items: [
      {
        key: "integration",
        label: "Integration",
        description: "REST API keys and outbound webhook connections for external systems.",
        icon: <ApiOutlined />,
        Component: IntegrationTab,
        staffOnly: true,
      },
      {
        key: "audit",
        label: "Audit Logs",
        description: "A record of who did what, for accountability and troubleshooting.",
        icon: <FileSearchOutlined />,
        Component: AuditLogsTab,
        staffOnly: true,
      },
    ],
  },
];

export const ALL_SETTINGS_ITEMS: SettingsItem[] = SETTINGS_GROUPS.flatMap((g) => g.items);
export const DEFAULT_SETTINGS_SECTION = "general";

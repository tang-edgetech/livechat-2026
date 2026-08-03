import type { ComponentType } from "react";

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

export type SettingsItem = { key: string; label: string; Component: ComponentType; staffOnly?: boolean };
export type SettingsGroup = { key: string; label: string; items: SettingsItem[] };

// Single source of truth for both the left-nav (settings/layout.tsx) and
// the standalone per-section routes (settings/[section]/page.tsx) — every
// item here is reachable at its own URL, e.g. /settings/files, not just a
// client-side tab switch.
export const SETTINGS_GROUPS: SettingsGroup[] = [
  {
    key: "general",
    label: "General",
    items: [
      { key: "general", label: "General", Component: GeneralTab },
      { key: "files", label: "Files", Component: FilesTab },
      { key: "embed", label: "Embed", Component: EmbedTab, staffOnly: true },
      { key: "system", label: "System", Component: SystemTab, staffOnly: true },
    ],
  },
  {
    key: "conversations",
    label: "Conversations",
    items: [
      { key: "canned-messages", label: "Canned Messages", Component: CannedMessagesTab },
      { key: "automation", label: "Automation", Component: AutomationTab, staffOnly: true },
      { key: "bot", label: "Bot", Component: BotTab, staffOnly: true },
    ],
  },
  {
    key: "team",
    label: "Team & Merchants",
    items: [
      { key: "users", label: "Users", Component: UsersTab, staffOnly: true },
      { key: "merchants", label: "Merchants", Component: MerchantsTab, staffOnly: true },
      { key: "visitors", label: "Visitors", Component: VisitorsTab, staffOnly: true },
    ],
  },
  {
    key: "integrations",
    label: "Integrations & Logs",
    items: [
      { key: "integration", label: "Integration", Component: IntegrationTab, staffOnly: true },
      { key: "audit", label: "Audit Logs", Component: AuditLogsTab, staffOnly: true },
    ],
  },
];

export const ALL_SETTINGS_ITEMS: SettingsItem[] = SETTINGS_GROUPS.flatMap((g) => g.items);
export const DEFAULT_SETTINGS_SECTION = "general";

"use client";

import { useParams } from "next/navigation";
import { Typography } from "antd";

import { ALL_SETTINGS_ITEMS } from "@/components/settings/settingsSections";

export default function SettingsSectionPage() {
  const { section } = useParams<{ section: string }>();
  const match = ALL_SETTINGS_ITEMS.find((i) => i.key === section);

  if (!match) {
    return (
      <div className="rounded-lg border border-black/10 bg-white p-6 dark:border-white/10 dark:bg-neutral-900">
        <Typography.Paragraph type="secondary">That settings section doesn&apos;t exist.</Typography.Paragraph>
      </div>
    );
  }

  const Component = match.Component;
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-start gap-3">
        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-black/10 bg-black/[0.03] text-[17px] dark:border-white/10 dark:bg-white/[0.06]">
          {match.icon}
        </div>
        <div className="flex flex-col">
          <Typography.Title level={4} style={{ margin: 0 }}>
            {match.label}
          </Typography.Title>
          <Typography.Text type="secondary" style={{ fontSize: 13.5 }}>
            {match.description}
          </Typography.Text>
        </div>
      </div>

      <div
        className="rounded-lg border border-black/10 bg-white p-6 dark:border-white/10 dark:bg-neutral-900"
        style={match.narrow ? { maxWidth: 520 } : undefined}
      >
        <Component />
      </div>
    </div>
  );
}

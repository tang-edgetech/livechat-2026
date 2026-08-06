"use client";

import { useParams } from "next/navigation";
import { Typography } from "antd";

import { PageHeader } from "@/components/layout/PageHeader";
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
      <PageHeader icon={match.icon} title={match.label} description={match.description} />

      <div
        className="rounded-lg border border-black/10 bg-white p-6 dark:border-white/10 dark:bg-neutral-900"
        style={match.narrow ? { maxWidth: 520 } : undefined}
      >
        <Component />
      </div>
    </div>
  );
}

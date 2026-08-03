"use client";

import { useParams } from "next/navigation";
import { Card, Typography } from "antd";

import { ALL_SETTINGS_ITEMS } from "@/components/settings/settingsSections";

export default function SettingsSectionPage() {
  const { section } = useParams<{ section: string }>();
  const match = ALL_SETTINGS_ITEMS.find((i) => i.key === section);

  if (!match) {
    return (
      <Card>
        <Typography.Paragraph type="secondary">That settings section doesn&apos;t exist.</Typography.Paragraph>
      </Card>
    );
  }

  const Component = match.Component;
  return (
    <Card>
      <Component />
    </Card>
  );
}

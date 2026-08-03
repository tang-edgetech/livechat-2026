"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

import { DEFAULT_SETTINGS_SECTION } from "@/components/settings/settingsSections";

export default function SettingsIndexPage() {
  const router = useRouter();

  useEffect(() => {
    router.replace(`/settings/${DEFAULT_SETTINGS_SECTION}`);
  }, [router]);

  return null;
}

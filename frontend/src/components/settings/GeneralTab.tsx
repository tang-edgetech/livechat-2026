"use client";

import { useEffect, useState } from "react";
import { Button, Input, InputNumber, Space, Typography, message } from "antd";

import { apiGet, apiPatch, ApiError } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";

type SettingsMap = Record<string, string>;

// Site-wide config (overview.md §6.1 "General" tab) — visible to everyone
// so any staff member can see e.g. the configured site title, but only
// Super Admin can change it (settings are global, not per-merchant).
export function GeneralTab() {
  const { user } = useAuth();
  const canEdit = user?.role === "super_admin";

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [siteTitle, setSiteTitle] = useState("");
  const [timezone, setTimezone] = useState("");
  const [itemsPerPage, setItemsPerPage] = useState(20);

  function load() {
    setLoading(true);
    apiGet<{ settings: SettingsMap }>("/api/settings")
      .then((res) => {
        setSiteTitle(res.settings.site_title ?? "");
        setTimezone(res.settings.timezone ?? "");
        setItemsPerPage(Number(res.settings.items_per_page_default ?? 20));
      })
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial load on mount.
    load();
  }, []);

  async function save() {
    setSaving(true);
    try {
      await apiPatch("/api/settings", {
        site_title: siteTitle,
        timezone,
        items_per_page_default: String(itemsPerPage),
      });
      message.success("Settings saved");
      load();
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "Could not save settings");
    } finally {
      setSaving(false);
    }
  }

  if (loading) return null;

  return (
    <div>
      <Space orientation="vertical" style={{ width: "100%" }}>
        <div>
          <Typography.Text strong>Site title</Typography.Text>
          <Input value={siteTitle} onChange={(e) => setSiteTitle(e.target.value)} disabled={!canEdit} />
        </div>
        <div>
          <Typography.Text strong>Timezone</Typography.Text>
          <Input value={timezone} onChange={(e) => setTimezone(e.target.value)} disabled={!canEdit} placeholder="e.g. UTC, Asia/Kuala_Lumpur" />
        </div>
        <div>
          <Typography.Text strong>Default items per page</Typography.Text>
          <InputNumber
            style={{ width: "100%" }}
            min={5}
            max={200}
            value={itemsPerPage}
            onChange={(v) => setItemsPerPage(v ?? 20)}
            disabled={!canEdit}
          />
          <Typography.Paragraph type="secondary" style={{ marginTop: 4, marginBottom: 0 }}>
            Applies to the Chat List (overview.md §8); each agent can still override it for themselves.
          </Typography.Paragraph>
        </div>
        {canEdit ? (
          <Button type="primary" loading={saving} onClick={save}>
            Save
          </Button>
        ) : (
          <Typography.Text type="secondary">Only a Super Admin can change these values.</Typography.Text>
        )}
      </Space>
    </div>
  );
}

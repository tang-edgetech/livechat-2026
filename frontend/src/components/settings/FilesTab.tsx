"use client";

import { useEffect, useState } from "react";
import { Button, Card, Input, InputNumber, Space, Typography, message } from "antd";

import { apiGet, apiPatch, ApiError } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";

type SettingsMap = Record<string, string>;

// Files tab (overview.md §6.8) — configurable allowed/disallowed formats
// and a max size, enforced server-side on every upload (both the staff
// chat attachment path and the visitor widget path share the same
// validateFileRules check in backend/internal/handlers/chat.go).
export function FilesTab() {
  const { user } = useAuth();
  const canEdit = user?.role === "super_admin";

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [allowedExtensions, setAllowedExtensions] = useState("");
  const [maxSizeMb, setMaxSizeMb] = useState(20);

  function load() {
    setLoading(true);
    apiGet<{ settings: SettingsMap }>("/api/settings")
      .then((res) => {
        setAllowedExtensions(res.settings.file_allowed_extensions ?? "");
        setMaxSizeMb(Number(res.settings.file_max_size_mb ?? 20));
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
        file_allowed_extensions: allowedExtensions,
        file_max_size_mb: String(maxSizeMb),
      });
      message.success("File settings saved");
      load();
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "Could not save settings");
    } finally {
      setSaving(false);
    }
  }

  if (loading) return null;

  return (
    <Card loading={loading} style={{ maxWidth: 480 }}>
      <Space orientation="vertical" style={{ width: "100%" }}>
        <div>
          <Typography.Text strong>Allowed file extensions</Typography.Text>
          <Input
            value={allowedExtensions}
            onChange={(e) => setAllowedExtensions(e.target.value)}
            disabled={!canEdit}
            placeholder="e.g. jpg, png, pdf — leave blank to allow any type"
          />
          <Typography.Paragraph type="secondary" style={{ marginTop: 4, marginBottom: 0 }}>
            Comma-separated, no dot needed. Applies to both chat attachments and Agent/Admin uploads.
          </Typography.Paragraph>
        </div>
        <div>
          <Typography.Text strong>Maximum file size (MB)</Typography.Text>
          <InputNumber style={{ width: "100%" }} min={1} max={500} value={maxSizeMb} onChange={(v) => setMaxSizeMb(v ?? 20)} disabled={!canEdit} />
        </div>
        {canEdit ? (
          <Button type="primary" loading={saving} onClick={save}>
            Save
          </Button>
        ) : (
          <Typography.Text type="secondary">Only a Super Admin can change these values.</Typography.Text>
        )}
      </Space>
    </Card>
  );
}

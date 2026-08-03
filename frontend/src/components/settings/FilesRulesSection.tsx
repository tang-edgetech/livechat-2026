"use client";

import { useEffect, useState } from "react";
import { Button, Card, Input, InputNumber, Select, Space, Tag, Typography, message } from "antd";

import { apiGet, apiPatch, ApiError } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";
import type { Merchant } from "@/lib/types";

type SettingsMap = Record<string, string>;
type FileRules = { allowedExtensions: string; maxSizeMb: string; hasOverride: boolean };

// Files rules (overview.md §6.8) — a global default plus an optional
// per-merchant override, enforced server-side on every upload
// (validateFileRules in backend/internal/handlers/chat.go checks the
// merchant's own override first, falling back to this global default).
export function FilesRulesSection() {
  const { user } = useAuth();
  const canEdit = user?.role === "super_admin";

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [allowedExtensions, setAllowedExtensions] = useState("");
  const [maxSizeMb, setMaxSizeMb] = useState(20);

  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [merchantUuid, setMerchantUuid] = useState<string | undefined>(undefined);
  const [override, setOverride] = useState<FileRules | null>(null);
  const [overrideLoading, setOverrideLoading] = useState(false);
  const [overrideSaving, setOverrideSaving] = useState(false);
  const [overrideExtensions, setOverrideExtensions] = useState("");
  const [overrideMaxSizeMb, setOverrideMaxSizeMb] = useState(20);

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
    if (canEdit) {
      apiGet<{ merchants: Merchant[] }>("/api/merchants").then((res) => setMerchants(res.merchants));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
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

  function loadOverride(uuid: string) {
    setOverrideLoading(true);
    apiGet<FileRules>(`/api/merchants/${uuid}/file-rules`)
      .then((res) => {
        setOverride(res);
        setOverrideExtensions(res.allowedExtensions);
        setOverrideMaxSizeMb(Number(res.maxSizeMb) || 0);
      })
      .finally(() => setOverrideLoading(false));
  }

  function selectMerchant(uuid: string) {
    setMerchantUuid(uuid);
    loadOverride(uuid);
  }

  async function saveOverride() {
    if (!merchantUuid) return;
    setOverrideSaving(true);
    try {
      await apiPatch(`/api/merchants/${merchantUuid}/file-rules`, {
        allowedExtensions: overrideExtensions,
        maxSizeMb: String(overrideMaxSizeMb),
      });
      message.success("Override saved for this merchant");
      loadOverride(merchantUuid);
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "Could not save override");
    } finally {
      setOverrideSaving(false);
    }
  }

  async function clearOverride() {
    if (!merchantUuid) return;
    setOverrideSaving(true);
    try {
      await apiPatch(`/api/merchants/${merchantUuid}/file-rules`, { clearOverride: true });
      message.success("Reverted to the global default");
      loadOverride(merchantUuid);
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "Could not clear override");
    } finally {
      setOverrideSaving(false);
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <Card loading={loading} title="Global default" style={{ maxWidth: 480 }}>
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
              Comma-separated, no dot needed. Applies to both chat attachments and Agent/Admin uploads, for any
              merchant without its own override below.
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

      {canEdit && (
        <Card title="Per-merchant override" style={{ maxWidth: 480 }}>
          <Space orientation="vertical" style={{ width: "100%" }}>
            <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
              Exempt or tighten the rules for one specific merchant — leave a field blank/zero to not limit that
              merchant at all.
            </Typography.Paragraph>
            <Select
              placeholder="Choose a merchant"
              style={{ width: "100%" }}
              value={merchantUuid}
              onChange={selectMerchant}
              options={merchants.map((m) => ({ value: m.uuid, label: m.name }))}
            />
            {overrideLoading && <Typography.Text type="secondary">Loading…</Typography.Text>}
            {override && !overrideLoading && (
              <>
                <Tag color={override.hasOverride ? "blue" : "default"}>
                  {override.hasOverride ? "Custom override set" : "Using global default"}
                </Tag>
                <div>
                  <Typography.Text strong>Allowed file extensions</Typography.Text>
                  <Input
                    value={overrideExtensions}
                    onChange={(e) => setOverrideExtensions(e.target.value)}
                    placeholder="Leave blank to allow any type"
                  />
                </div>
                <div>
                  <Typography.Text strong>Maximum file size (MB)</Typography.Text>
                  <InputNumber style={{ width: "100%" }} min={0} max={500} value={overrideMaxSizeMb} onChange={(v) => setOverrideMaxSizeMb(v ?? 0)} />
                  <Typography.Paragraph type="secondary" style={{ marginTop: 4, marginBottom: 0 }}>
                    0 means unlimited for this merchant.
                  </Typography.Paragraph>
                </div>
                <Space>
                  <Button type="primary" loading={overrideSaving} onClick={saveOverride}>
                    Save override
                  </Button>
                  <Button danger loading={overrideSaving} onClick={clearOverride} disabled={!override.hasOverride}>
                    Revert to global default
                  </Button>
                </Space>
              </>
            )}
          </Space>
        </Card>
      )}
    </div>
  );
}

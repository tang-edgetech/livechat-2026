"use client";

import { useEffect, useState } from "react";
import { Button, InputNumber, Space, Typography, message } from "antd";

import { apiGet, apiPatch, apiPost, ApiError } from "@/lib/api";
import { confirmAction } from "@/components/modals/confirm";
import { useAuth } from "@/context/AuthContext";

type SettingsMap = Record<string, string>;
type PurgeReport = { audit_logs_deleted: number; messages_deleted: number; files_deleted: number };

// Retention windows + manual purge (overview.md §9: "Flexible, default 1
// year... plus a manual purge now action"). A daily in-process ticker
// (backend/internal/retention) already runs this automatically; this tab
// just exposes the knobs and an on-demand trigger.
export function SystemTab() {
  const { user } = useAuth();
  const canEdit = user?.role === "super_admin";

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [purging, setPurging] = useState(false);
  const [auditDays, setAuditDays] = useState(365);
  const [messageDays, setMessageDays] = useState(365);
  const [fileDays, setFileDays] = useState(365);

  function load() {
    setLoading(true);
    apiGet<{ settings: SettingsMap }>("/api/settings")
      .then((res) => {
        setAuditDays(Number(res.settings.retention_audit_log_days ?? 365));
        setMessageDays(Number(res.settings.retention_message_days ?? 365));
        setFileDays(Number(res.settings.retention_file_days ?? 365));
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
        retention_audit_log_days: String(auditDays),
        retention_message_days: String(messageDays),
        retention_file_days: String(fileDays),
      });
      message.success("Retention settings saved");
      load();
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "Could not save settings");
    } finally {
      setSaving(false);
    }
  }

  function purgeNow() {
    confirmAction({
      title: "Purge now?",
      content: "Permanently deletes any audit log, message, or file older than its configured retention window. This can't be undone.",
      okText: "Purge",
      danger: true,
      onConfirm: async () => {
        setPurging(true);
        try {
          const res = await apiPost<{ report: PurgeReport }>("/api/settings/purge-now");
          const r = res.report;
          message.success(
            `Purged ${r.audit_logs_deleted} audit log${r.audit_logs_deleted === 1 ? "" : "s"}, ${r.messages_deleted} message${r.messages_deleted === 1 ? "" : "s"}, ${r.files_deleted} file${r.files_deleted === 1 ? "" : "s"}.`,
          );
        } catch (err) {
          message.error(err instanceof ApiError ? err.message : "Purge failed");
        } finally {
          setPurging(false);
        }
      },
    });
  }

  if (loading) return null;

  return (
    <div>
      <Space orientation="vertical" style={{ width: "100%" }}>
        <Typography.Paragraph type="secondary">
          A daily background job enforces these windows on its own; use &quot;Purge Now&quot; to run it
          immediately instead of waiting.
        </Typography.Paragraph>
        <div>
          <Typography.Text strong>Audit log retention (days)</Typography.Text>
          <InputNumber style={{ width: "100%" }} min={1} value={auditDays} onChange={(v) => setAuditDays(v ?? 365)} disabled={!canEdit} />
        </div>
        <div>
          <Typography.Text strong>Message retention (days)</Typography.Text>
          <InputNumber style={{ width: "100%" }} min={1} value={messageDays} onChange={(v) => setMessageDays(v ?? 365)} disabled={!canEdit} />
        </div>
        <div>
          <Typography.Text strong>File retention (days)</Typography.Text>
          <InputNumber style={{ width: "100%" }} min={1} value={fileDays} onChange={(v) => setFileDays(v ?? 365)} disabled={!canEdit} />
        </div>
        {canEdit ? (
          <Space>
            <Button type="primary" loading={saving} onClick={save}>
              Save
            </Button>
            <Button danger loading={purging} onClick={purgeNow}>
              Purge Now
            </Button>
          </Space>
        ) : (
          <Typography.Text type="secondary">Only a Super Admin can change these values.</Typography.Text>
        )}
      </Space>
    </div>
  );
}

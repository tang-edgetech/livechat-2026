"use client";

import { useEffect, useState } from "react";
import { Alert, Button, Card, Input, Select, Space, Typography, message } from "antd";
import { useParams } from "next/navigation";

import { apiGet, apiPatch, apiPost, ApiError } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";
import { confirmAction } from "@/components/modals/confirm";
import type { MerchantDetail, WidgetConfig } from "@/lib/types";

export default function EditMerchantPage() {
  const { uuid } = useParams<{ uuid: string }>();
  const { user } = useAuth();
  const isSuperAdmin = user?.role === "super_admin";

  const [merchant, setMerchant] = useState<MerchantDetail | null>(null);
  const [routingMode, setRoutingMode] = useState<"manual" | "round_robin">("manual");
  const [config, setConfig] = useState<WidgetConfig>({});
  const [timeoutMinutes, setTimeoutMinutes] = useState(30);
  const [saving, setSaving] = useState(false);
  const [newSecret, setNewSecret] = useState<string | null>(null);
  const [generating, setGenerating] = useState(false);
  const [newAutoLoginSecret, setNewAutoLoginSecret] = useState<string | null>(null);
  const [generatingAutoLogin, setGeneratingAutoLogin] = useState(false);

  function load() {
    apiGet<MerchantDetail>(`/api/merchants/${uuid}`).then((m) => {
      setMerchant(m);
      setRoutingMode(m.routing_mode);
      setTimeoutMinutes(m.inactivity_timeout_minutes);
      try {
        setConfig(m.widget_config ? JSON.parse(m.widget_config) : {});
      } catch {
        setConfig({});
      }
    });
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [uuid]);

  function save() {
    confirmAction({
      title: "Save changes?",
      onConfirm: async () => {
        setSaving(true);
        try {
          await apiPatch(`/api/merchants/${uuid}`, {
            routingMode,
            widgetConfig: JSON.stringify(config),
            inactivityTimeoutMinutes: timeoutMinutes,
          });
          message.success("Merchant updated");
          load();
        } catch {
          message.error("Could not save changes");
        } finally {
          setSaving(false);
        }
      },
    });
  }

  function generateSecret() {
    confirmAction({
      title: "Generate a new widget identity secret?",
      content: "Any existing secret for this merchant stops working immediately.",
      okText: "Generate",
      onConfirm: async () => {
        setGenerating(true);
        try {
          const res = await apiPost<{ secret: string }>(`/api/merchants/${uuid}/widget-identity`);
          setNewSecret(res.secret);
          load();
        } catch (err) {
          message.error(err instanceof ApiError ? err.message : "Could not generate secret");
        } finally {
          setGenerating(false);
        }
      },
    });
  }

  function generateAutoLoginSecret() {
    confirmAction({
      title: "Generate a new auto-login secret?",
      content: "Any existing secret for this merchant stops working immediately — the B2B partner's own backend will need the new one.",
      okText: "Generate",
      onConfirm: async () => {
        setGeneratingAutoLogin(true);
        try {
          const res = await apiPost<{ secret: string }>(`/api/merchants/${uuid}/auto-login`);
          setNewAutoLoginSecret(res.secret);
          load();
        } catch (err) {
          message.error(err instanceof ApiError ? err.message : "Could not generate secret");
        } finally {
          setGeneratingAutoLogin(false);
        }
      },
    });
  }

  if (!merchant) return null;

  const embedSnippet = `<script src="${typeof window !== "undefined" ? window.location.origin : ""}/embed.js" data-merchant-code="${merchant.code}" async></script>`;

  return (
    <div className="flex flex-col gap-6">
      <Card title="Merchant">
        <Typography.Text strong>{merchant.name}</Typography.Text>{" "}
        <Typography.Text type="secondary">({merchant.code})</Typography.Text>
      </Card>

      <Card title="Chat routing">
        <Space orientation="vertical">
          <Select
            value={routingMode}
            style={{ width: 220 }}
            onChange={setRoutingMode}
            options={[
              { value: "manual", label: "Manual — Agents claim from queue" },
              { value: "round_robin", label: "Round robin — auto-assign" },
            ]}
          />
        </Space>
      </Card>

      <Card title="Branding">
        <Space orientation="vertical" style={{ width: "100%", maxWidth: 360 }}>
          <div>
            <Typography.Text>Accent color</Typography.Text>
            <Input
              type="color"
              value={config.accentColor || "#1677ff"}
              onChange={(e) => setConfig({ ...config, accentColor: e.target.value })}
              style={{ width: 80 }}
            />
          </div>
          <div>
            <Typography.Text>Widget corner</Typography.Text>
            <Select
              style={{ width: "100%" }}
              value={config.corner || "bottom-right"}
              onChange={(v) => setConfig({ ...config, corner: v })}
              options={[
                { value: "bottom-right", label: "Bottom right" },
                { value: "bottom-left", label: "Bottom left" },
              ]}
            />
          </div>
          <div>
            <Typography.Text>Default language</Typography.Text>
            <Input
              placeholder="en"
              value={config.language || ""}
              onChange={(e) => setConfig({ ...config, language: e.target.value })}
            />
          </div>
        </Space>
      </Card>

      <Card title="Inactivity timeout">
        <Space>
          <Input
            type="number"
            min={1}
            value={timeoutMinutes}
            onChange={(e) => setTimeoutMinutes(Number(e.target.value))}
            style={{ width: 100 }}
            suffix="minutes"
          />
        </Space>
      </Card>

      <Button type="primary" loading={saving} onClick={save} style={{ width: 160 }}>
        Save changes
      </Button>

      <Card title="Embed on your website">
        <Typography.Paragraph type="secondary">
          Paste this snippet into your site&apos;s HTML, just before the closing <code>&lt;/body&gt;</code> tag.
        </Typography.Paragraph>
        <Input.TextArea value={embedSnippet} readOnly rows={2} />
      </Card>

      {isSuperAdmin && (
        <Card title="Logged-in visitor identity (technical)">
          <Typography.Paragraph type="secondary">
            For merchants who want their already-logged-in website visitors to skip the chat form, their own
            backend signs a payload with this secret. Set up by your dev team, not the everyday Admin.
          </Typography.Paragraph>
          <Space orientation="vertical" style={{ width: "100%" }}>
            <Typography.Text>
              Status: {merchant.has_widget_identity ? "configured" : "not configured"}
            </Typography.Text>
            <Button loading={generating} onClick={generateSecret}>
              {merchant.has_widget_identity ? "Regenerate secret" : "Generate secret"}
            </Button>
            {newSecret && (
              <Alert
                type="warning"
                showIcon
                title="Copy this now — it won't be shown again"
                description={<Input.TextArea value={newSecret} readOnly rows={2} />}
              />
            )}
          </Space>
        </Card>
      )}

      {isSuperAdmin && (
        <Card title="B2B auto-login (technical)">
          <Typography.Paragraph type="secondary">
            Lets a trusted partner system deep-link one of this merchant&apos;s own staff straight into the panel,
            already signed in. Their own backend signs a short-lived token with this secret and sends their user to{" "}
            <code>/api/auto-login?merchant={merchant.code}&amp;token=...</code>. Set up by your dev team.
          </Typography.Paragraph>
          <Space orientation="vertical" style={{ width: "100%" }}>
            <Typography.Text>Status: {merchant.has_auto_login ? "configured" : "not configured"}</Typography.Text>
            <Button loading={generatingAutoLogin} onClick={generateAutoLoginSecret}>
              {merchant.has_auto_login ? "Regenerate secret" : "Generate secret"}
            </Button>
            {newAutoLoginSecret && (
              <Alert
                type="warning"
                showIcon
                message="Copy this now — it won't be shown again"
                description={<Input.TextArea value={newAutoLoginSecret} readOnly rows={2} />}
              />
            )}
          </Space>
        </Card>
      )}
    </div>
  );
}

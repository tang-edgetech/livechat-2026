"use client";

import { useEffect, useState } from "react";
import { Button, Card, Select, message } from "antd";

import { apiGet, apiPatch, ApiError } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";
import { EmbedSection } from "@/components/settings/EmbedSection";
import type { Merchant, MerchantDetail, WidgetConfig } from "@/lib/types";

// A direct, discoverable home for "how do I put this chat on my
// website" (overview.md §6.5/§10.1) — the snippet itself was previously
// only reachable by drilling into Settings > Merchants > pick one > edit.
export function EmbedTab() {
  const { user } = useAuth();
  const canEdit = user?.role === "admin" || user?.role === "super_admin";

  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [merchantUuid, setMerchantUuid] = useState<string | undefined>(undefined);
  const [detail, setDetail] = useState<MerchantDetail | null>(null);
  const [config, setConfig] = useState<WidgetConfig>({});
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    apiGet<{ merchants: Merchant[] }>("/api/merchants").then((res) => {
      setMerchants(res.merchants);
      if (res.merchants.length === 1) {
        selectMerchant(res.merchants[0].uuid);
      }
    });
  }, []);

  function selectMerchant(uuid: string) {
    setMerchantUuid(uuid);
    setLoading(true);
    apiGet<MerchantDetail>(`/api/merchants/${uuid}`)
      .then((m) => {
        setDetail(m);
        try {
          setConfig(m.widget_config ? JSON.parse(m.widget_config) : {});
        } catch {
          setConfig({});
        }
      })
      .finally(() => setLoading(false));
  }

  async function save() {
    if (!merchantUuid || !detail) return;
    setSaving(true);
    try {
      await apiPatch(`/api/merchants/${merchantUuid}`, {
        routingMode: detail.routing_mode,
        widgetConfig: JSON.stringify(config),
        inactivityTimeoutMinutes: detail.inactivity_timeout_minutes,
      });
      message.success("Embed settings saved");
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "Could not save");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <Select
        placeholder="Choose a merchant"
        style={{ maxWidth: 320 }}
        value={merchantUuid}
        onChange={selectMerchant}
        options={merchants.map((m) => ({ value: m.uuid, label: m.name }))}
      />

      {detail && (
        <Card loading={loading}>
          <EmbedSection code={detail.code} config={config} onConfigChange={setConfig} />
          {canEdit && (
            <Button type="primary" loading={saving} onClick={save} style={{ marginTop: 16 }}>
              Save
            </Button>
          )}
        </Card>
      )}
    </div>
  );
}

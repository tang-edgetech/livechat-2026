"use client";

import { useEffect, useState } from "react";
import { Button, Card, Checkbox, Input, Select, Space, Table, Typography, message } from "antd";
import { DeleteOutlined, PlusOutlined } from "@ant-design/icons";

import { apiDelete, apiGet, apiPost } from "@/lib/api";
import { confirmAction } from "@/components/modals/confirm";
import type { WebhookIntegration } from "@/lib/automationTypes";
import type { Merchant } from "@/lib/types";

// The one place raw technical fields (URL, secret) are unavoidable
// (overview.md §6.5) — set up by Super Admin/dev team once; the Bot flow
// builder only ever picks a connection by name (see BotFlowBuilder).
export function IntegrationTab() {
  const [items, setItems] = useState<WebhookIntegration[]>([]);
  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [isGlobal, setIsGlobal] = useState(false);
  const [merchantUuid, setMerchantUuid] = useState<string | undefined>(undefined);
  const [submitting, setSubmitting] = useState(false);
  const [testing, setTesting] = useState<number | null>(null);

  function load() {
    setLoading(true);
    apiGet<{ integrations: WebhookIntegration[] }>("/api/integrations")
      .then((res) => setItems(res.integrations))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial load on mount.
    load();
    apiGet<{ merchants: Merchant[] }>("/api/merchants").then((res) => setMerchants(res.merchants));
  }, []);

  async function submit() {
    if (!name.trim() || !url.trim()) {
      message.error("Please fill in a name and endpoint URL.");
      return;
    }
    if (!isGlobal && !merchantUuid) {
      message.error("Choose a merchant, or mark this as global.");
      return;
    }
    setSubmitting(true);
    try {
      await apiPost("/api/integrations", { name, url, isGlobal, merchantUuid: isGlobal ? undefined : merchantUuid });
      message.success("Connection created");
      setCreating(false);
      setName("");
      setUrl("");
      load();
    } catch {
      message.error("Could not create connection");
    } finally {
      setSubmitting(false);
    }
  }

  async function test(id: number) {
    setTesting(id);
    try {
      const res = await apiPost<{ ok: boolean; message: string }>(`/api/integrations/${id}/test`);
      if (res.ok) {
        message.success(res.message);
      } else {
        message.error(res.message);
      }
    } finally {
      setTesting(null);
    }
  }

  function remove(id: number) {
    confirmAction({
      title: "Delete this connection?",
      content: "Any bot flow using it will stop being able to reach it.",
      okText: "Delete",
      danger: true,
      onConfirm: async () => {
        await apiDelete(`/api/integrations/${id}`);
        load();
      },
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <Typography.Paragraph type="secondary">
        Connections to outside systems (like an AI/chatbot service) that a Bot flow can call. This is technical
        setup, usually done once by your dev team — once created, anyone building a Bot flow just picks it by name.
      </Typography.Paragraph>

      {!creating ? (
        <div>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreating(true)}>
            Create Connection
          </Button>
        </div>
      ) : (
        <Card title="New connection">
          <Space orientation="vertical" style={{ width: "100%", maxWidth: 480 }}>
            <Input placeholder="Name (e.g. Our AI Assistant)" value={name} onChange={(e) => setName(e.target.value)} />
            <Input placeholder="Endpoint URL (https://...)" value={url} onChange={(e) => setUrl(e.target.value)} />
            <Checkbox checked={isGlobal} onChange={(e) => setIsGlobal(e.target.checked)}>
              Apply to all merchants
            </Checkbox>
            {!isGlobal && (
              <Select
                placeholder="Merchant"
                style={{ width: "100%" }}
                value={merchantUuid}
                onChange={setMerchantUuid}
                options={merchants.map((m) => ({ value: m.uuid, label: m.name }))}
              />
            )}
            <Typography.Text type="secondary">
              A secret is generated automatically and sent as a Bearer token on every call, so the other system can
              verify the request came from us.
            </Typography.Text>
            <Space>
              <Button onClick={() => setCreating(false)}>Cancel</Button>
              <Button type="primary" loading={submitting} onClick={submit}>
                Save
              </Button>
            </Space>
          </Space>
        </Card>
      )}

      <Table
        rowKey="id"
        loading={loading}
        dataSource={items}
        columns={[
          { title: "Name", dataIndex: "name" },
          {
            title: "Actions",
            key: "actions",
            render: (_, r) => (
              <Space>
                <Button size="small" loading={testing === r.id} onClick={() => test(r.id)}>
                  Test Connection
                </Button>
                <Button type="text" danger icon={<DeleteOutlined />} onClick={() => remove(r.id)} />
              </Space>
            ),
          },
        ]}
      />
    </div>
  );
}

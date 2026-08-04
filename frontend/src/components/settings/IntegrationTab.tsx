"use client";

import { useEffect, useState } from "react";
import { Alert, Button, Card, Checkbox, Input, Select, Space, Table, Typography, message } from "antd";
import { DeleteOutlined, EditOutlined, PlusOutlined } from "@ant-design/icons";

import { apiDelete, apiGet, apiPatch, apiPost, ApiError } from "@/lib/api";
import { confirmAction } from "@/components/modals/confirm";
import type { ApiKey, WebhookIntegrationDetail, WebhookIntegration } from "@/lib/automationTypes";
import type { Merchant } from "@/lib/types";

const WEBHOOK_EVENTS = [
  { value: "chat.created", label: "A new chat starts" },
  { value: "message.received", label: "A visitor sends a message" },
  { value: "chat.closed", label: "A chat is closed" },
];

// The one place raw technical fields (URL, secret, API keys) are
// unavoidable (overview.md §6.5) — set up by Super Admin/dev team once;
// the Bot flow builder only ever picks a webhook connection by name (see
// BotFlowBuilder), and a B2B partner's own backend is the one that calls
// in with the API key or signs an auto-login deep link (see the
// merchant edit page for that secret).
export function IntegrationTab() {
  const [items, setItems] = useState<WebhookIntegration[]>([]);
  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [secret, setSecret] = useState("");
  const [isGlobal, setIsGlobal] = useState(false);
  const [merchantUuid, setMerchantUuid] = useState<string | undefined>(undefined);
  const [events, setEvents] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [testing, setTesting] = useState<number | null>(null);
  const [loadingDetail, setLoadingDetail] = useState(false);

  const [apiKeys, setApiKeys] = useState<ApiKey[]>([]);
  const [apiKeysLoading, setApiKeysLoading] = useState(true);
  const [creatingApiKey, setCreatingApiKey] = useState(false);
  const [apiKeyName, setApiKeyName] = useState("");
  const [apiKeyMerchantUuid, setApiKeyMerchantUuid] = useState<string | undefined>(undefined);
  const [submittingApiKey, setSubmittingApiKey] = useState(false);
  const [newApiKey, setNewApiKey] = useState<string | null>(null);

  function load() {
    setLoading(true);
    apiGet<{ integrations: WebhookIntegration[] }>("/api/integrations")
      .then((res) => setItems(res.integrations))
      .finally(() => setLoading(false));
  }

  function loadApiKeys() {
    setApiKeysLoading(true);
    apiGet<{ apiKeys: ApiKey[] }>("/api/api-keys")
      .then((res) => setApiKeys(res.apiKeys))
      .finally(() => setApiKeysLoading(false));
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial load on mount.
    load();
    loadApiKeys();
    apiGet<{ merchants: Merchant[] }>("/api/merchants").then((res) => setMerchants(res.merchants));
  }, []);

  function startCreate() {
    setEditingId(null);
    setName("");
    setUrl("");
    setSecret("");
    setIsGlobal(false);
    setMerchantUuid(undefined);
    setEvents([]);
    setCreating(true);
  }

  async function startEdit(id: number) {
    setEditingId(id);
    setCreating(true);
    setLoadingDetail(true);
    try {
      const detail = await apiGet<WebhookIntegrationDetail>(`/api/integrations/${id}`);
      setName(detail.name);
      setUrl(detail.url);
      setSecret("");
      setIsGlobal(detail.isGlobal);
      setMerchantUuid(detail.merchantUuid ?? undefined);
      setEvents(detail.events ?? []);
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "Could not load connection");
      setCreating(false);
    } finally {
      setLoadingDetail(false);
    }
  }

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
      const payload = { name, url, secret, isGlobal, merchantUuid: isGlobal ? undefined : merchantUuid, events };
      if (editingId) {
        await apiPatch(`/api/integrations/${editingId}`, payload);
        message.success("Connection updated");
      } else {
        await apiPost("/api/integrations", payload);
        message.success("Connection created");
      }
      setCreating(false);
      setEditingId(null);
      setName("");
      setUrl("");
      setEvents([]);
      load();
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "Could not save connection");
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

  async function submitApiKey() {
    if (!apiKeyName.trim() || !apiKeyMerchantUuid) {
      message.error("Please fill in a name and choose a merchant.");
      return;
    }
    setSubmittingApiKey(true);
    try {
      const res = await apiPost<{ apiKey: string }>("/api/api-keys", { name: apiKeyName, merchantUuid: apiKeyMerchantUuid });
      setNewApiKey(res.apiKey);
      setCreatingApiKey(false);
      setApiKeyName("");
      setApiKeyMerchantUuid(undefined);
      loadApiKeys();
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "Could not create API key");
    } finally {
      setSubmittingApiKey(false);
    }
  }

  function revokeApiKey(id: number) {
    confirmAction({
      title: "Revoke this API key?",
      content: "Any system still using it will immediately be locked out.",
      okText: "Revoke",
      danger: true,
      onConfirm: async () => {
        await apiDelete(`/api/api-keys/${id}`);
        loadApiKeys();
      },
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4">
        <Typography.Title level={5} style={{ marginBottom: 0 }}>
          Webhook Connections
        </Typography.Title>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          Connections to outside systems (like an AI/chatbot service, or your own system for event notifications)
          that a Bot flow can call, or that we push platform events to. This is technical setup, usually done once
          by your dev team — once created, anyone building a Bot flow just picks it by name.
        </Typography.Paragraph>

        {!creating ? (
          <div>
            <Button type="primary" icon={<PlusOutlined />} onClick={startCreate}>
              Create Connection
            </Button>
          </div>
        ) : (
          <Card title={editingId ? "Edit connection" : "New connection"} loading={loadingDetail}>
            <Space orientation="vertical" style={{ width: "100%", maxWidth: 480 }}>
              <Input placeholder="Name (e.g. Our AI Assistant)" value={name} onChange={(e) => setName(e.target.value)} />
              <Input placeholder="Endpoint URL (https://...)" value={url} onChange={(e) => setUrl(e.target.value)} />
              {editingId && (
                <Input placeholder="Leave blank to keep the existing secret" value={secret} onChange={(e) => setSecret(e.target.value)} />
              )}
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
              <div>
                <Typography.Text>Notify this endpoint when:</Typography.Text>
                <Checkbox.Group
                  style={{ display: "flex", flexDirection: "column", marginTop: 4 }}
                  value={events}
                  onChange={(v) => setEvents(v as string[])}
                  options={WEBHOOK_EVENTS}
                />
                <Typography.Paragraph type="secondary" style={{ marginTop: 4, marginBottom: 0 }}>
                  Leave all unchecked if this connection is only used inside a Bot flow&apos;s &quot;Connect to
                  another system&quot; step.
                </Typography.Paragraph>
              </div>
              <Typography.Text type="secondary">
                A secret is generated automatically and sent as a Bearer token on every call (and used to sign
                event notifications), so the other system can verify the request came from us.
              </Typography.Text>
              <Space>
                <Button onClick={() => { setCreating(false); setEditingId(null); }}>Cancel</Button>
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
              title: "Notifies on",
              dataIndex: "events",
              render: (v: string[] | undefined) => (v && v.length > 0 ? v.join(", ") : "—"),
            },
            {
              title: "Actions",
              key: "actions",
              render: (_, r) => (
                <Space>
                  <Button size="small" loading={testing === r.id} onClick={() => test(r.id)}>
                    Test Connection
                  </Button>
                  <Button type="text" icon={<EditOutlined />} onClick={() => startEdit(r.id)} />
                  <Button type="text" danger icon={<DeleteOutlined />} onClick={() => remove(r.id)} />
                </Space>
              ),
            },
          ]}
        />
      </div>

      <div className="flex flex-col gap-4">
        <Typography.Title level={5} style={{ marginBottom: 0 }}>
          REST API Keys
        </Typography.Title>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          Lets an external system call into LiveChat directly (start a chat, send a message) on behalf of one
          merchant, authenticated with a Bearer key instead of a login. See the API docs your dev team was given
          for the request shapes.
        </Typography.Paragraph>

        {newApiKey && (
          <Alert
            type="warning"
            showIcon
            title="Copy this key now — it won't be shown again"
            description={<Input.TextArea value={newApiKey} readOnly rows={2} />}
            closable
            onClose={() => setNewApiKey(null)}
          />
        )}

        {!creatingApiKey ? (
          <div>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreatingApiKey(true)}>
              Create API Key
            </Button>
          </div>
        ) : (
          <Card title="New API key">
            <Space orientation="vertical" style={{ width: "100%", maxWidth: 480 }}>
              <Input placeholder="Name (e.g. Our CRM)" value={apiKeyName} onChange={(e) => setApiKeyName(e.target.value)} />
              <Select
                placeholder="Merchant"
                style={{ width: "100%" }}
                value={apiKeyMerchantUuid}
                onChange={setApiKeyMerchantUuid}
                options={merchants.map((m) => ({ value: m.uuid, label: m.name }))}
              />
              <Space>
                <Button onClick={() => setCreatingApiKey(false)}>Cancel</Button>
                <Button type="primary" loading={submittingApiKey} onClick={submitApiKey}>
                  Save
                </Button>
              </Space>
            </Space>
          </Card>
        )}

        <Table
          rowKey="id"
          loading={apiKeysLoading}
          dataSource={apiKeys}
          columns={[
            { title: "Name", dataIndex: "name" },
            { title: "Merchant", dataIndex: "merchant_name" },
            { title: "Created", dataIndex: "created_at" },
            {
              title: "Actions",
              key: "actions",
              render: (_, r) => <Button type="text" danger icon={<DeleteOutlined />} onClick={() => revokeApiKey(r.id)} />,
            },
          ]}
        />
      </div>
    </div>
  );
}

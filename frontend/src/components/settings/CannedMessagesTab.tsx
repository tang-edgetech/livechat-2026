"use client";

import { useEffect, useState } from "react";
import { Button, Card, Checkbox, Input, Select, Space, Table, Tag, Typography, message } from "antd";
import { DeleteOutlined, PlusOutlined } from "@ant-design/icons";

import { apiDelete, apiGet, apiPost } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";
import { confirmAction } from "@/components/modals/confirm";
import type { CannedMessage } from "@/lib/automationTypes";
import type { Merchant } from "@/lib/types";

export function CannedMessagesTab() {
  const { user } = useAuth();
  const isSuperAdmin = user?.role === "super_admin";
  const canManage = user?.role === "admin" || isSuperAdmin;

  const [items, setItems] = useState<CannedMessage[]>([]);
  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [isGlobal, setIsGlobal] = useState(false);
  const [merchantUuid, setMerchantUuid] = useState<string | undefined>(undefined);
  const [submitting, setSubmitting] = useState(false);

  function load() {
    setLoading(true);
    apiGet<{ cannedMessages: CannedMessage[] }>("/api/canned-messages")
      .then((res) => setItems(res.cannedMessages))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial load on mount.
    load();
    if (canManage) {
      apiGet<{ merchants: Merchant[] }>("/api/merchants").then((res) => setMerchants(res.merchants));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function submit() {
    if (!title.trim() || !body.trim()) {
      message.error("Please fill in a title and message.");
      return;
    }
    if (!isGlobal && !merchantUuid) {
      message.error("Choose a merchant, or mark this as global.");
      return;
    }
    setSubmitting(true);
    try {
      await apiPost("/api/canned-messages", { title, body, isGlobal, merchantUuid: isGlobal ? undefined : merchantUuid });
      message.success("Canned message created");
      setCreating(false);
      setTitle("");
      setBody("");
      load();
    } catch {
      message.error("Could not create canned message");
    } finally {
      setSubmitting(false);
    }
  }

  function remove(id: number) {
    confirmAction({
      title: "Delete this canned message?",
      okText: "Delete",
      danger: true,
      onConfirm: async () => {
        await apiDelete(`/api/canned-messages/${id}`);
        load();
      },
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <Typography.Paragraph type="secondary">
        Ready-made replies your team can insert with one click while chatting.
      </Typography.Paragraph>

      {canManage && (
        <>
          {!creating ? (
            <div>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreating(true)}>
                Create Canned Message
              </Button>
            </div>
          ) : (
            <Card title="New canned message">
              <Space orientation="vertical" style={{ width: "100%", maxWidth: 480 }}>
                <Input placeholder="Title (e.g. Greeting)" value={title} onChange={(e) => setTitle(e.target.value)} />
                <Input.TextArea rows={3} placeholder="Message text" value={body} onChange={(e) => setBody(e.target.value)} />
                {isSuperAdmin && (
                  <Checkbox checked={isGlobal} onChange={(e) => setIsGlobal(e.target.checked)}>
                    Apply to all merchants
                  </Checkbox>
                )}
                {!isGlobal && (
                  <Select
                    placeholder="Merchant"
                    style={{ width: "100%" }}
                    value={merchantUuid}
                    onChange={setMerchantUuid}
                    options={merchants.map((m) => ({ value: m.uuid, label: m.name }))}
                  />
                )}
                <Space>
                  <Button onClick={() => setCreating(false)}>Cancel</Button>
                  <Button type="primary" loading={submitting} onClick={submit}>
                    Save
                  </Button>
                </Space>
              </Space>
            </Card>
          )}
        </>
      )}

      <Table
        rowKey="id"
        loading={loading}
        dataSource={items}
        columns={[
          { title: "Title", dataIndex: "title" },
          { title: "Message", dataIndex: "body", ellipsis: true },
          {
            title: "Scope",
            key: "scope",
            render: (_, r) => (r.is_global ? <Tag color="blue">Global</Tag> : <Tag>Merchant</Tag>),
          },
          ...(canManage
            ? [
                {
                  title: "Actions",
                  key: "actions",
                  render: (_: unknown, r: CannedMessage) => (
                    <Button type="text" danger icon={<DeleteOutlined />} onClick={() => remove(r.id)} />
                  ),
                },
              ]
            : []),
        ]}
      />
    </div>
  );
}

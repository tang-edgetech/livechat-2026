"use client";

import { useCallback, useEffect, useState } from "react";
import { Badge, Button, Card, Input, Select, Space, Table, Tag, message } from "antd";
import { CommentOutlined } from "@ant-design/icons";
import { useRouter } from "next/navigation";

import { apiGet, apiPost } from "@/lib/api";
import { useSocket } from "@/lib/socket";
import { confirmAction } from "@/components/modals/confirm";
import { PageHeader } from "@/components/layout/PageHeader";
import { STATUS_COLOR, type ChatSummary } from "@/lib/chatTypes";
import { titleCase } from "@/lib/format";
import type { Merchant } from "@/lib/types";

const STATUS_DOT: Record<ChatSummary["status"], "success" | "warning" | "default" | "processing" | "error"> = {
  active: "success",
  pending: "warning",
  closed: "default",
  bot: "processing",
  enquiry: "error",
};

export default function ChatsPage() {
  const router = useRouter();
  const [chats, setChats] = useState<ChatSummary[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const [merchantUuid, setMerchantUuid] = useState<string | undefined>(undefined);
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [selectedKeys, setSelectedKeys] = useState<React.Key[]>([]);

  const load = useCallback(() => {
    const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) });
    if (status) params.set("status", status);
    if (search) params.set("search", search);
    if (merchantUuid) params.set("merchantUuid", merchantUuid);
    setLoading(true);
    apiGet<{ chats: ChatSummary[]; total: number }>(`/api/chats?${params}`)
      .then((res) => {
        setChats(res.chats);
        setTotal(res.total);
      })
      .finally(() => setLoading(false));
  }, [page, pageSize, status, search, merchantUuid]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- refetch whenever filters/pagination change.
    load();
  }, [load]);

  useEffect(() => {
    apiGet<{ merchants: Merchant[] }>("/api/merchants").catch(() => null).then((res) => {
      if (res) setMerchants(res.merchants);
    });
  }, []);

  // Live updates: any chat_updated/presence_changed nudge on this
  // account's dashboard subject(s) just triggers a refetch — simplest
  // correct behavior for Phase 2's scale (overview.md §2/§8).
  useSocket("/ws", (event) => {
    if (event.type === "chat_updated" || event.type === "presence_changed") {
      load();
    }
  });

  function bulkClose() {
    confirmAction({
      title: "Close selected chats?",
      content: `${selectedKeys.length} chat(s) will be closed.`,
      okText: "Close",
      danger: true,
      onConfirm: async () => {
        await Promise.all(selectedKeys.map((uuid) => apiPost(`/api/chats/${String(uuid)}/close`)));
        setSelectedKeys([]);
        message.success("Chats closed");
        load();
      },
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <PageHeader icon={<CommentOutlined />} title="Chats" description="Every conversation across the merchants you can access." />

      <Card>
        <Space orientation="horizontal" style={{ marginBottom: 16, width: "100%", justifyContent: "space-between" }} wrap>
          <Space wrap>
            <Input.Search
              placeholder="Search customer name"
              allowClear
              style={{ width: 220 }}
              onSearch={(v) => {
                setPage(1);
                setSearch(v);
              }}
            />
            <Select
              allowClear
              placeholder="Status"
              style={{ width: 140 }}
              value={status}
              onChange={(v) => {
                setPage(1);
                setStatus(v);
              }}
              options={["active", "pending", "enquiry", "closed", "bot"].map((s) => ({ value: s, label: titleCase(s) }))}
            />
            {merchants.length > 1 && (
              <Select
                allowClear
                placeholder="Merchant"
                style={{ width: 180 }}
                value={merchantUuid}
                onChange={(v) => {
                  setPage(1);
                  setMerchantUuid(v);
                }}
                options={merchants.map((m) => ({ value: m.uuid, label: m.name }))}
              />
            )}
          </Space>
          {selectedKeys.length > 0 && (
            <Button danger onClick={bulkClose}>
              Close selected ({selectedKeys.length})
            </Button>
          )}
        </Space>

        <Table
          rowKey="uuid"
          loading={loading}
          dataSource={chats}
          rowSelection={{ selectedRowKeys: selectedKeys, onChange: setSelectedKeys }}
          onRow={(record) => ({ onClick: () => router.push(`/chats/${record.uuid}`) })}
          rowClassName="cursor-pointer"
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            onChange: (p, ps) => {
              setPage(p);
              setPageSize(ps);
            },
          }}
          columns={[
            {
              title: "Chat",
              key: "chat",
              render: (_, r) => (
                <Space>
                  <Badge status={STATUS_DOT[r.status]} />
                  <span className="font-mono text-xs">{r.uuid.slice(0, 8)}</span>
                </Space>
              ),
            },
            {
              title: "Customer",
              dataIndex: "visitor_name",
              render: (v: string, r: ChatSummary) => (
                <Space>
                  {v}
                  {r.visitor_tier === "vip" && <Tag color="gold">VIP</Tag>}
                </Space>
              ),
            },
            {
              title: "Timestamp",
              dataIndex: "last_message_at",
              render: (v: string | null, r: ChatSummary) => new Date(v ?? r.started_at).toLocaleString(),
            },
            {
              title: "PIC",
              key: "pic",
              render: (_, r) =>
                r.agent_name ? (
                  <span>
                    {r.agent_name} <span className="text-xs text-neutral-500">({r.agent_email})</span>
                  </span>
                ) : (
                  <Tag>Unassigned</Tag>
                ),
            },
            { title: "Merchant", dataIndex: "merchant_name" },
            {
              title: "Status",
              dataIndex: "status",
              render: (s: ChatSummary["status"]) => <Tag color={STATUS_COLOR[s]}>{titleCase(s)}</Tag>,
            },
          ]}
        />
      </Card>
    </div>
  );
}

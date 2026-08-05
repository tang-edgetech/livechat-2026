"use client";

import { useState } from "react";
import { Button, Input, Select, Space, Table, Tag, Typography, message } from "antd";

import { apiGet, apiPatch, apiPost, ApiError } from "@/lib/api";
import { confirmAction } from "@/components/modals/confirm";

type Visitor = {
  uuid: string;
  display_name: string;
  phone: string | null;
  email: string | null;
  tier: "normal" | "vip";
  merchant_name: string;
};

// The manual merge tool (overview.md §10.3) — handles the "visitor got a
// new phone number" case automatic identity resolution can't. No
// dedicated "Visitors" tab was ever specced elsewhere, so this lives
// alongside Users/Merchants where Admin/Super Admin already look for
// account-management tools.
export function VisitorsTab() {
  const [search, setSearch] = useState("");
  const [visitors, setVisitors] = useState<Visitor[]>([]);
  const [loading, setLoading] = useState(false);
  const [sourceUuid, setSourceUuid] = useState<string | undefined>(undefined);
  const [targetUuid, setTargetUuid] = useState<string | undefined>(undefined);

  function runSearch(q: string) {
    setLoading(true);
    apiGet<{ visitors: Visitor[] }>(`/api/visitors?search=${encodeURIComponent(q)}`)
      .then((res) => setVisitors(res.visitors))
      .finally(() => setLoading(false));
  }

  function toggleTier(v: Visitor) {
    const nextTier = v.tier === "vip" ? "normal" : "vip";
    confirmAction({
      title: `Mark ${v.display_name} as ${nextTier === "vip" ? "VIP" : "Normal"}?`,
      content:
        nextTier === "vip"
          ? "A new chat from this customer will route directly to an agent who handles VIP clients instead of the bot."
          : "This customer goes back to the standard bot-first routing.",
      onConfirm: async () => {
        try {
          await apiPatch(`/api/visitors/${v.uuid}`, { tier: nextTier });
          message.success("Tier updated");
          runSearch(search);
        } catch (err) {
          message.error(err instanceof ApiError ? err.message : "Could not update tier");
        }
      },
    });
  }

  function merge() {
    if (!sourceUuid || !targetUuid) return;
    confirmAction({
      title: "Merge these visitors?",
      content: "The source's chat history moves to the target. This can't be undone.",
      okText: "Merge",
      danger: true,
      onConfirm: async () => {
        try {
          await apiPost("/api/visitors/merge", { sourceUuid, targetUuid });
          message.success("Visitors merged");
          setSourceUuid(undefined);
          setTargetUuid(undefined);
          runSearch(search);
        } catch (err) {
          message.error(err instanceof ApiError ? err.message : "Could not merge visitors");
        }
      },
    });
  }

  const options = visitors.map((v) => ({
    value: v.uuid,
    label: `${v.display_name} — ${v.phone ?? v.email ?? "no contact"} (${v.merchant_name})`,
  }));

  return (
    <div className="flex flex-col gap-4">
      <Typography.Paragraph type="secondary">
        Search for a visitor by name, phone, or email to correct their email, or merge two records that turned
        out to be the same person (e.g. they came back with a new phone number).
      </Typography.Paragraph>

      <Input.Search
        placeholder="Search visitors"
        onSearch={(v) => {
          setSearch(v);
          runSearch(v);
        }}
        style={{ maxWidth: 320 }}
      />

      <Table
        rowKey="uuid"
        loading={loading}
        dataSource={visitors}
        columns={[
          { title: "Name", dataIndex: "display_name" },
          { title: "Phone", dataIndex: "phone", render: (v: string | null) => v ?? "—" },
          { title: "Email", dataIndex: "email", render: (v: string | null) => v ?? "—" },
          {
            title: "Tier",
            dataIndex: "tier",
            render: (t: Visitor["tier"]) => (t === "vip" ? <Tag color="gold">VIP</Tag> : <Tag>Normal</Tag>),
          },
          { title: "Merchant", dataIndex: "merchant_name" },
          {
            title: "Actions",
            key: "actions",
            render: (_: unknown, r: Visitor) => (
              <Button size="small" onClick={() => toggleTier(r)}>
                {r.tier === "vip" ? "Mark as Normal" : "Mark as VIP"}
              </Button>
            ),
          },
        ]}
      />

      <div className="flex flex-col gap-2 rounded border border-black/10 p-4 dark:border-white/10" style={{ maxWidth: 480 }}>
        <Typography.Text strong>Merge two records</Typography.Text>
        <Select
          placeholder="Source (old/duplicate record)"
          options={options}
          value={sourceUuid}
          onChange={setSourceUuid}
          allowClear
        />
        <Select
          placeholder="Target (record to keep)"
          options={options}
          value={targetUuid}
          onChange={setTargetUuid}
          allowClear
        />
        <Space>
          <Button type="primary" danger disabled={!sourceUuid || !targetUuid} onClick={merge}>
            Merge
          </Button>
        </Space>
      </div>
    </div>
  );
}

"use client";

import { useEffect, useState } from "react";
import { Button, Table, Tag, Tooltip, message } from "antd";
import { BarChartOutlined, DeleteOutlined, EditOutlined, PlusOutlined } from "@ant-design/icons";
import { useRouter } from "next/navigation";

import { apiDelete, apiGet } from "@/lib/api";
import { confirmAction } from "@/components/modals/confirm";
import { BotFlowAnalyticsDrawer } from "@/components/settings/botflow/BotFlowAnalyticsDrawer";
import type { BotFlow } from "@/lib/automationTypes";

export function BotTab() {
  const router = useRouter();
  const [flows, setFlows] = useState<BotFlow[]>([]);
  const [loading, setLoading] = useState(true);
  const [analyticsFlow, setAnalyticsFlow] = useState<BotFlow | null>(null);

  function load() {
    setLoading(true);
    apiGet<{ botFlows: BotFlow[] }>("/api/bot-flows")
      .then((res) => setFlows(res.botFlows))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial load on mount.
    load();
  }, []);

  function remove(id: number) {
    confirmAction({
      title: "Delete this bot flow?",
      okText: "Delete",
      danger: true,
      onConfirm: async () => {
        await apiDelete(`/api/bot-flows/${id}`);
        message.success("Deleted");
        load();
      },
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => router.push("/settings/bot-flows/new")}>
          Create Bot Flow
        </Button>
      </div>
      <Table
        rowKey="id"
        loading={loading}
        dataSource={flows}
        columns={[
          { title: "Name", dataIndex: "name" },
          {
            title: "Status",
            key: "status",
            render: (_, r) => <Tag color={r.is_active ? "success" : "default"}>{r.is_active ? "Active" : "Inactive"}</Tag>,
          },
          {
            title: "Scope",
            key: "scope",
            render: (_, r) => (r.is_global ? <Tag color="blue">Global</Tag> : <Tag>Merchant</Tag>),
          },
          {
            title: "Actions",
            key: "actions",
            render: (_, r) => (
              <>
                <Tooltip title="Analytics">
                  <Button type="text" icon={<BarChartOutlined />} onClick={() => setAnalyticsFlow(r)} />
                </Tooltip>
                <Tooltip title="Edit">
                  <Button type="text" icon={<EditOutlined />} onClick={() => router.push(`/settings/bot-flows/${r.id}`)} />
                </Tooltip>
                <Tooltip title="Delete">
                  <Button type="text" danger icon={<DeleteOutlined />} onClick={() => remove(r.id)} />
                </Tooltip>
              </>
            ),
          },
        ]}
      />
      <BotFlowAnalyticsDrawer flow={analyticsFlow} open={analyticsFlow !== null} onClose={() => setAnalyticsFlow(null)} />
    </div>
  );
}

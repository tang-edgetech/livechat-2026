"use client";

import { useEffect, useState } from "react";
import { Button, Space, Table, Tag, Tooltip, message } from "antd";
import { EditOutlined, PauseCircleOutlined, PlayCircleOutlined, PlusOutlined } from "@ant-design/icons";
import { useRouter } from "next/navigation";

import { apiGet, apiPatch } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";
import { confirmAction } from "@/components/modals/confirm";
import { titleCase } from "@/lib/format";
import type { Merchant } from "@/lib/types";

export function MerchantsTab() {
  const router = useRouter();
  const { user } = useAuth();
  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    apiGet<{ merchants: Merchant[] }>("/api/merchants")
      .then((res) => setMerchants(res.merchants))
      .finally(() => setLoading(false));
  }, []);

  function toggleStatus(merchant: Merchant) {
    const nextStatus = merchant.status === "active" ? "suspended" : "active";
    confirmAction({
      title: "Save changes?",
      content: `Set "${merchant.name}" to ${nextStatus}.`,
      onConfirm: async () => {
        try {
          await apiPatch(`/api/merchants/${merchant.uuid}/status`, { status: nextStatus });
          setMerchants((prev) =>
            prev.map((m) => (m.uuid === merchant.uuid ? { ...m, status: nextStatus } : m)),
          );
          message.success("Status updated");
        } catch {
          message.error("Could not update status");
        }
      },
    });
  }

  return (
    <div className="flex flex-col gap-4">
      {user?.role === "super_admin" && (
        <div className="flex justify-end">
          <Button type="primary" icon={<PlusOutlined />} onClick={() => router.push("/settings/merchants/new")}>
            Create Merchant
          </Button>
        </div>
      )}
      <Table
        rowKey="uuid"
        loading={loading}
        dataSource={merchants}
        columns={[
          { title: "Name", dataIndex: "name" },
          { title: "Code", dataIndex: "code" },
          {
            title: "Status",
            dataIndex: "status",
            render: (s: Merchant["status"]) => <Tag color={s === "active" ? "success" : "error"}>{titleCase(s)}</Tag>,
          },
          {
            title: "Actions",
            key: "actions",
            render: (_: unknown, record: Merchant) => (
              <Space>
                <Tooltip title="Edit">
                  <Button
                    type="text"
                    icon={<EditOutlined />}
                    onClick={() => router.push(`/settings/merchants/${record.uuid}`)}
                  />
                </Tooltip>
                {user?.role === "super_admin" && (
                  <Tooltip title={record.status === "active" ? "Suspend" : "Activate"}>
                    <Button
                      type="text"
                      icon={record.status === "active" ? <PauseCircleOutlined /> : <PlayCircleOutlined />}
                      onClick={() => toggleStatus(record)}
                    />
                  </Tooltip>
                )}
              </Space>
            ),
          },
        ]}
      />
    </div>
  );
}

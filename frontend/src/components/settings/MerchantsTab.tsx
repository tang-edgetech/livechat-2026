"use client";

import { useEffect, useState } from "react";
import { Button, Table, Tag } from "antd";
import { PlusOutlined } from "@ant-design/icons";
import { useRouter } from "next/navigation";

import { apiGet } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";
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
            render: (s: Merchant["status"]) => <Tag color={s === "active" ? "success" : "error"}>{s}</Tag>,
          },
        ]}
      />
    </div>
  );
}

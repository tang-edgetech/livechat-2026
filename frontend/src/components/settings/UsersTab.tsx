"use client";

import { useEffect, useState } from "react";
import { Button, Table, Tag, Tooltip } from "antd";
import { EditOutlined, PlusOutlined } from "@ant-design/icons";
import { useRouter } from "next/navigation";

import { apiGet } from "@/lib/api";
import { titleCase } from "@/lib/format";
import type { StaffUser } from "@/lib/types";

const STATUS_COLOR: Record<StaffUser["status"], string> = {
  active: "success",
  inactive: "default",
  suspended: "error",
};

export function UsersTab() {
  const router = useRouter();
  const [users, setUsers] = useState<StaffUser[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    apiGet<{ users: StaffUser[] }>("/api/users")
      .then((res) => setUsers(res.users))
      .finally(() => setLoading(false));
  }, []);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex justify-end">
        <Button type="primary" icon={<PlusOutlined />} onClick={() => router.push("/settings/users/new")}>
          Create User
        </Button>
      </div>
      <Table
        rowKey="uuid"
        loading={loading}
        dataSource={users}
        columns={[
          { title: "Display Name", dataIndex: "display_name" },
          { title: "Username", dataIndex: "username" },
          { title: "Email", dataIndex: "email" },
          { title: "Role", dataIndex: "role", render: (r: StaffUser["role"]) => <Tag>{titleCase(r)}</Tag> },
          {
            title: "Status",
            dataIndex: "status",
            render: (s: StaffUser["status"]) => <Tag color={STATUS_COLOR[s]}>{titleCase(s)}</Tag>,
          },
          {
            title: "Actions",
            key: "actions",
            render: (_, record) => (
              <Tooltip title="Edit">
                <Button
                  type="text"
                  icon={<EditOutlined />}
                  onClick={() => router.push(`/settings/users/${record.uuid}`)}
                />
              </Tooltip>
            ),
          },
        ]}
      />
    </div>
  );
}

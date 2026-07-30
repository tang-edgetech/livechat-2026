"use client";

import { useEffect, useState } from "react";
import { Button, Table, Tag } from "antd";
import { PlusOutlined } from "@ant-design/icons";
import { useRouter } from "next/navigation";

import { apiGet } from "@/lib/api";
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
        onRow={(record) => ({ onClick: () => router.push(`/settings/users/${record.uuid}`) })}
        rowClassName="cursor-pointer"
        columns={[
          { title: "Display name", dataIndex: "display_name" },
          { title: "Username", dataIndex: "username" },
          { title: "Email", dataIndex: "email" },
          { title: "Role", dataIndex: "role", render: (r: StaffUser["role"]) => <Tag>{r}</Tag> },
          {
            title: "Status",
            dataIndex: "status",
            render: (s: StaffUser["status"]) => <Tag color={STATUS_COLOR[s]}>{s}</Tag>,
          },
        ]}
      />
    </div>
  );
}

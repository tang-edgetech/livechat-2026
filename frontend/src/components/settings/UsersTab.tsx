"use client";

import { useEffect, useState } from "react";
import { Button, Space, Table, Tag, Tooltip, message } from "antd";
import { EditOutlined, LogoutOutlined, PlusOutlined } from "@ant-design/icons";
import { useRouter } from "next/navigation";
import dayjs from "dayjs";

import { apiGet, apiPatch, apiPost, ApiError } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";
import { confirmAction } from "@/components/modals/confirm";
import { titleCase } from "@/lib/format";
import type { StaffUser } from "@/lib/types";

const STATUS_COLOR: Record<StaffUser["status"], string> = {
  active: "success",
  inactive: "default",
  suspended: "error",
};

export function UsersTab() {
  const router = useRouter();
  const { user } = useAuth();
  const [users, setUsers] = useState<StaffUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedUuids, setSelectedUuids] = useState<string[]>([]);
  const [bulkApplying, setBulkApplying] = useState(false);

  function load() {
    setLoading(true);
    apiGet<{ users: StaffUser[] }>("/api/users")
      .then((res) => setUsers(res.users))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial load on mount.
    load();
  }, []);

  function quickToggle(target: StaffUser) {
    const next = target.status === "active" ? "inactive" : "active";
    confirmAction({
      title: `${next === "active" ? "Activate" : "Deactivate"} this account?`,
      content: `${target.display_name} will be set to ${titleCase(next)}.`,
      onConfirm: async () => {
        try {
          await apiPatch(`/api/users/${target.uuid}/status`, { status: next });
          message.success("Status updated");
          load();
        } catch (err) {
          message.error(err instanceof ApiError ? err.message : "Could not update status");
        }
      },
    });
  }

  function forceLogout(target: StaffUser) {
    confirmAction({
      title: "Force this user out?",
      content: `${target.display_name} will be signed out of every device right now. This doesn't deactivate their account — they can just log back in.`,
      okText: "Force Logout",
      danger: true,
      onConfirm: async () => {
        try {
          await apiPost(`/api/users/${target.uuid}/force-logout`);
          message.success("User signed out");
        } catch (err) {
          message.error(err instanceof ApiError ? err.message : "Could not sign the user out");
        }
      },
    });
  }

  function bulkForceLogout() {
    confirmAction({
      title: `Force out ${selectedUuids.length} account(s)?`,
      content: "Each will be signed out of every device right now — this doesn't deactivate their accounts.",
      okText: "Force Logout",
      danger: true,
      onConfirm: async () => {
        setBulkApplying(true);
        try {
          const res = await apiPost<{ applied: number; skipped: number }>("/api/users/bulk-force-logout", {
            uuids: selectedUuids,
          });
          message.success(`${res.applied} signed out${res.skipped ? `, ${res.skipped} skipped` : ""}`);
          setSelectedUuids([]);
        } catch (err) {
          message.error(err instanceof ApiError ? err.message : "Bulk force-logout failed");
        } finally {
          setBulkApplying(false);
        }
      },
    });
  }

  function bulkSetStatus(status: "active" | "inactive") {
    confirmAction({
      title: `${status === "active" ? "Activate" : "Deactivate"} ${selectedUuids.length} account(s)?`,
      onConfirm: async () => {
        setBulkApplying(true);
        try {
          const res = await apiPatch<{ applied: number; skipped: number }>("/api/users/bulk-status", {
            uuids: selectedUuids,
            status,
          });
          message.success(`${res.applied} updated${res.skipped ? `, ${res.skipped} skipped` : ""}`);
          setSelectedUuids([]);
          load();
        } catch (err) {
          message.error(err instanceof ApiError ? err.message : "Bulk update failed");
        } finally {
          setBulkApplying(false);
        }
      },
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex justify-between items-center">
        <Space>
          {selectedUuids.length > 0 && (
            <>
              <Tag>{selectedUuids.length} selected</Tag>
              <Button loading={bulkApplying} onClick={() => bulkSetStatus("active")}>
                Activate Selected
              </Button>
              <Button loading={bulkApplying} danger onClick={() => bulkSetStatus("inactive")}>
                Deactivate Selected
              </Button>
              <Button loading={bulkApplying} danger onClick={bulkForceLogout}>
                Force Logout Selected
              </Button>
            </>
          )}
        </Space>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => router.push("/settings/users/new")}>
          Create User
        </Button>
      </div>
      <Table<StaffUser>
        rowKey="uuid"
        loading={loading}
        dataSource={users}
        rowSelection={{
          selectedRowKeys: selectedUuids,
          onChange: (keys) => setSelectedUuids(keys as string[]),
          getCheckboxProps: (record) => ({ disabled: record.role === "super_admin" }),
        }}
        scroll={{ x: true }}
        columns={[
          {
            title: "Full Name",
            dataIndex: "display_name",
            sorter: (a, b) => a.display_name.localeCompare(b.display_name),
            render: (v: string, r) => (
              <>
                {v}
                {r.uuid === user?.uuid && <span className="text-neutral-500"> (Me)</span>}
              </>
            ),
          },
          { title: "Email Address", dataIndex: "email" },
          {
            title: "Role",
            dataIndex: "role",
            filters: [
              { text: "Super Admin", value: "super_admin" },
              { text: "Admin", value: "admin" },
              { text: "Agent", value: "agent" },
            ],
            onFilter: (value, record) => record.role === value,
            render: (r: StaffUser["role"]) => <Tag>{titleCase(r)}</Tag>,
          },
          {
            title: "Status",
            dataIndex: "status",
            filters: [
              { text: "Active", value: "active" },
              { text: "Inactive", value: "inactive" },
              { text: "Suspended", value: "suspended" },
            ],
            onFilter: (value, record) => record.status === value,
            render: (s: StaffUser["status"]) => <Tag color={STATUS_COLOR[s]}>{titleCase(s)}</Tag>,
          },
          {
            title: "Created",
            dataIndex: "created_at",
            sorter: (a, b) => a.created_at.localeCompare(b.created_at),
            render: (v: string) => dayjs(v).format("MMM D, YYYY h:mm A"),
          },
          {
            title: "Created By",
            dataIndex: "created_by_name",
            render: (v: string | null) => v ?? "—",
          },
          {
            title: "Actions",
            key: "actions",
            render: (_, record) => (
              <Space>
                {record.role !== "super_admin" && (
                  <Button size="small" onClick={() => quickToggle(record)}>
                    {record.status === "active" ? "Deactivate" : "Activate"}
                  </Button>
                )}
                {record.uuid !== user?.uuid && (
                  <Tooltip title="Force logout">
                    <Button size="small" danger icon={<LogoutOutlined />} onClick={() => forceLogout(record)} />
                  </Tooltip>
                )}
                <Tooltip title="Edit">
                  <Button
                    type="text"
                    icon={<EditOutlined />}
                    onClick={() => router.push(`/settings/users/${record.uuid}`)}
                  />
                </Tooltip>
              </Space>
            ),
          },
        ]}
      />
    </div>
  );
}

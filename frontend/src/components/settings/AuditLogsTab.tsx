"use client";

import { useEffect, useState } from "react";
import { Button, DatePicker, Input, Modal, Select, Space, Table, Tag, Typography, message } from "antd";
import { DeleteOutlined, DownloadOutlined } from "@ant-design/icons";
import type { TableProps } from "antd";
import dayjs, { type Dayjs } from "dayjs";

import { apiDelete, apiGet, ApiError } from "@/lib/api";
import { confirmAction } from "@/components/modals/confirm";
import { useAuth } from "@/context/AuthContext";
import { titleCase } from "@/lib/format";
import type { StaffUser } from "@/lib/types";

type AuditLog = {
  id: number;
  merchant_name: string | null;
  user_name: string | null;
  category: string;
  message: string;
  status_code: number;
  status_message: string | null;
  source: string;
  ip_address: string | null;
  created_at: string;
};

const CATEGORIES = [
  "auth",
  "user",
  "merchant",
  "visitor",
  "visitor_merge",
  "chat",
  "settings",
  "canned_message",
  "automation",
  "bot_flow",
  "integration",
  "audit_log",
  "setup",
];

function statusColor(code: number) {
  if (code >= 500) return "error";
  if (code >= 400) return "warning";
  return "success";
}

function toCsv(rows: AuditLog[]) {
  const header = ["ID", "Timestamp", "Category", "User", "Merchant", "Message", "Status Code", "Source", "IP Address"];
  const escape = (v: string) => `"${v.replace(/"/g, '""')}"`;
  const lines = rows.map((r) =>
    [r.id, r.created_at, r.category, r.user_name ?? "", r.merchant_name ?? "", r.message, r.status_code, r.source, r.ip_address ?? ""]
      .map((v) => escape(String(v)))
      .join(","),
  );
  return [header.map(escape).join(","), ...lines].join("\n");
}

// Audit Logs tab (overview.md §6.7) — filters, keyword search, sorting,
// row quick-view, CSV export, Super-Admin-only delete. Logging itself
// started back in Phase 0 (audit.Log calls throughout the handlers); this
// is just the first UI surface for reading that trail back.
export function AuditLogsTab() {
  const { user } = useAuth();
  const canDelete = user?.role === "super_admin";

  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [exporting, setExporting] = useState(false);
  const [users, setUsers] = useState<StaffUser[]>([]);
  const [viewing, setViewing] = useState<AuditLog | null>(null);

  const [category, setCategory] = useState<string | undefined>(undefined);
  const [userUuid, setUserUuid] = useState<string | undefined>(undefined);
  const [statusCode, setStatusCode] = useState("");
  const [search, setSearch] = useState("");
  const [dateRange, setDateRange] = useState<[Dayjs, Dayjs] | null>(null);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [sortBy, setSortBy] = useState("created_at");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");

  type QueryOverrides = { page?: number; pageSize?: number; sortBy?: string; sortDir?: "asc" | "desc" };

  function buildQuery(overrides: QueryOverrides = {}) {
    const params = new URLSearchParams();
    if (category) params.set("category", category);
    if (userUuid) params.set("userUuid", userUuid);
    if (statusCode) params.set("statusCode", statusCode);
    if (search) params.set("search", search);
    if (dateRange) {
      params.set("from", dateRange[0].format("YYYY-MM-DD 00:00:00"));
      params.set("to", dateRange[1].format("YYYY-MM-DD 23:59:59"));
    }
    params.set("sortBy", overrides.sortBy ?? sortBy);
    params.set("sortDir", overrides.sortDir ?? sortDir);
    params.set("page", String(overrides.page ?? page));
    params.set("pageSize", String(overrides.pageSize ?? pageSize));
    return params.toString();
  }

  function load(overrides: QueryOverrides = {}) {
    setLoading(true);
    apiGet<{ logs: AuditLog[]; total: number }>(`/api/audit-logs?${buildQuery(overrides)}`)
      .then((res) => {
        setLogs(res.logs);
        setTotal(res.total);
      })
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial load on mount.
    load();
    apiGet<{ users: StaffUser[] }>("/api/users").then((res) => setUsers(res.users));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function applyFilters() {
    setPage(1);
    load({ page: 1 });
  }

  async function exportCsv() {
    setExporting(true);
    try {
      const res = await apiGet<{ logs: AuditLog[]; total: number }>(`/api/audit-logs?${buildQuery({ page: 1, pageSize: 5000 })}`);
      const blob = new Blob([toCsv(res.logs)], { type: "text/csv;charset=utf-8" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `audit-log-${dayjs().format("YYYY-MM-DD")}.csv`;
      a.click();
      URL.revokeObjectURL(url);
      if (res.total > res.logs.length) {
        message.warning(`Exported the first ${res.logs.length} of ${res.total} matching rows — narrow your filters for a complete export.`);
      }
    } catch {
      message.error("Could not export audit logs");
    } finally {
      setExporting(false);
    }
  }

  function remove(id: number) {
    confirmAction({
      title: "Delete this audit log entry?",
      content: "This can't be undone.",
      okText: "Delete",
      danger: true,
      onConfirm: async () => {
        try {
          await apiDelete(`/api/audit-logs/${id}`);
          message.success("Entry deleted");
          load();
        } catch (err) {
          message.error(err instanceof ApiError ? err.message : "Could not delete entry");
        }
      },
    });
  }

  const handleTableChange: TableProps<AuditLog>["onChange"] = (pagination, _filters, sorter) => {
    const nextPage = pagination.current ?? 1;
    const nextPageSize = pagination.pageSize ?? 20;
    const s = Array.isArray(sorter) ? sorter[0] : sorter;
    const nextSortBy = (s?.field as string) ?? "created_at";
    const nextSortDir = s?.order === "ascend" ? "asc" : "desc";

    setPage(nextPage);
    setPageSize(nextPageSize);
    setSortBy(nextSortBy);
    setSortDir(nextSortDir);
    load({ page: nextPage, pageSize: nextPageSize, sortBy: nextSortBy, sortDir: nextSortDir });
  };

  return (
    <div className="flex flex-col gap-4">
      <Space wrap>
        <Select
          placeholder="Category"
          allowClear
          style={{ width: 160 }}
          value={category}
          onChange={setCategory}
          options={CATEGORIES.map((c) => ({ value: c, label: titleCase(c) }))}
        />
        <Select
          placeholder="User"
          allowClear
          showSearch
          style={{ width: 200 }}
          value={userUuid}
          onChange={setUserUuid}
          optionFilterProp="label"
          options={users.map((u) => ({ value: u.uuid, label: `${u.display_name} (${u.email})` }))}
        />
        <Input
          placeholder="Status code"
          style={{ width: 120 }}
          value={statusCode}
          onChange={(e) => setStatusCode(e.target.value.replace(/\D/g, ""))}
        />
        <DatePicker.RangePicker value={dateRange} onChange={(v) => setDateRange(v as [Dayjs, Dayjs] | null)} />
        <Input.Search
          placeholder="Search message"
          style={{ width: 220 }}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          onSearch={applyFilters}
        />
        <Button type="primary" onClick={applyFilters}>
          Apply Filters
        </Button>
        <Button icon={<DownloadOutlined />} loading={exporting} onClick={exportCsv}>
          Export CSV
        </Button>
      </Space>

      <Table<AuditLog>
        rowKey="id"
        loading={loading}
        dataSource={logs}
        onChange={handleTableChange}
        onRow={(record) => ({ onClick: () => setViewing(record), style: { cursor: "pointer" } })}
        pagination={{ current: page, pageSize, total, showSizeChanger: true }}
        scroll={{ x: true }}
        columns={[
          { title: "Timestamp", dataIndex: "created_at", key: "created_at", sorter: true },
          { title: "Category", dataIndex: "category", key: "category", sorter: true, render: (v: string) => <Tag>{titleCase(v)}</Tag> },
          { title: "User", dataIndex: "user_name", render: (v: string | null) => v ?? "—" },
          { title: "Merchant", dataIndex: "merchant_name", render: (v: string | null) => v ?? "—" },
          { title: "Message", dataIndex: "message", ellipsis: true },
          {
            title: "Status",
            dataIndex: "status_code",
            key: "status_code",
            sorter: true,
            render: (v: number) => <Tag color={statusColor(v)}>{v}</Tag>,
          },
          { title: "Source", dataIndex: "source", key: "source", sorter: true, render: (v: string) => titleCase(v) },
          ...(canDelete
            ? [
                {
                  title: "Actions",
                  key: "actions",
                  render: (_: unknown, r: AuditLog) => (
                    <Button
                      type="text"
                      danger
                      icon={<DeleteOutlined />}
                      onClick={(e) => {
                        e.stopPropagation();
                        remove(r.id);
                      }}
                    />
                  ),
                },
              ]
            : []),
        ]}
      />

      <Modal open={!!viewing} onCancel={() => setViewing(null)} onOk={() => setViewing(null)} title="Audit Log Entry" footer={null}>
        {viewing && (
          <Space orientation="vertical" style={{ width: "100%" }}>
            <Typography.Text strong>ID</Typography.Text>
            <Typography.Text>{viewing.id}</Typography.Text>
            <Typography.Text strong>Timestamp</Typography.Text>
            <Typography.Text>{viewing.created_at}</Typography.Text>
            <Typography.Text strong>Category</Typography.Text>
            <Typography.Text>{titleCase(viewing.category)}</Typography.Text>
            <Typography.Text strong>User</Typography.Text>
            <Typography.Text>{viewing.user_name ?? "—"}</Typography.Text>
            <Typography.Text strong>Merchant</Typography.Text>
            <Typography.Text>{viewing.merchant_name ?? "—"}</Typography.Text>
            <Typography.Text strong>Message</Typography.Text>
            <Typography.Text>{viewing.message}</Typography.Text>
            <Typography.Text strong>Status</Typography.Text>
            <Typography.Text>
              {viewing.status_code} {viewing.status_message ?? ""}
            </Typography.Text>
            <Typography.Text strong>Source</Typography.Text>
            <Typography.Text>{titleCase(viewing.source)}</Typography.Text>
            <Typography.Text strong>IP Address</Typography.Text>
            <Typography.Text>{viewing.ip_address ?? "—"}</Typography.Text>
          </Space>
        )}
      </Modal>
    </div>
  );
}

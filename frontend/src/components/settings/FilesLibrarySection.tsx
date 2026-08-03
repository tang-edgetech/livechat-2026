"use client";

import { useEffect, useState } from "react";
import { Button, Card, Empty, Input, Segmented, Select, Space, Table, Tag, Typography, message } from "antd";
import {
  AppstoreOutlined,
  BarsOutlined,
  CheckOutlined,
  CloseOutlined,
  DeleteOutlined,
  EditOutlined,
  FileOutlined,
  FileImageOutlined,
  FilePdfOutlined,
} from "@ant-design/icons";

import { apiDelete, apiGet, apiPatch, ApiError } from "@/lib/api";
import { confirmAction } from "@/components/modals/confirm";
import { useAuth } from "@/context/AuthContext";
import { titleCase } from "@/lib/format";
import type { Merchant } from "@/lib/types";

type LibraryFile = {
  uuid: string;
  original_name: string;
  mime_type: string;
  size_bytes: number;
  purpose: string;
  merchant_name: string;
  uploader_type: "visitor" | "user" | "system";
  uploader_name: string | null;
  created_at: string;
};

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function fileIcon(mimeType: string) {
  if (mimeType.startsWith("image/")) return <FileImageOutlined />;
  if (mimeType === "application/pdf") return <FilePdfOutlined />;
  return <FileOutlined />;
}

function uploaderLabel(f: LibraryFile) {
  if (f.uploader_type === "system") return "System";
  return f.uploader_name ?? "—";
}

// Files "Library" — a browsable view of what's actually been uploaded
// (overview.md §6.8), previously nowhere to see this except by opening
// individual chats. List and grid share the same fetched data; the
// toggle only changes how it's rendered.
export function FilesLibrarySection() {
  const { user } = useAuth();
  const canManage = user?.role === "admin" || user?.role === "super_admin";

  const [files, setFiles] = useState<LibraryFile[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [view, setView] = useState<"list" | "grid">("list");
  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [merchantUuid, setMerchantUuid] = useState<string | undefined>(undefined);
  const [mimePrefix, setMimePrefix] = useState<string | undefined>(undefined);
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const pageSize = 24;
  const [renamingUuid, setRenamingUuid] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");

  function load(overridePage?: number) {
    setLoading(true);
    const params = new URLSearchParams();
    if (merchantUuid) params.set("merchantUuid", merchantUuid);
    if (mimePrefix) params.set("mimePrefix", mimePrefix);
    if (search) params.set("search", search);
    params.set("page", String(overridePage ?? page));
    params.set("pageSize", String(pageSize));
    apiGet<{ files: LibraryFile[]; total: number }>(`/api/files?${params.toString()}`)
      .then((res) => {
        setFiles(res.files);
        setTotal(res.total);
      })
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial load on mount.
    load();
    apiGet<{ merchants: Merchant[] }>("/api/merchants").then((res) => setMerchants(res.merchants));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function applyFilters() {
    setPage(1);
    load(1);
  }

  function startRename(f: LibraryFile) {
    setRenamingUuid(f.uuid);
    setRenameValue(f.original_name);
  }

  async function saveRename() {
    if (!renamingUuid) return;
    try {
      await apiPatch(`/api/files/${renamingUuid}`, { originalName: renameValue });
      message.success("File renamed");
      setRenamingUuid(null);
      load();
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "Could not rename file");
    }
  }

  function removeFile(f: LibraryFile) {
    confirmAction({
      title: "Delete this file?",
      content: `"${f.original_name}" will be permanently removed.`,
      okText: "Delete",
      danger: true,
      onConfirm: async () => {
        try {
          await apiDelete(`/api/files/${f.uuid}`);
          message.success("File deleted");
          load();
        } catch (err) {
          message.error(err instanceof ApiError ? err.message : "Could not delete file");
        }
      },
    });
  }

  function fileNameCell(f: LibraryFile) {
    if (renamingUuid === f.uuid) {
      return (
        <Space.Compact>
          <Input size="small" value={renameValue} onChange={(e) => setRenameValue(e.target.value)} onPressEnter={saveRename} autoFocus />
          <Button size="small" icon={<CheckOutlined />} onClick={saveRename} />
          <Button size="small" icon={<CloseOutlined />} onClick={() => setRenamingUuid(null)} />
        </Space.Compact>
      );
    }
    return (
      <a href={`/api/files/${f.uuid}`} target="_blank" rel="noopener noreferrer">
        {fileIcon(f.mime_type)} {f.original_name}
      </a>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2 justify-between">
        <div className="flex flex-wrap gap-2">
          <Select
            placeholder="Merchant"
            allowClear
            style={{ width: 180 }}
            value={merchantUuid}
            onChange={(v) => setMerchantUuid(v)}
            options={merchants.map((m) => ({ value: m.uuid, label: m.name }))}
          />
          <Select
            placeholder="Media type"
            allowClear
            style={{ width: 160 }}
            value={mimePrefix}
            onChange={(v) => setMimePrefix(v)}
            options={[
              { value: "image/", label: "Images" },
              { value: "application/pdf", label: "PDFs" },
            ]}
          />
          <Input.Search placeholder="Search filename" style={{ width: 200 }} value={search} onChange={(e) => setSearch(e.target.value)} onSearch={applyFilters} />
        </div>
        <Segmented
          value={view}
          onChange={(v) => setView(v as "list" | "grid")}
          options={[
            { value: "list", icon: <BarsOutlined /> },
            { value: "grid", icon: <AppstoreOutlined /> },
          ]}
        />
      </div>

      {view === "list" ? (
        <Table<LibraryFile>
          rowKey="uuid"
          loading={loading}
          dataSource={files}
          pagination={{
            current: page,
            pageSize,
            total,
            onChange: (p) => {
              setPage(p);
              load(p);
            },
          }}
          scroll={{ x: true }}
          columns={[
            { title: "File", key: "file", render: (_, r) => fileNameCell(r) },
            { title: "Type", dataIndex: "mime_type" },
            { title: "Size", dataIndex: "size_bytes", render: (v: number) => formatBytes(v) },
            { title: "Merchant", dataIndex: "merchant_name" },
            { title: "Uploaded By", key: "uploaded_by", render: (_, r) => <>{uploaderLabel(r)} <Tag>{titleCase(r.uploader_type)}</Tag></> },
            { title: "Timestamp", dataIndex: "created_at" },
            ...(canManage
              ? [
                  {
                    title: "Actions",
                    key: "actions",
                    render: (_: unknown, r: LibraryFile) => (
                      <Space>
                        <Button type="text" icon={<EditOutlined />} onClick={() => startRename(r)} />
                        <Button type="text" danger icon={<DeleteOutlined />} onClick={() => removeFile(r)} />
                      </Space>
                    ),
                  },
                ]
              : []),
          ]}
        />
      ) : (
        <>
          {files.length === 0 && !loading ? (
            <Empty description="No files found" />
          ) : (
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-3">
              {files.map((f) => (
                <Card
                  key={f.uuid}
                  size="small"
                  loading={loading}
                  actions={
                    canManage
                      ? [
                          <EditOutlined key="edit" onClick={() => startRename(f)} />,
                          <DeleteOutlined key="delete" onClick={() => removeFile(f)} />,
                        ]
                      : undefined
                  }
                >
                  <div className="flex flex-col items-center gap-2 text-center">
                    <a href={`/api/files/${f.uuid}`} target="_blank" rel="noopener noreferrer" className="w-full">
                      {f.mime_type.startsWith("image/") ? (
                        <img src={`/api/files/${f.uuid}`} alt={f.original_name} className="h-24 w-full object-cover rounded" />
                      ) : (
                        <div className="h-24 flex items-center justify-center text-4xl">{fileIcon(f.mime_type)}</div>
                      )}
                    </a>
                    {renamingUuid === f.uuid ? (
                      <Space.Compact style={{ width: "100%" }}>
                        <Input size="small" value={renameValue} onChange={(e) => setRenameValue(e.target.value)} onPressEnter={saveRename} autoFocus />
                        <Button size="small" icon={<CheckOutlined />} onClick={saveRename} />
                      </Space.Compact>
                    ) : (
                      <Typography.Text ellipsis style={{ maxWidth: "100%" }} title={f.original_name}>
                        {f.original_name}
                      </Typography.Text>
                    )}
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      {formatBytes(f.size_bytes)} · {uploaderLabel(f)}
                    </Typography.Text>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      {f.created_at}
                    </Typography.Text>
                  </div>
                </Card>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

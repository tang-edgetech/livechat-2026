"use client";

import { useEffect, useState } from "react";
import { Button, Card, Descriptions, Input, Select, Space, Tag, Typography, message } from "antd";
import { useParams } from "next/navigation";

import { apiDelete, apiGet, apiPost, apiPatch, ApiError } from "@/lib/api";
import { confirmAction } from "@/components/modals/confirm";
import type { Merchant, StaffUser } from "@/lib/types";

export default function EditUserPage() {
  const { uuid } = useParams<{ uuid: string }>();

  const [target, setTarget] = useState<StaffUser | null>(null);
  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [status, setStatus] = useState<StaffUser["status"]>("active");
  const [selectedMerchants, setSelectedMerchants] = useState<string[]>([]);
  const [newPassword, setNewPassword] = useState("");
  const [loading, setLoading] = useState(true);
  const [savingStatus, setSavingStatus] = useState(false);
  const [savingMerchants, setSavingMerchants] = useState(false);
  const [savingPassword, setSavingPassword] = useState(false);

  useEffect(() => {
    Promise.all([
      apiGet<{ users: StaffUser[] }>("/api/users"),
      apiGet<{ merchants: Merchant[] }>("/api/merchants"),
    ]).then(([usersRes, merchantsRes]) => {
      const found = usersRes.users.find((u) => u.uuid === uuid) ?? null;
      setTarget(found);
      setStatus(found?.status ?? "active");
      setSelectedMerchants(found?.merchants.map((m) => m.uuid) ?? []);
      setMerchants(merchantsRes.merchants);
      setLoading(false);
    });
  }, [uuid]);

  function saveStatus() {
    confirmAction({
      title: "Save changes?",
      content: `Set status to "${status}".`,
      onConfirm: async () => {
        setSavingStatus(true);
        try {
          await apiPatch(`/api/users/${uuid}/status`, { status });
          message.success("Status updated");
        } catch {
          message.error("Could not update status");
        } finally {
          setSavingStatus(false);
        }
      },
    });
  }

  function saveMerchants() {
    confirmAction({
      title: "Save changes?",
      content: "Update merchant access for this account.",
      onConfirm: async () => {
        setSavingMerchants(true);
        try {
          const before = new Set(target?.merchants.map((m) => m.uuid));
          const after = new Set(selectedMerchants);
          const toGrant = selectedMerchants.filter((m) => !before.has(m));
          const toRevoke = [...before].filter((m) => !after.has(m));

          await Promise.all([
            ...toGrant.map((m) => apiPost(`/api/users/${uuid}/merchants`, { merchantUuid: m })),
            ...toRevoke.map((m) => apiDelete(`/api/users/${uuid}/merchants/${m}`)),
          ]);
          message.success("Merchant access updated");
        } catch (err) {
          message.error(err instanceof ApiError ? err.message : "Could not update merchant access");
        } finally {
          setSavingMerchants(false);
        }
      },
    });
  }

  function forcePassword() {
    if (newPassword.length < 10) {
      message.error("Password must be at least 10 characters");
      return;
    }
    confirmAction({
      title: "Save changes?",
      content: "Force-reset this account's password.",
      onConfirm: async () => {
        setSavingPassword(true);
        try {
          await apiPost(`/api/users/${uuid}/force-password`, { newPassword });
          setNewPassword("");
          message.success("Password reset");
        } catch {
          message.error("Could not reset password");
        } finally {
          setSavingPassword(false);
        }
      },
    });
  }

  if (loading) return null;
  if (!target) return <Typography.Paragraph>User not found.</Typography.Paragraph>;

  return (
    <div className="flex flex-col gap-6">
      <Card title="Account">
        <Descriptions column={1} bordered size="small">
          <Descriptions.Item label="Display Name">{target.display_name}</Descriptions.Item>
          <Descriptions.Item label="Username">{target.username}</Descriptions.Item>
          <Descriptions.Item label="Email">{target.email}</Descriptions.Item>
          <Descriptions.Item label="Role">
            <Tag>{target.role}</Tag>
          </Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="Status">
        <Space>
          <Select
            value={status}
            style={{ width: 160 }}
            onChange={setStatus}
            options={["active", "inactive", "suspended"].map((s) => ({ value: s, label: s }))}
          />
          <Button type="primary" loading={savingStatus} onClick={saveStatus}>
            Save
          </Button>
        </Space>
      </Card>

      {target.role !== "super_admin" && (
        <Card title="Merchant access">
          <Space orientation="vertical" style={{ width: "100%" }}>
            <Select
              mode="multiple"
              style={{ width: "100%" }}
              value={selectedMerchants}
              onChange={setSelectedMerchants}
              options={merchants.map((m) => ({ value: m.uuid, label: m.name }))}
            />
            <Button type="primary" loading={savingMerchants} onClick={saveMerchants}>
              Save
            </Button>
          </Space>
        </Card>
      )}

      <Card title="Force password reset">
        <Space orientation="vertical" style={{ width: "100%" }}>
          <Input.Password
            placeholder="New password (min 10 characters)"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
          />
          <Button danger loading={savingPassword} onClick={forcePassword}>
            Reset password
          </Button>
        </Space>
      </Card>
    </div>
  );
}

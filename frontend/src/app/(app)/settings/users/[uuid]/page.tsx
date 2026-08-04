"use client";

import { useEffect, useState } from "react";
import { Button, Card, Col, Descriptions, Input, Row, Segmented, Select, Space, Typography, message } from "antd";
import { useParams } from "next/navigation";

import { apiDelete, apiGet, apiPost, apiPatch, ApiError } from "@/lib/api";
import { confirmAction } from "@/components/modals/confirm";
import { titleCase } from "@/lib/format";
import type { Merchant, StaffUser } from "@/lib/types";

export default function EditUserPage() {
  const { uuid } = useParams<{ uuid: string }>();

  const [target, setTarget] = useState<StaffUser | null>(null);
  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [selectedMerchants, setSelectedMerchants] = useState<string[]>([]);
  const [newPassword, setNewPassword] = useState("");
  const [loading, setLoading] = useState(true);
  const [savingStatus, setSavingStatus] = useState(false);
  const [savingMerchants, setSavingMerchants] = useState(false);
  const [savingPassword, setSavingPassword] = useState(false);

  function load() {
    Promise.all([
      apiGet<{ users: StaffUser[] }>("/api/users"),
      apiGet<{ merchants: Merchant[] }>("/api/merchants"),
    ]).then(([usersRes, merchantsRes]) => {
      const found = usersRes.users.find((u) => u.uuid === uuid) ?? null;
      setTarget(found);
      setSelectedMerchants(found?.merchants.map((m) => m.uuid) ?? []);
      setMerchants(merchantsRes.merchants);
      setLoading(false);
    });
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [uuid]);

  // Quick toggle — applies immediately on pick (with confirmation), no
  // separate Select-then-Save step.
  function changeStatus(status: StaffUser["status"]) {
    if (!target || status === target.status) return;
    confirmAction({
      title: "Save changes?",
      content: `Set status to "${titleCase(status)}".`,
      onConfirm: async () => {
        setSavingStatus(true);
        try {
          await apiPatch(`/api/users/${uuid}/status`, { status });
          message.success("Status updated");
          load();
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
    if (newPassword.length < 8 || newPassword.length > 16 || !/[A-Z]/.test(newPassword) || !/[0-9]/.test(newPassword)) {
      message.error("Password must be 8-16 characters with at least one uppercase letter and one digit");
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
    <Row gutter={24}>
      <Col xs={24} lg={12}>
        <Card title="Basic Information">
          <Space orientation="vertical" style={{ width: "100%" }}>
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label="Full Name">{target.display_name}</Descriptions.Item>
              <Descriptions.Item label="Email Address">{target.email}</Descriptions.Item>
              <Descriptions.Item label="Role">{titleCase(target.role)}</Descriptions.Item>
              <Descriptions.Item label="Created">
                {target.created_at}
                {target.created_by_name ? ` by ${target.created_by_name}` : ""}
              </Descriptions.Item>
            </Descriptions>

            <div>
              <Typography.Text strong>Status</Typography.Text>
              <div style={{ marginTop: 4 }}>
                <Segmented
                  disabled={savingStatus}
                  value={target.status}
                  onChange={(v) => changeStatus(v as StaffUser["status"])}
                  options={(["active", "inactive", "suspended"] as const).map((s) => ({ value: s, label: titleCase(s) }))}
                />
              </div>
            </div>
          </Space>
        </Card>
      </Col>

      <Col xs={24} lg={12}>
        <Space orientation="vertical" style={{ width: "100%" }}>
          {target.role !== "super_admin" && (
            <Card title="Merchant Access">
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

          <Card title="Force Password Reset">
            <Space orientation="vertical" style={{ width: "100%" }}>
              <Input.Password
                placeholder="New password (8-16 chars, 1 uppercase, 1 digit)"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
              />
              <Button danger loading={savingPassword} onClick={forcePassword}>
                Reset Password
              </Button>
            </Space>
          </Card>
        </Space>
      </Col>
    </Row>
  );
}

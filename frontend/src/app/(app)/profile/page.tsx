"use client";

import { useState } from "react";
import { Button, Card, Descriptions, Form, Input, Typography, message } from "antd";

import { useAuth } from "@/context/AuthContext";
import { apiFetch, ApiError } from "@/lib/api";
import { confirmAction } from "@/components/modals/confirm";

export default function ProfilePage() {
  const { user, refresh } = useAuth();
  const [nameForm] = Form.useForm();
  const [savingName, setSavingName] = useState(false);
  const [savingPassword, setSavingPassword] = useState(false);

  function saveDisplayName(values: { displayName: string }) {
    confirmAction({
      title: "Save changes?",
      content: "Update your display name.",
      onConfirm: async () => {
        setSavingName(true);
        try {
          await apiFetch("/api/profile", { method: "PATCH", body: JSON.stringify(values) });
          await refresh();
          message.success("Display name updated");
        } catch {
          message.error("Could not update display name");
        } finally {
          setSavingName(false);
        }
      },
    });
  }

  function savePassword(values: { currentPassword: string; newPassword: string; confirmNewPassword: string }) {
    if (values.newPassword !== values.confirmNewPassword) {
      message.error("New passwords do not match");
      return;
    }
    confirmAction({
      title: "Save changes?",
      content: "Change your password.",
      onConfirm: async () => {
        setSavingPassword(true);
        try {
          await apiFetch("/api/profile/password", {
            method: "POST",
            body: JSON.stringify({ currentPassword: values.currentPassword, newPassword: values.newPassword }),
          });
          message.success("Password changed");
        } catch (err) {
          const detail = err instanceof ApiError ? err.message : "Could not change password";
          message.error(
            detail === "current_password_incorrect" ? "Current password is incorrect" : detail,
          );
        } finally {
          setSavingPassword(false);
        }
      },
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <Card title="Profile">
        <Descriptions column={1} bordered size="small">
          <Descriptions.Item label="Role">{user?.role}</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="Display name">
        <Form
          form={nameForm}
          layout="vertical"
          initialValues={{ displayName: user?.display_name }}
          onFinish={saveDisplayName}
          disabled={savingName}
          style={{ maxWidth: 360 }}
        >
          <Form.Item name="displayName" label="Display name" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={savingName}>
            Save
          </Button>
        </Form>
      </Card>

      <Card title="Change password">
        <Form layout="vertical" onFinish={savePassword} disabled={savingPassword} style={{ maxWidth: 360 }}>
          <Form.Item name="currentPassword" label="Current password" rules={[{ required: true }]}>
            <Input.Password />
          </Form.Item>
          <Form.Item name="newPassword" label="New password" rules={[{ required: true }]}>
            <Input.Password />
          </Form.Item>
          <Form.Item name="confirmNewPassword" label="Confirm new password" rules={[{ required: true }]}>
            <Input.Password />
          </Form.Item>
          <Typography.Text type="secondary" style={{ display: "block", marginBottom: 16 }}>
            Minimum 10 characters, no other rules.
          </Typography.Text>
          <Button type="primary" htmlType="submit" loading={savingPassword}>
            Save
          </Button>
        </Form>
      </Card>
    </div>
  );
}

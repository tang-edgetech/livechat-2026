"use client";

import { useState } from "react";
import { Button, Card, Col, Descriptions, Form, Input, Row, Typography, message } from "antd";

import { useAuth } from "@/context/AuthContext";
import { apiFetch, ApiError } from "@/lib/api";
import { confirmAction } from "@/components/modals/confirm";
import { titleCase } from "@/lib/format";

export default function ProfilePage() {
  const { user, refresh } = useAuth();
  const [nameForm] = Form.useForm();
  const [savingName, setSavingName] = useState(false);
  const [savingPassword, setSavingPassword] = useState(false);

  function saveDisplayName(values: { displayName: string }) {
    confirmAction({
      title: "Save changes?",
      content: "Update your full name.",
      onConfirm: async () => {
        setSavingName(true);
        try {
          await apiFetch("/api/profile", { method: "PATCH", body: JSON.stringify(values) });
          await refresh();
          message.success("Full name updated");
        } catch {
          message.error("Could not update full name");
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
    <Row gutter={24}>
      <Col xs={24} lg={12}>
        <Card title="Basic Information">
          <Descriptions column={1} bordered size="small" style={{ marginBottom: 24 }}>
            <Descriptions.Item label="Email Address">{user?.email}</Descriptions.Item>
            <Descriptions.Item label="Role">{user ? titleCase(user.role) : ""}</Descriptions.Item>
          </Descriptions>

          <Form
            form={nameForm}
            layout="vertical"
            initialValues={{ displayName: user?.display_name }}
            onFinish={saveDisplayName}
            disabled={savingName}
          >
            <Form.Item name="displayName" label="Full Name" rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Button type="primary" htmlType="submit" loading={savingName}>
              Save
            </Button>
          </Form>
        </Card>
      </Col>

      <Col xs={24} lg={12}>
        <Card title="Change Password">
          <Form layout="vertical" onFinish={savePassword} disabled={savingPassword}>
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
              8-16 characters, at least one uppercase letter and one digit. Symbols optional.
            </Typography.Text>
            <Button type="primary" htmlType="submit" loading={savingPassword}>
              Save
            </Button>
          </Form>
        </Card>
      </Col>
    </Row>
  );
}

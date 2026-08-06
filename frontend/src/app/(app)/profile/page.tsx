"use client";

import { useState } from "react";
import { Button, Card, Col, Descriptions, Form, Input, Row, Typography, message } from "antd";
import { CheckOutlined, UserOutlined } from "@ant-design/icons";

import { useAuth } from "@/context/AuthContext";
import { apiFetch, ApiError } from "@/lib/api";
import { confirmAction } from "@/components/modals/confirm";
import { PageHeader } from "@/components/layout/PageHeader";
import { titleCase } from "@/lib/format";
import { THEME_KEYS, THEME_LABELS, THEME_SWATCHES, resolveTheme, type ThemeKey } from "@/lib/theme";

export default function ProfilePage() {
  const { user, setUser, refresh } = useAuth();
  const [nameForm] = Form.useForm();
  const [savingName, setSavingName] = useState(false);
  const [savingPassword, setSavingPassword] = useState(false);
  const [savingTheme, setSavingTheme] = useState<ThemeKey | null>(null);

  async function selectTheme(key: ThemeKey) {
    if (!user || key === resolveTheme(user.theme_preference)) return;
    const previous = user.theme_preference;
    setSavingTheme(key);
    setUser({ ...user, theme_preference: key });
    try {
      await apiFetch("/api/profile", { method: "PATCH", body: JSON.stringify({ themePreference: key }) });
    } catch {
      setUser({ ...user, theme_preference: previous });
      message.error("Could not save Appearance preference");
    } finally {
      setSavingTheme(null);
    }
  }

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

  const activeTheme = resolveTheme(user?.theme_preference);

  return (
    <div className="flex flex-col gap-4">
      <PageHeader icon={<UserOutlined />} title="Profile" description="Your personal account — name, password, and appearance." />

      <Row gutter={[24, 24]}>
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

        <Col xs={24}>
          <Card title="Appearance">
            <Typography.Paragraph type="secondary" style={{ marginBottom: 16 }}>
              Choose how the panel looks. Saved to your account, so it follows you to any device.
            </Typography.Paragraph>
            <div className="flex flex-wrap gap-3">
              {THEME_KEYS.map((key) => {
                const swatch = THEME_SWATCHES[key];
                const isActive = key === activeTheme;
                return (
                  <button
                    key={key}
                    type="button"
                    onClick={() => selectTheme(key)}
                    disabled={savingTheme !== null}
                    className="flex w-36 flex-col gap-2 rounded-md border p-3 text-left transition-shadow hover:shadow-sm"
                    style={{ borderColor: isActive ? swatch.accent : undefined, borderWidth: isActive ? 2 : 1 }}
                  >
                    <div className="flex h-12 items-center justify-center gap-1.5 rounded" style={{ backgroundColor: swatch.bg, border: "1px solid rgba(0,0,0,0.08)" }}>
                      <span className="h-3 w-3 rounded-full" style={{ backgroundColor: swatch.primary }} />
                      <span className="h-3 w-3 rounded-full" style={{ backgroundColor: swatch.accent }} />
                    </div>
                    <span className="flex items-center justify-between text-sm font-medium">
                      {THEME_LABELS[key]}
                      {isActive && <CheckOutlined style={{ color: swatch.accent }} />}
                    </span>
                  </button>
                );
              })}
            </div>
          </Card>
        </Col>
      </Row>
    </div>
  );
}

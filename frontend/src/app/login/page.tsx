"use client";

import { useState } from "react";
import { Button, Card, Form, Input, Typography, message } from "antd";
import { MessageOutlined } from "@ant-design/icons";
import { useRouter } from "next/navigation";

import { apiGet, apiPost, ApiError } from "@/lib/api";
import { useAuth, type CurrentUser } from "@/context/AuthContext";
import { THEME_PRIMARY } from "@/lib/theme";

type LoginFormValues = {
  login: string;
  password: string;
};

export default function LoginPage() {
  const [submitting, setSubmitting] = useState(false);
  const router = useRouter();
  const { refresh } = useAuth();

  async function handleSubmit(values: LoginFormValues) {
    setSubmitting(true);
    try {
      await apiPost("/api/auth/login", values);
      const me = await apiGet<CurrentUser>("/api/auth/me");
      await refresh();
      // Agents land directly on the Chat List; Admin/Super Admin land on
      // the Overview dashboard (overview.md §6.0/§8).
      router.push(me.role === "agent" ? "/chats" : "/dashboard");
    } catch (err) {
      const detail = err instanceof ApiError ? err.message : "Something went wrong";
      message.error(detail === "invalid_credentials" ? "Incorrect email or password" : detail);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex h-screen items-stretch bg-neutral-50 dark:bg-neutral-950">
      <div
        className="hidden w-1/2 flex-col justify-center gap-4 px-16 text-white lg:flex"
        style={{ background: `linear-gradient(135deg, ${THEME_PRIMARY}, #4338CA)` }}
      >
        <div className="flex items-center gap-2 text-2xl font-semibold">
          <MessageOutlined />
          LiveChat
        </div>
        <Typography.Paragraph style={{ color: "rgba(255,255,255,0.85)", fontSize: 16, maxWidth: 420 }}>
          Real-time customer care for every brand you run — chat, automate, and route conversations from one panel.
        </Typography.Paragraph>
      </div>

      <div className="flex flex-1 items-center justify-center p-6">
        <Card style={{ width: 360 }}>
          <Typography.Title level={3} style={{ textAlign: "center", marginBottom: 24 }}>
            <span className="lg:hidden">LiveChat</span>
            <span className="hidden lg:inline">Log in</span>
          </Typography.Title>
          <Form layout="vertical" onFinish={handleSubmit} disabled={submitting}>
            <Form.Item name="login" label="Email" rules={[{ required: true, type: "email" }]}>
              <Input autoFocus />
            </Form.Item>
            <Form.Item name="password" label="Password" rules={[{ required: true }]}>
              <Input.Password />
            </Form.Item>
            <Form.Item style={{ marginBottom: 0 }}>
              <Button type="primary" htmlType="submit" block loading={submitting}>
                Log in
              </Button>
            </Form.Item>
          </Form>
        </Card>
      </div>
    </div>
  );
}

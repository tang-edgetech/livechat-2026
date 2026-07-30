"use client";

import { useState } from "react";
import { Button, Card, Form, Input, Typography, message } from "antd";
import { useRouter } from "next/navigation";

import { apiGet, apiPost, ApiError } from "@/lib/api";
import { useAuth, type CurrentUser } from "@/context/AuthContext";

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
      message.error(detail === "invalid_credentials" ? "Incorrect username/email or password" : detail);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex h-screen items-center justify-center bg-neutral-50 dark:bg-neutral-950">
      <Card style={{ width: 360 }}>
        <Typography.Title level={3} style={{ textAlign: "center", marginBottom: 24 }}>
          LiveChat
        </Typography.Title>
        <Form layout="vertical" onFinish={handleSubmit} disabled={submitting}>
          <Form.Item name="login" label="Username or email" rules={[{ required: true }]}>
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
  );
}

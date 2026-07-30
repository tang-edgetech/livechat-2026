"use client";

import { useEffect, useState } from "react";
import { Button, Card, Form, Input, Select, Typography, message } from "antd";
import { useRouter } from "next/navigation";

import { useAuth } from "@/context/AuthContext";
import { apiGet, apiPost, ApiError } from "@/lib/api";
import type { Merchant } from "@/lib/types";

type FormValues = {
  username: string;
  email: string;
  displayName: string;
  password: string;
  role?: "admin" | "agent";
  merchantUuids: string[];
};

export default function CreateUserPage() {
  const router = useRouter();
  const { user } = useAuth();
  const isSuperAdmin = user?.role === "super_admin";

  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    apiGet<{ merchants: Merchant[] }>("/api/merchants").then((res) => setMerchants(res.merchants));
  }, []);

  async function handleSubmit(values: FormValues) {
    setSubmitting(true);
    try {
      await apiPost("/api/users", {
        ...values,
        role: isSuperAdmin ? values.role : "agent",
      });
      message.success("User created");
      router.push("/settings");
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "Could not create user");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Card title="Create User" style={{ maxWidth: 480 }}>
      <Form layout="vertical" onFinish={handleSubmit} disabled={submitting}>
        <Form.Item name="displayName" label="Display name" rules={[{ required: true }]}>
          <Input />
        </Form.Item>
        <Form.Item name="username" label="Username" rules={[{ required: true }]}>
          <Input />
        </Form.Item>
        <Form.Item name="email" label="Email" rules={[{ required: true, type: "email" }]}>
          <Input />
        </Form.Item>
        <Form.Item name="password" label="Password" rules={[{ required: true }]} extra="Minimum 10 characters.">
          <Input.Password />
        </Form.Item>

        {isSuperAdmin && (
          <Form.Item name="role" label="Role" rules={[{ required: true }]} initialValue="agent">
            <Select
              options={[
                { value: "admin", label: "Admin" },
                { value: "agent", label: "Agent" },
              ]}
            />
          </Form.Item>
        )}

        <Form.Item
          name="merchantUuids"
          label="Merchants"
          extra={
            isSuperAdmin
              ? "Leave empty when creating a Super Admin."
              : "Only merchants you have access to are listed."
          }
        >
          <Select
            mode="multiple"
            placeholder="Select merchant(s)"
            options={merchants.map((m) => ({ value: m.uuid, label: m.name }))}
          />
        </Form.Item>

        <Typography.Paragraph type="secondary">
          You&apos;re setting this account&apos;s initial password directly — share it with them out of band.
        </Typography.Paragraph>

        <Button type="primary" htmlType="submit" loading={submitting}>
          Create
        </Button>
      </Form>
    </Card>
  );
}

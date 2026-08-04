"use client";

import { useEffect, useState } from "react";
import { Button, Card, Form, Input, Select, message } from "antd";
import { useRouter } from "next/navigation";

import { apiGet, apiPost, ApiError } from "@/lib/api";
import type { StaffUser } from "@/lib/types";

type FormValues = {
  name: string;
  code: string;
  initialAdminUuid?: string;
};

export default function CreateMerchantPage() {
  const router = useRouter();
  const [admins, setAdmins] = useState<StaffUser[]>([]);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    apiGet<{ users: StaffUser[] }>("/api/users").then((res) =>
      setAdmins(res.users.filter((u) => u.role === "admin")),
    );
  }, []);

  async function handleSubmit(values: FormValues) {
    setSubmitting(true);
    try {
      await apiPost("/api/merchants", values);
      message.success("Merchant created");
      router.push("/settings/merchants");
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "Could not create merchant");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Card title="Create Merchant" style={{ maxWidth: 480 }}>
      <Form layout="vertical" onFinish={handleSubmit} disabled={submitting}>
        <Form.Item name="name" label="Merchant / brand name" rules={[{ required: true }]}>
          <Input />
        </Form.Item>
        <Form.Item
          name="code"
          label="Code"
          rules={[{ required: true }]}
          extra="Used in the widget embed / API — lowercase, no spaces."
        >
          <Input />
        </Form.Item>
        <Form.Item
          name="initialAdminUuid"
          label="First Admin (optional)"
          extra="Grants this existing Admin access to the new merchant right away."
        >
          <Select
            allowClear
            placeholder="Select an Admin"
            options={admins.map((a) => ({ value: a.uuid, label: `${a.display_name} (${a.email})` }))}
          />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={submitting}>
          Create
        </Button>
      </Form>
    </Card>
  );
}

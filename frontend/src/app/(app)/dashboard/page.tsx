"use client";

import { Card, Typography } from "antd";

import { useAuth } from "@/context/AuthContext";

export default function DashboardPage() {
  const { user } = useAuth();

  return (
    <Card>
      <Typography.Title level={3}>Welcome, {user?.display_name}</Typography.Title>
      <Typography.Paragraph type="secondary">
        The Overview dashboard (Online Agents, Entries, Records, Traffic, Merchants, Active
        Chats, Bot Chats) is built in Phase 2.
      </Typography.Paragraph>
    </Card>
  );
}

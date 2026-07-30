"use client";

import { useCallback, useEffect, useState } from "react";
import { Card, Col, Row, Statistic, Typography } from "antd";

import { useAuth } from "@/context/AuthContext";
import { apiGet } from "@/lib/api";
import { useSocket } from "@/lib/socket";

type Summary = {
  onlineAgents: number;
  activeChats: number;
  entries: number;
  records: number;
  traffic: number;
  merchants: number;
  botChats: number;
};

// overview.md §9.1: Online Agents/Active Chats ride the WebSocket
// presence/chat_updated events already flowing for the Chat List, so
// they refresh as soon as a nudge arrives; Entries/Records/Traffic/Bot
// Chats are historical aggregates, polled every ~60s since sub-second
// accuracy doesn't matter for them.
const POLL_MS = 60_000;

export default function DashboardPage() {
  const { user } = useAuth();
  const [summary, setSummary] = useState<Summary | null>(null);

  const load = useCallback(() => {
    apiGet<Summary>("/api/dashboard/summary").then(setSummary);
  }, []);

  useEffect(() => {
    load();
    const interval = setInterval(load, POLL_MS);
    return () => clearInterval(interval);
  }, [load]);

  useSocket("/ws", (event) => {
    if (event.type === "chat_updated" || event.type === "presence_changed") {
      load();
    }
  });

  return (
    <div className="flex flex-col gap-6">
      <Typography.Title level={3}>Welcome, {user?.display_name}</Typography.Title>
      <Row gutter={[16, 16]}>
        <StatCard title="Online Agents" value={summary?.onlineAgents} />
        <StatCard title="Active Chats" value={summary?.activeChats} />
        <StatCard title="Entries" value={summary?.entries} />
        <StatCard title="Records" value={summary?.records} />
        <StatCard title="Traffic" value={summary?.traffic} />
        <StatCard title="Merchants / Brands" value={summary?.merchants} />
        <StatCard title="Bot Chats" value={summary?.botChats} />
      </Row>
    </div>
  );
}

function StatCard({ title, value }: { title: string; value: number | undefined }) {
  return (
    <Col xs={24} sm={12} md={8} lg={6}>
      <Card>
        <Statistic title={title} value={value ?? "—"} />
      </Card>
    </Col>
  );
}

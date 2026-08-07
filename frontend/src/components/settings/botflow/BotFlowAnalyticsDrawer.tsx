"use client";

import { useEffect, useState } from "react";
import { Drawer, Statistic, Table, Typography } from "antd";

import { apiGet } from "@/lib/api";
import type { BotFlow, BotFlowAnalytics } from "@/lib/automationTypes";

function pct(rate: number | null): string {
  return rate === null ? "—" : `${(rate * 100).toFixed(1)}%`;
}

// Bot Analytics' read-side UI — completion/handoff/abandonment rate plus
// per-node drop-off, so an Admin can see whether a flow is actually
// working instead of building it blind (item: "bot performance
// visibility"). A Drawer rather than a route: no frontend/src/lib/
// routes.ts back-button entry needed for what's just a read-only detail
// view over an existing row.
export function BotFlowAnalyticsDrawer({
  flow,
  open,
  onClose,
}: {
  flow: BotFlow | null;
  open: boolean;
  onClose: () => void;
}) {
  const [data, setData] = useState<BotFlowAnalytics | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open || !flow) return;
    setLoading(true);
    setData(null);
    apiGet<BotFlowAnalytics>(`/api/bot-flows/${flow.id}/analytics`)
      .then(setData)
      .finally(() => setLoading(false));
  }, [open, flow]);

  return (
    <Drawer title={flow ? `Analytics — ${flow.name}` : "Analytics"} open={open} onClose={onClose} width={480} loading={loading}>
      {data && (
        <div className="flex flex-col gap-6">
          <div className="grid grid-cols-2 gap-4">
            <Statistic title="Total Runs" value={data.total_runs} />
            <Statistic title="Completion Rate" value={pct(data.completion_rate)} />
            <Statistic title="Handoff Rate" value={pct(data.handoff_rate)} />
            <Statistic title="Abandonment Rate" value={pct(data.abandonment_rate)} />
          </div>

          <div>
            <Typography.Text strong>Outcome breakdown</Typography.Text>
            <Table
              size="small"
              className="mt-2"
              pagination={false}
              rowKey="label"
              dataSource={[
                { label: "Active (still in progress)", count: data.active_runs },
                { label: "Completed", count: data.completed_runs },
                { label: "Handoff", count: data.handoff_runs },
                { label: "Closed", count: data.closed_runs },
                { label: "Abandoned", count: data.abandoned_runs },
              ]}
              columns={[
                { title: "Outcome", dataIndex: "label" },
                { title: "Chats", dataIndex: "count", width: 80 },
              ]}
            />
          </div>

          <div>
            <Typography.Text strong>Drop-off by step</Typography.Text>
            {data.mode === "ai_passthrough" ? (
              <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
                AI passthrough flows have no node graph to attribute drop-off to.
              </Typography.Paragraph>
            ) : (
              <Table
                size="small"
                className="mt-2"
                pagination={false}
                rowKey="node_id"
                dataSource={data.drop_off_nodes ?? []}
                locale={{ emptyText: "No drop-off recorded yet" }}
                columns={[
                  { title: "Step", dataIndex: "label" },
                  { title: "Visitors lost here", dataIndex: "count", width: 130 },
                ]}
              />
            )}
          </div>
        </div>
      )}
    </Drawer>
  );
}

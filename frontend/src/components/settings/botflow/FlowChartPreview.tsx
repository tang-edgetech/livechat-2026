import { DownOutlined } from "@ant-design/icons";
import { Tag } from "antd";

import { stepLabel, type BuilderStep } from "./stepTypes";

// Read-only, secondary view (overview.md §6.4) — generated straight from
// the same step list that runs the flow, so there's no separate diagram
// to hand-maintain. Not where anyone builds/edits; just a sanity check.
export function FlowChartPreview({ steps }: { steps: BuilderStep[] }) {
  if (steps.length === 0) {
    return <p className="text-neutral-500">Add steps to see the flow chart.</p>;
  }

  return (
    <div className="flex flex-col items-center gap-1 py-2">
      <Tag color="green">Trigger: chat starts</Tag>
      <DownOutlined className="text-neutral-400" />
      {steps.map((step, i) => (
        <div key={step.id} className="flex flex-col items-center gap-1">
          <div className="rounded-lg border border-black/10 bg-white px-4 py-2 text-center shadow-sm dark:border-white/10 dark:bg-neutral-800">
            <div className="text-xs text-neutral-400">Step {i + 1}</div>
            <div className="font-medium">{stepLabel(step.type)}</div>
            {step.type === "send_message" && (
              <div className="max-w-[220px] truncate text-xs text-neutral-500">&quot;{String(step.config.message ?? "")}&quot;</div>
            )}
            {step.type === "ask_question" && (
              <div className="max-w-[220px] truncate text-xs text-neutral-500">&quot;{String(step.config.message ?? "")}&quot;</div>
            )}
          </div>
          {step.type === "condition" ? (
            <div className="flex items-center gap-4 text-xs text-neutral-500">
              <span>✓ yes → next step</span>
              <span>✕ no → step {steps.findIndex((s) => s.id === step.falseTarget) + 1 || "(end)"}</span>
            </div>
          ) : null}
          {i < steps.length - 1 && step.type !== "handoff_to_agent" && step.type !== "close_chat" && (
            <DownOutlined className="text-neutral-400" />
          )}
        </div>
      ))}
    </div>
  );
}

import type { ReactNode } from "react";
import {
  ApiOutlined,
  BranchesOutlined,
  CloseCircleOutlined,
  MessageOutlined,
  QuestionCircleOutlined,
  UserSwitchOutlined,
} from "@ant-design/icons";

import type { FlowNode } from "@/lib/automationTypes";

// The six step buttons from overview.md §6.4 — deliberately not a "type"
// dropdown of raw node names. set_variable/delay exist in the engine
// (botengine) but aren't offered here; they're not part of the plain-
// language builder the doc specifies.
export const STEP_TYPES: { type: FlowNode["type"]; label: string; description: string; icon: ReactNode }[] = [
  { type: "send_message", label: "Send a message", description: "Say something to the visitor.", icon: <MessageOutlined /> },
  { type: "ask_question", label: "Ask a question", description: "Capture what the visitor types back.", icon: <QuestionCircleOutlined /> },
  { type: "condition", label: "Check something", description: "Branch based on an earlier answer.", icon: <BranchesOutlined /> },
  { type: "call_integration", label: "Connect to another system", description: "Call an AI or third-party service.", icon: <ApiOutlined /> },
  { type: "handoff_to_agent", label: "Hand over to a real person", description: "End the bot, route to an Agent.", icon: <UserSwitchOutlined /> },
  { type: "close_chat", label: "End the chat", description: "Close the conversation.", icon: <CloseCircleOutlined /> },
];

export function stepLabel(type: FlowNode["type"]): string {
  return STEP_TYPES.find((s) => s.type === type)?.label ?? type;
}

export function stepIcon(type: FlowNode["type"]): ReactNode {
  return STEP_TYPES.find((s) => s.type === type)?.icon ?? null;
}

// A node with no outgoing connection at all (not just "no more steps
// after it") — handoff_to_agent/close_chat end the flow; every other
// type always has exactly one "next" output, condition has two.
export function isTerminalType(type: FlowNode["type"]): boolean {
  return type === "handoff_to_agent" || type === "close_chat";
}

let counter = 0;
export function newStepId(): string {
  counter += 1;
  return `step_${Date.now()}_${counter}`;
}

// Fresh node placed a bit right of wherever the admin last placed one
// (typically the selected node), nudged downward until it clears any
// node already sitting there — e.g. adding a step while the flow's
// *first* step is still selected would otherwise land exactly on top of
// whatever already follows it. Dragging to a nicer layout afterward is
// expected either way; this just avoids a straight-up overlap.
export function createNode(type: FlowNode["type"], occupied: { x: number; y: number }[], afterPosition?: { x: number; y: number }): FlowNode {
  const base = afterPosition ? { x: afterPosition.x + 260, y: afterPosition.y } : { x: 80, y: 80 };
  let position = base;
  while (occupied.some((p) => Math.abs(p.x - position.x) < 40 && Math.abs(p.y - position.y) < 40)) {
    position = { x: position.x, y: position.y + 150 };
  }
  return { id: newStepId(), type, config: {}, position };
}

// A flow saved before the visual canvas existed has no position at all
// (position is a canvas-only concern, ignored entirely by the engine) —
// this gives it a sane starting layout the first time it's opened here,
// via a simple BFS from entry: each hop right, siblings stacked down.
export function withAutoLayout(nodes: FlowNode[], entry: string): FlowNode[] {
  if (nodes.every((n) => n.position)) return nodes;

  const byID = new Map(nodes.map((n) => [n.id, n]));
  const positioned = new Map<string, { x: number; y: number }>();
  const columnNextY = new Map<number, number>();

  function place(id: string, depth: number) {
    if (positioned.has(id) || !byID.has(id)) return;
    const y = columnNextY.get(depth) ?? 80;
    positioned.set(id, { x: 80 + depth * 260, y });
    columnNextY.set(depth, y + 140);

    const node = byID.get(id)!;
    if (node.type === "condition") {
      if (node.branches?.true) place(node.branches.true, depth + 1);
      if (node.branches?.false) place(node.branches.false, depth + 1);
    } else if (node.next) {
      place(node.next, depth + 1);
    }
  }

  if (entry) place(entry, 0);
  for (const n of nodes) place(n.id, 0); // anything unreached by the walk still gets a slot

  return nodes.map((n) => ({ ...n, position: n.position ?? positioned.get(n.id) ?? { x: 80, y: 80 } }));
}

export type FlowIssue = { nodeId: string; message: string };

// Traces every path reachable from entry and flags any that dead-end
// without reaching a terminal step (handoff_to_agent/close_chat) — the
// exact shape of bug Bot Analytics uncovered live (a flow that just runs
// off the end leaves the chat permanently stuck in 'bot' status). Also
// flags a step that isn't connected to the start at all, since it will
// simply never run. Ai_passthrough flows have no graph and are never
// passed here.
export function validateFlowGraph(steps: FlowNode[], entry: string): FlowIssue[] {
  const issues: FlowIssue[] = [];
  const byId = new Map(steps.map((s) => [s.id, s]));
  const label = (s: FlowNode) => `"${stepLabel(s.type)}" (step ${steps.indexOf(s) + 1})`;
  const visited = new Set<string>();
  const queue: string[] = entry ? [entry] : [];

  function checkTarget(node: FlowNode, target: string | undefined, branchName?: string) {
    const suffix = branchName ? ` — the "${branchName}" branch` : "";
    if (!target) {
      issues.push({
        nodeId: node.id,
        message: `${label(node)}${suffix} doesn't continue to another step and isn't an ending — connect it onward, or end with "Hand over to a real person" / "End the chat".`,
      });
    } else if (!byId.has(target)) {
      issues.push({ nodeId: node.id, message: `${label(node)}${suffix} points to a step that no longer exists.` });
    } else {
      queue.push(target);
    }
  }

  while (queue.length > 0) {
    const id = queue.shift()!;
    if (visited.has(id)) continue;
    visited.add(id);
    const node = byId.get(id);
    if (!node || isTerminalType(node.type)) continue;

    if (node.type === "condition") {
      checkTarget(node, node.branches?.true, "Yes");
      checkTarget(node, node.branches?.false, "No");
    } else {
      checkTarget(node, node.next);
    }
  }

  for (const s of steps) {
    if (!visited.has(s.id)) {
      issues.push({ nodeId: s.id, message: `${label(s)} isn't connected to the start — it will never run.` });
    }
  }

  return issues;
}

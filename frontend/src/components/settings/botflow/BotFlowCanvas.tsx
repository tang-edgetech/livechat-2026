"use client";

import { useCallback, useMemo } from "react";
import {
  Background,
  Controls,
  Handle,
  Position,
  ReactFlow,
  ReactFlowProvider,
  type Connection,
  type Edge,
  type Node,
  type NodeChange,
  type NodeProps,
  type OnConnect,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Tag } from "antd";

import type { FlowNode } from "@/lib/automationTypes";
import { isTerminalType, stepIcon, stepLabel } from "./stepTypes";

type StepNodeData = { flowNode: FlowNode; isEntry: boolean };
type StepNode = Node<StepNodeData, "step">;

function preview(flowNode: FlowNode): string | null {
  switch (flowNode.type) {
    case "send_message":
    case "ask_question":
      return (flowNode.config.message as string) || null;
    case "call_integration":
      return flowNode.config.integrationId ? `Connection #${flowNode.config.integrationId}` : "No connection chosen yet";
    default:
      return null;
  }
}

function StepNodeCard({ data, selected }: NodeProps<StepNode>) {
  const { flowNode, isEntry } = data;
  const text = preview(flowNode);

  return (
    <div
      className={`w-56 rounded-lg border bg-white px-3 py-2 shadow-sm dark:bg-neutral-900 ${
        selected ? "border-blue-500 ring-2 ring-blue-500/30" : "border-black/10 dark:border-white/10"
      }`}
    >
      <Handle type="target" position={Position.Top} style={{ background: "#999" }} />
      <div className="flex items-center gap-2">
        <span className="text-[15px]">{stepIcon(flowNode.type)}</span>
        <span className="truncate text-[13px] font-medium">{stepLabel(flowNode.type)}</span>
        {isEntry && <Tag color="green" style={{ marginLeft: "auto", marginInlineEnd: 0 }}>Start</Tag>}
      </div>
      {text && <div className="mt-1 truncate text-xs text-neutral-500">&quot;{text}&quot;</div>}

      {flowNode.type === "condition" ? (
        <>
          <Handle type="source" position={Position.Bottom} id="true" style={{ left: "30%", background: "#22c55e" }} />
          <Handle type="source" position={Position.Bottom} id="false" style={{ left: "70%", background: "#ef4444" }} />
          <div className="mt-1 flex justify-between text-[10px] text-neutral-400">
            <span>Yes</span>
            <span>No</span>
          </div>
        </>
      ) : !isTerminalType(flowNode.type) ? (
        <Handle type="source" position={Position.Bottom} id="next" style={{ background: "#999" }} />
      ) : null}
    </div>
  );
}

const nodeTypes = { step: StepNodeCard };

export function BotFlowCanvas({
  steps,
  entry,
  selectedId,
  onSelect,
  onMove,
  onConnect,
  onDeleteNodes,
  onDeleteEdges,
}: {
  steps: FlowNode[];
  entry: string;
  selectedId: string | null;
  onSelect: (id: string | null) => void;
  onMove: (id: string, position: { x: number; y: number }) => void;
  onConnect: (connection: Connection) => void;
  onDeleteNodes: (ids: string[]) => void;
  onDeleteEdges: (edges: { source: string; sourceHandle: string | null }[]) => void;
}) {
  const nodes: StepNode[] = useMemo(
    () =>
      steps.map((s) => ({
        id: s.id,
        type: "step",
        position: s.position ?? { x: 80, y: 80 },
        data: { flowNode: s, isEntry: s.id === entry },
        selected: s.id === selectedId,
      })),
    [steps, entry, selectedId],
  );

  const edges: Edge[] = useMemo(() => {
    const es: Edge[] = [];
    for (const s of steps) {
      if (s.type === "condition") {
        if (s.branches?.true) es.push({ id: `${s.id}-true`, source: s.id, sourceHandle: "true", target: s.branches.true, label: "Yes", style: { stroke: "#22c55e" } });
        if (s.branches?.false) es.push({ id: `${s.id}-false`, source: s.id, sourceHandle: "false", target: s.branches.false, label: "No", style: { stroke: "#ef4444" } });
      } else if (s.next) {
        es.push({ id: `${s.id}-next`, source: s.id, sourceHandle: "next", target: s.next });
      }
    }
    return es;
  }, [steps]);

  const handleNodesChange = useCallback(
    (changes: NodeChange<StepNode>[]) => {
      for (const change of changes) {
        if (change.type === "position" && change.position) {
          onMove(change.id, change.position);
        }
        if (change.type === "select" && change.selected) {
          onSelect(change.id);
        }
        if (change.type === "remove") {
          onDeleteNodes([change.id]);
        }
      }
    },
    [onMove, onSelect, onDeleteNodes],
  );

  const handleConnect: OnConnect = useCallback((connection) => onConnect(connection), [onConnect]);

  const handleEdgesDelete = useCallback(
    (deleted: Edge[]) => {
      onDeleteEdges(deleted.map((e) => ({ source: e.source, sourceHandle: e.sourceHandle ?? null })));
    },
    [onDeleteEdges],
  );

  return (
    <div className="h-[520px] w-full rounded-lg border border-black/10 dark:border-white/10">
      <ReactFlowProvider>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          onNodesChange={handleNodesChange}
          onConnect={handleConnect}
          onEdgesDelete={handleEdgesDelete}
          onPaneClick={() => onSelect(null)}
          deleteKeyCode={["Backspace", "Delete"]}
          fitView
        >
          <Background />
          <Controls />
        </ReactFlow>
      </ReactFlowProvider>
    </div>
  );
}

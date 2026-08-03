import type { FlowNode } from "@/lib/automationTypes";

// The six step buttons from overview.md §6.4 — deliberately not a "type"
// dropdown of raw node names. set_variable/delay exist in the engine
// (botengine) but aren't offered here; they're not part of the plain-
// language builder the doc specifies.
export const STEP_TYPES: { type: FlowNode["type"]; label: string; description: string }[] = [
  { type: "send_message", label: "Send a message", description: "Say something to the visitor." },
  { type: "ask_question", label: "Ask a question", description: "Capture what the visitor types back." },
  { type: "condition", label: "Check something", description: "Branch based on an earlier answer." },
  { type: "call_integration", label: "Connect to another system", description: "Call an AI or third-party service." },
  { type: "handoff_to_agent", label: "Hand over to a real person", description: "End the bot, route to an Agent." },
  { type: "close_chat", label: "End the chat", description: "Close the conversation." },
];

export function stepLabel(type: FlowNode["type"]): string {
  return STEP_TYPES.find((s) => s.type === type)?.label ?? type;
}

export type BuilderStep = FlowNode & { falseTarget?: string };

// Turns the ordered step list into the flow JSON the engine actually
// runs (overview.md §4/§6.4) — array order becomes `next` pointers,
// condition steps get `branches` (true = the following step, false =
// wherever the admin picked).
export function stepsToFlow(steps: BuilderStep[]): { entry: string; nodes: FlowNode[] } {
  const nodes: FlowNode[] = steps.map((step, i) => {
    const nextID = steps[i + 1]?.id;
    if (step.type === "condition") {
      return { id: step.id, type: step.type, config: step.config, branches: { true: nextID, false: step.falseTarget } };
    }
    if (step.type === "handoff_to_agent" || step.type === "close_chat") {
      return { id: step.id, type: step.type, config: step.config };
    }
    return { id: step.id, type: step.type, config: step.config, next: nextID };
  });
  return { entry: steps[0]?.id ?? "", nodes };
}

export function flowToSteps(flow: { entry: string; nodes: FlowNode[] }): BuilderStep[] {
  // Reconstruct array order by walking `next`/`branches.true` from entry
  // — good enough for the linear-with-one-side-branch shape this builder
  // produces; a flow authored elsewhere with a more exotic graph shape
  // just won't round-trip perfectly into the builder (still runs fine).
  const byID = new Map(flow.nodes.map((n) => [n.id, n]));
  const steps: BuilderStep[] = [];
  const seen = new Set<string>();
  let current = flow.entry;
  while (current && byID.has(current) && !seen.has(current)) {
    seen.add(current);
    const node = byID.get(current)!;
    const falseTarget = node.type === "condition" ? node.branches?.false : undefined;
    steps.push({ ...node, falseTarget });
    current = node.type === "condition" ? node.branches?.true ?? "" : node.next ?? "";
  }
  return steps;
}

let counter = 0;
export function newStepId(): string {
  counter += 1;
  return `step_${Date.now()}_${counter}`;
}

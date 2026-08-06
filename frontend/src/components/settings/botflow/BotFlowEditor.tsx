"use client";

import { useEffect, useState } from "react";
import { AutoComplete, Button, Card, Checkbox, Input, InputNumber, Modal, Segmented, Select, Space, Switch, Typography, message } from "antd";
import { DeleteOutlined, PlusOutlined } from "@ant-design/icons";
import type { Connection } from "@xyflow/react";
import { useRouter } from "next/navigation";

import { apiGet, apiPost, apiPatch, ApiError } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";
import type { BotFlow, ConditionRule, ConditionSet, FlowDef, FlowNode, PassthroughConfig, TriggerDef } from "@/lib/automationTypes";
import type { Merchant } from "@/lib/types";
import { BotFlowCanvas } from "./BotFlowCanvas";
import { STEP_TYPES, createNode, isTerminalType, newStepId, stepLabel, withAutoLayout } from "./stepTypes";

// Always available to reference in a Condition step alongside "answer_<id>"
// (a captured question answer) — computed by the engine itself, nothing
// to configure (overview.md §6.9.1/§12).
const BUILT_IN_FIELDS = [
  { value: "visitor_tier", label: "Visitor tier (Normal/VIP)" },
  { value: "chat_duration_seconds", label: "Chat duration (seconds)" },
];

// Plain-language validation presets for "Ask a question" (overview.md
// §6.0 — never a bare regex field by default) — "Custom pattern" is the
// escape hatch for anyone who does want to hand-write one.
const VALIDATION_PRESETS = [
  { value: "", label: "Any answer" },
  { value: "^[0-9]+$", label: "Numbers only" },
  { value: "^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$", label: "Email address" },
  { value: "^[0-9+\\-\\s()]{7,}$", label: "Phone number" },
  { value: "custom", label: "Custom pattern..." },
];

function parseExistingFlow(existing?: BotFlow): FlowDef | null {
  if (!existing) return null;
  try {
    return JSON.parse(existing.flow) as FlowDef;
  } catch {
    return null;
  }
}

function buildTemplate(): { steps: FlowNode[]; entry: string } {
  const ids = [newStepId(), newStepId(), newStepId(), newStepId()];
  const steps: FlowNode[] = [
    { id: ids[0], type: "ask_question", config: { message: "What's your name?", variable: `answer_${ids[0]}` }, next: ids[1], position: { x: 80, y: 80 } },
    { id: ids[1], type: "ask_question", config: { message: "And your email address?", variable: `answer_${ids[1]}` }, next: ids[2], position: { x: 340, y: 80 } },
    { id: ids[2], type: "send_message", config: { message: "Thanks! Connecting you to a member of our team now." }, next: ids[3], position: { x: 600, y: 80 } },
    { id: ids[3], type: "handoff_to_agent", config: {}, position: { x: 860, y: 80 } },
  ];
  return { steps, entry: ids[0] };
}

export function BotFlowEditor({ existing }: { existing?: BotFlow }) {
  const router = useRouter();
  const { user } = useAuth();
  const isSuperAdmin = user?.role === "super_admin";

  const [name, setName] = useState(existing?.name ?? "");
  const [isActive, setIsActive] = useState(existing?.is_active ?? true);
  const [isGlobal, setIsGlobal] = useState(existing?.is_global ?? false);
  const [merchantUuid, setMerchantUuid] = useState<string | undefined>(existing?.merchant_uuid ?? undefined);
  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [integrations, setIntegrations] = useState<{ id: number; name: string }[]>([]);

  const [usePageCondition, setUsePageCondition] = useState(false);
  const [pageContains, setPageContains] = useState("");
  const [audience, setAudience] = useState<"normal" | "vip" | "both">("normal");

  const [steps, setSteps] = useState<FlowNode[]>(() => {
    const flow = parseExistingFlow(existing);
    return flow ? withAutoLayout(flow.nodes ?? [], flow.entry ?? "") : [];
  });
  const [entry, setEntry] = useState<string>(() => parseExistingFlow(existing)?.entry ?? "");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const [flowMode, setFlowMode] = useState<"steps" | "ai_passthrough">(() => parseExistingFlow(existing)?.mode ?? "steps");
  const [passthrough, setPassthrough] = useState<PassthroughConfig>(() => parseExistingFlow(existing)?.passthrough ?? {});

  useEffect(() => {
    apiGet<{ merchants: Merchant[] }>("/api/merchants").then((res) => setMerchants(res.merchants));
    apiGet<{ integrations: { id: number; name: string }[] }>("/api/integrations").then((res) => setIntegrations(res.integrations));

    if (existing) {
      try {
        const trigger: TriggerDef = JSON.parse(existing.trigger);
        const pageRule = trigger.conditions?.rules?.find((r) => r.field === "page_url");
        if (pageRule) {
          // eslint-disable-next-line react-hooks/set-state-in-effect -- seeding the form from an existing flow on mount.
          setUsePageCondition(true);
          setPageContains(String(pageRule.value));
        }
        setAudience(trigger.audience ?? "normal");
      } catch {
        // ignore
      }
    }
  }, [existing]);

  function addStep(type: FlowNode["type"]) {
    const anchor = steps.find((s) => s.id === selectedId) ?? steps[steps.length - 1];
    const node = createNode(
      type,
      steps.map((s) => s.position ?? { x: 80, y: 80 }),
      anchor?.position,
    );
    setSteps((prev) => [...prev, node]);
    setSelectedId(node.id);
    if (!entry) setEntry(node.id);
    setPickerOpen(false);
  }

  function updateNode(id: string, patch: Partial<FlowNode>) {
    setSteps((prev) => prev.map((s) => (s.id === id ? { ...s, ...patch } : s)));
  }

  function moveNode(id: string, position: { x: number; y: number }) {
    setSteps((prev) => prev.map((s) => (s.id === id ? { ...s, position } : s)));
  }

  function connectNodes(connection: Connection) {
    const { source, sourceHandle, target } = connection;
    if (!target || source === target) return;
    setSteps((prev) =>
      prev.map((s) => {
        if (s.id !== source) return s;
        if (s.type === "condition") {
          const key = sourceHandle === "false" ? "false" : "true";
          return { ...s, branches: { ...s.branches, [key]: target } };
        }
        return { ...s, next: target };
      }),
    );
  }

  function removeEdges(deleted: { source: string; sourceHandle: string | null }[]) {
    setSteps((prev) =>
      prev.map((s) => {
        let next = s.next;
        let branches = s.branches;
        for (const e of deleted) {
          if (e.source !== s.id) continue;
          if (s.type === "condition") {
            branches = { ...branches, [e.sourceHandle === "false" ? "false" : "true"]: undefined };
          } else {
            next = undefined;
          }
        }
        return { ...s, next, branches };
      }),
    );
  }

  function removeNodes(ids: string[]) {
    const idSet = new Set(ids);
    const remaining = steps
      .filter((s) => !idSet.has(s.id))
      .map((s) => ({
        ...s,
        next: s.next && idSet.has(s.next) ? undefined : s.next,
        branches: s.branches
          ? {
              true: s.branches.true && idSet.has(s.branches.true) ? undefined : s.branches.true,
              false: s.branches.false && idSet.has(s.branches.false) ? undefined : s.branches.false,
            }
          : s.branches,
        config: s.config.retryFailNext && idSet.has(s.config.retryFailNext as string) ? { ...s.config, retryFailNext: undefined } : s.config,
      }));
    setSteps(remaining);
    if (entry && idSet.has(entry)) setEntry(remaining[0]?.id ?? "");
    if (selectedId && idSet.has(selectedId)) setSelectedId(null);
  }

  function applyTemplate() {
    const t = buildTemplate();
    setSteps(t.steps);
    setEntry(t.entry);
    setSelectedId(null);
  }

  const questionSteps = steps.filter((s) => s.type === "ask_question");
  const hasTerminal = steps.some((s) => isTerminalType(s.type));
  const selectedNode = steps.find((s) => s.id === selectedId) ?? null;

  async function save() {
    if (!name.trim()) {
      message.error("Please name this bot flow.");
      return;
    }
    if (flowMode === "ai_passthrough") {
      if (!passthrough.integrationId) {
        message.error("Choose which connection this flow should forward visitor messages to.");
        return;
      }
    } else {
      if (steps.length === 0) {
        message.error("Add at least one step.");
        return;
      }
      if (!entry) {
        message.error("Pick a starting step — click a node and use \"Set as start\", or add-step auto-picks the first one.");
        return;
      }
      if (!hasTerminal) {
        message.error('End the flow with "Hand over to a real person" or "End the chat".');
        return;
      }
    }
    if (!isGlobal && !merchantUuid) {
      message.error("Choose a merchant, or mark this as global.");
      return;
    }

    const conditions: ConditionSet = {
      logic: "and",
      rules: usePageCondition && pageContains ? [{ field: "page_url", operator: "contains", value: pageContains }] : [],
    };
    const trigger: TriggerDef = { type: "chat_start", conditions, audience };
    const flow: FlowDef =
      flowMode === "ai_passthrough" ? { entry: "", nodes: [], mode: "ai_passthrough", passthrough } : { entry, nodes: steps };

    setSubmitting(true);
    try {
      const payload = {
        name,
        trigger: JSON.stringify(trigger),
        flow: JSON.stringify(flow),
        integrationId: null,
        isGlobal,
        isActive,
        merchantUuid: isGlobal ? undefined : merchantUuid,
      };
      if (existing) {
        await apiPatch(`/api/bot-flows/${existing.id}`, payload);
      } else {
        await apiPost("/api/bot-flows", payload);
      }
      message.success("Bot flow saved");
      router.push("/settings/bot");
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "Could not save bot flow");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex flex-col gap-6 lg:flex-row">
      <div className="flex flex-1 flex-col gap-4">
        <Card title="Bot flow details">
          <Space orientation="vertical" style={{ width: "100%", maxWidth: 480 }}>
            <Input placeholder="Flow name (for your reference)" value={name} onChange={(e) => setName(e.target.value)} />
            <div className="flex items-center gap-2">
              <Switch checked={isActive} onChange={setIsActive} />
              <span>Active</span>
            </div>
            <Typography.Text strong>How does this flow work?</Typography.Text>
            <Segmented
              value={flowMode}
              onChange={(v) => setFlowMode(v as "steps" | "ai_passthrough")}
              options={[
                { label: "Step-by-step", value: "steps" },
                { label: "AI Passthrough", value: "ai_passthrough" },
              ]}
            />
            <Typography.Paragraph type="secondary" style={{ marginTop: -4, marginBottom: 0, fontSize: 12.5 }}>
              {flowMode === "steps"
                ? "A scripted sequence of steps you build below (ask questions, branch, hand off)."
                : "Every visitor message is forwarded to a connection (with the message and a session id) and whatever it replies with is sent back as-is — closer to handing the whole conversation to a live AI than running a script."}
            </Typography.Paragraph>
            {isSuperAdmin && (
              <Checkbox checked={isGlobal} onChange={(e) => setIsGlobal(e.target.checked)}>
                Apply to all merchants
              </Checkbox>
            )}
            {!isGlobal && (
              <Select
                placeholder="Merchant"
                style={{ width: "100%" }}
                value={merchantUuid}
                onChange={setMerchantUuid}
                options={merchants.map((m) => ({ value: m.uuid, label: m.name }))}
              />
            )}
          </Space>
        </Card>

        <Card title="When should this run?">
          <Typography.Text strong>Who is this for?</Typography.Text>
          <div style={{ marginTop: 8, marginBottom: 16 }}>
            <Segmented
              value={audience}
              onChange={(v) => setAudience(v as "normal" | "vip" | "both")}
              options={[
                { label: "Normal visitors", value: "normal" },
                { label: "VIP visitors", value: "vip" },
                { label: "Both", value: "both" },
              ]}
            />
            <Typography.Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0 }}>
              {audience === "normal" &&
                "Default: a VIP visitor skips this flow entirely and goes straight to an available agent (overview.md §6.9.1)."}
              {audience === "vip" &&
                "Only VIP visitors reach this flow; a Normal visitor never sees it. Use this for a VIP-specific concierge experience."}
              {audience === "both" && "Every visitor can reach this flow, regardless of tier."}
            </Typography.Paragraph>
          </div>

          <Checkbox checked={usePageCondition} onChange={(e) => setUsePageCondition(e.target.checked)}>
            Only when the page contains
          </Checkbox>
          {usePageCondition && (
            <Input
              style={{ marginTop: 8, maxWidth: 320 }}
              placeholder="e.g. /support"
              value={pageContains}
              onChange={(e) => setPageContains(e.target.value)}
            />
          )}
          {!usePageCondition && (
            <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
              Runs on every new chat for this merchant.
            </Typography.Paragraph>
          )}
        </Card>

        {flowMode === "ai_passthrough" ? (
          <Card title="AI Passthrough">
            <Space orientation="vertical" style={{ width: "100%", maxWidth: 480 }}>
              <Select
                placeholder="Which connection should receive every message?"
                style={{ width: "100%" }}
                value={passthrough.integrationId}
                onChange={(v) => setPassthrough({ ...passthrough, integrationId: v })}
                options={integrations.map((i) => ({ value: i.id, label: i.name }))}
                notFoundContent="No connections set up yet — ask your Super Admin to add one in Settings > Integration."
              />
              <Input.TextArea
                placeholder="Greeting message sent when the chat starts (optional)"
                rows={2}
                value={passthrough.greeting ?? ""}
                onChange={(e) => setPassthrough({ ...passthrough, greeting: e.target.value })}
              />
              <Checkbox
                checked={passthrough.logToAuditLog === true}
                onChange={(e) => setPassthrough({ ...passthrough, logToAuditLog: e.target.checked })}
              >
                Log every request &amp; response to Audit Logs (for debugging this connection)
              </Checkbox>
              <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
                Every visitor message is POSTed as <code>{"{ message, sessionId, visitorUuid }"}</code>; a reply of{" "}
                <code>{"{ message: \"...\" }"}</code> is sent back to the visitor as-is. Include{" "}
                <code>{"{ handoff: true }"}</code> in the response to route the chat to a human agent instead —
                exactly as if an operator claimed it.
              </Typography.Paragraph>
            </Space>
          </Card>
        ) : (
          <Card
            title="Flow"
            extra={
              <Space>
                {steps.length === 0 && (
                  <Button size="small" onClick={applyTemplate}>
                    Use a template
                  </Button>
                )}
                <Button size="small" type="primary" icon={<PlusOutlined />} onClick={() => setPickerOpen(true)}>
                  Add step
                </Button>
              </Space>
            }
          >
            {steps.length === 0 ? (
              <div className="flex h-40 items-center justify-center text-neutral-400">
                Add a step to start building this flow.
              </div>
            ) : (
              <>
                <Typography.Paragraph type="secondary" style={{ marginBottom: 8, fontSize: 12.5 }}>
                  Drag from a step&apos;s bottom dot to another step to connect them. Click a step to edit it on the
                  right. Select and press Delete to remove a step or a connection.
                </Typography.Paragraph>
                <BotFlowCanvas
                  steps={steps}
                  entry={entry}
                  selectedId={selectedId}
                  onSelect={setSelectedId}
                  onMove={moveNode}
                  onConnect={connectNodes}
                  onDeleteNodes={removeNodes}
                  onDeleteEdges={removeEdges}
                />
              </>
            )}
          </Card>
        )}

        <Button type="primary" loading={submitting} onClick={save} style={{ width: 160 }}>
          Save bot flow
        </Button>
      </div>

      {flowMode === "steps" && (
      <div className="w-full lg:w-96">
        <Card
          title={selectedNode ? stepLabel(selectedNode.type) : "Inspector"}
          extra={
            selectedNode && (
              <Space>
                {selectedNode.id !== entry && (
                  <Button size="small" onClick={() => setEntry(selectedNode.id)}>
                    Set as start
                  </Button>
                )}
                <Button type="text" danger icon={<DeleteOutlined />} onClick={() => removeNodes([selectedNode.id])} />
              </Space>
            )
          }
        >
          {selectedNode ? (
            <NodeInspector
              step={selectedNode}
              steps={steps}
              questionSteps={questionSteps}
              integrations={integrations}
              onChange={(patch) => updateNode(selectedNode.id, patch)}
            />
          ) : (
            <Typography.Paragraph type="secondary">
              Click a step on the canvas to edit its settings here.
            </Typography.Paragraph>
          )}
        </Card>
      </div>
      )}

      <Modal open={pickerOpen} onCancel={() => setPickerOpen(false)} footer={null} title="Add a step">
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          {STEP_TYPES.map((t) => (
            <button
              key={t.type}
              onClick={() => addStep(t.type)}
              className="rounded-lg border border-black/10 p-3 text-left hover:border-blue-500 dark:border-white/10"
            >
              <div className="font-medium">
                {t.icon} {t.label}
              </div>
              <div className="text-xs text-neutral-500">{t.description}</div>
            </button>
          ))}
        </div>
      </Modal>
    </div>
  );
}

function stepTargetOptions(steps: FlowNode[], excludeId: string) {
  return steps
    .filter((s) => s.id !== excludeId)
    .map((s) => ({ value: s.id, label: `${stepLabel(s.type)} (step ${steps.indexOf(s) + 1})` }));
}

function NodeInspector({
  step,
  steps,
  questionSteps,
  integrations,
  onChange,
}: {
  step: FlowNode;
  steps: FlowNode[];
  questionSteps: FlowNode[];
  integrations: { id: number; name: string }[];
  onChange: (patch: Partial<FlowNode>) => void;
}) {
  return (
    <>
      {step.type === "send_message" && (
        <Input.TextArea
          placeholder="Message to send"
          value={String(step.config.message ?? "")}
          onChange={(e) => onChange({ config: { ...step.config, message: e.target.value } })}
        />
      )}

      {step.type === "ask_question" && <AskQuestionFields step={step} steps={steps} onChange={onChange} />}

      {step.type === "condition" && <ConditionFields step={step} steps={steps} questionSteps={questionSteps} onChange={onChange} />}

      {step.type === "call_integration" && (
        <Space orientation="vertical" style={{ width: "100%" }}>
          <Select
            placeholder="Which connection?"
            style={{ width: "100%" }}
            value={step.config.integrationId as number | undefined}
            onChange={(v) => onChange({ config: { ...step.config, integrationId: v } })}
            options={integrations.map((i) => ({ value: i.id, label: i.name }))}
            notFoundContent="No connections set up yet — ask your Super Admin to add one in Settings > Integration."
          />
          <Input
            placeholder="Save response into variable (optional)"
            value={step.config.saveResponseAs as string | undefined}
            onChange={(e) => onChange({ config: { ...step.config, saveResponseAs: e.target.value } })}
          />
          <Input
            placeholder="Extract from response path, e.g. data.answer (optional)"
            value={step.config.responsePath as string | undefined}
            onChange={(e) => onChange({ config: { ...step.config, responsePath: e.target.value } })}
          />
          <Checkbox
            checked={step.config.sendAsMessage !== false}
            onChange={(e) => onChange({ config: { ...step.config, sendAsMessage: e.target.checked } })}
          >
            Send response as a message to the visitor
          </Checkbox>
          <Checkbox
            checked={step.config.logToAuditLog === true}
            onChange={(e) => onChange({ config: { ...step.config, logToAuditLog: e.target.checked } })}
          >
            Log the request &amp; response to Audit Logs (for debugging this connection)
          </Checkbox>
          <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
            Saving a response into a variable also sets &quot;&lt;variable&gt;_ok&quot; to true/false based on
            whether the call succeeded — branch on it with a Condition step to fall back to Handoff to Agent on
            failure.
          </Typography.Paragraph>
        </Space>
      )}

      {(step.type === "handoff_to_agent" || step.type === "close_chat") && (
        <Typography.Text type="secondary">No extra setup needed for this step.</Typography.Text>
      )}
    </>
  );
}

// Never a bare regex field by default (overview.md §6.0) — a plain-
// language preset picker that resolves to a regex under the hood, with
// "Custom pattern..." as the escape hatch. Retry limit + fail target only
// appear once a format is actually required.
function AskQuestionFields({
  step,
  steps,
  onChange,
}: {
  step: FlowNode;
  steps: FlowNode[];
  onChange: (patch: Partial<FlowNode>) => void;
}) {
  const pattern = (step.config.validationPattern as string | undefined) ?? "";
  const knownPreset = VALIDATION_PRESETS.find((p) => p.value === pattern && p.value !== "custom");
  const presetValue = pattern === "" ? "" : knownPreset ? knownPreset.value : "custom";

  return (
    <Space orientation="vertical" style={{ width: "100%" }}>
      <Input.TextArea
        placeholder="Question to ask"
        value={String(step.config.message ?? "")}
        onChange={(e) => onChange({ config: { ...step.config, message: e.target.value, variable: `answer_${step.id}` } })}
      />
      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
        The visitor&apos;s reply will be remembered as &quot;the answer to this question&quot;.
      </Typography.Text>

      <Typography.Text>Require a specific format?</Typography.Text>
      <Select
        style={{ width: "100%" }}
        value={presetValue}
        onChange={(v) => onChange({ config: { ...step.config, validationPattern: v === "custom" ? pattern || " " : v } })}
        options={VALIDATION_PRESETS}
      />
      {presetValue === "custom" && (
        <Input
          placeholder="Custom regex pattern"
          value={pattern}
          onChange={(e) => onChange({ config: { ...step.config, validationPattern: e.target.value } })}
        />
      )}
      {presetValue !== "" && (
        <>
          <InputNumber
            style={{ width: "100%" }}
            placeholder="Max retries before giving up (optional — blank = unlimited)"
            min={1}
            value={step.config.maxRetries as number | undefined}
            onChange={(v) => onChange({ config: { ...step.config, maxRetries: v ?? undefined } })}
          />
          <Select
            placeholder="If retries run out, skip to step..."
            style={{ width: "100%" }}
            value={step.config.retryFailNext as string | undefined}
            onChange={(v) => onChange({ config: { ...step.config, retryFailNext: v } })}
            options={stepTargetOptions(steps, step.id)}
          />
        </>
      )}
    </Space>
  );
}

// Multi-rule AND/OR condition builder — the exact shape (ConditionSet:
// {logic, rules}) already used by Greeting Rules'/Bot triggers' own page-
// URL/time-of-day conditions, now also available mid-flow (overview.md
// §12). A rule's field can be a captured question answer, a built-in
// (visitor_tier/chat_duration_seconds), or any other variable name typed
// directly (e.g. a call_integration saveResponseAs/"_ok" flag). "Yes"/"No"
// connections are drawn on the canvas itself now, not picked from a list.
function ConditionFields({
  step,
  steps,
  questionSteps,
  onChange,
}: {
  step: FlowNode;
  steps: FlowNode[];
  questionSteps: FlowNode[];
  onChange: (patch: Partial<FlowNode>) => void;
}) {
  const legacyRule: ConditionRule[] =
    step.config.field && !step.config.rules
      ? [{ field: step.config.field as string, operator: (step.config.operator as string) ?? "contains", value: step.config.value }]
      : [];
  const rules: ConditionRule[] = (step.config.rules as ConditionRule[] | undefined) ?? legacyRule;
  const logic = (step.config.logic as "and" | "or" | undefined) ?? "and";

  const fieldOptions = [
    ...questionSteps.map((q) => ({
      value: `answer_${q.id}`,
      label: `"${String(q.config.message ?? "Question")}" (step ${steps.indexOf(q) + 1})`,
    })),
    ...BUILT_IN_FIELDS,
  ];

  function updateRules(next: ConditionRule[]) {
    onChange({ config: { ...step.config, rules: next, logic, field: undefined, operator: undefined, value: undefined } });
  }

  return (
    <Space orientation="vertical" style={{ width: "100%" }}>
      {rules.map((rule, ri) => (
        <Space key={ri} wrap>
          <AutoComplete
            placeholder="Field (answer, visitor_tier, a variable...)"
            style={{ width: 220 }}
            value={rule.field}
            options={fieldOptions}
            onChange={(v) => updateRules(rules.map((r, i) => (i === ri ? { ...r, field: v } : r)))}
          />
          <Select
            placeholder="Condition"
            style={{ width: 140 }}
            value={rule.operator}
            onChange={(v) => updateRules(rules.map((r, i) => (i === ri ? { ...r, operator: v } : r)))}
            options={[
              { value: "contains", label: "contains" },
              { value: "equals", label: "is exactly" },
              { value: "not_equals", label: "is not" },
            ]}
          />
          <Input
            placeholder="value"
            style={{ width: 160 }}
            value={String(rule.value ?? "")}
            onChange={(e) => updateRules(rules.map((r, i) => (i === ri ? { ...r, value: e.target.value } : r)))}
          />
          <Button type="text" danger icon={<DeleteOutlined />} onClick={() => updateRules(rules.filter((_, i) => i !== ri))} />
        </Space>
      ))}
      <Space>
        <Button size="small" onClick={() => updateRules([...rules, { field: "", operator: "contains", value: "" }])}>
          + Add condition
        </Button>
        {rules.length > 1 && (
          <Segmented
            size="small"
            value={logic}
            onChange={(v) => onChange({ config: { ...step.config, logic: v as "and" | "or" } })}
            options={[
              { value: "and", label: "Match all (AND)" },
              { value: "or", label: "Match any (OR)" },
            ]}
          />
        )}
      </Space>
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
        Connect this step&apos;s green (Yes) and red (No) dots to the next steps on the canvas.
      </Typography.Paragraph>
    </Space>
  );
}

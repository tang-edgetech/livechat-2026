"use client";

import { useEffect, useState } from "react";
import { AutoComplete, Button, Card, Checkbox, Input, InputNumber, Modal, Segmented, Select, Space, Switch, Typography, message } from "antd";
import { DeleteOutlined, PlusOutlined } from "@ant-design/icons";
import { useRouter } from "next/navigation";

import { apiGet, apiPost, apiPatch, ApiError } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";
import type { BotFlow, ConditionRule, ConditionSet, TriggerDef } from "@/lib/automationTypes";
import type { Merchant } from "@/lib/types";
import { FlowChartPreview } from "./FlowChartPreview";
import { STEP_TYPES, newStepId, stepsToFlow, flowToSteps, type BuilderStep } from "./stepTypes";

const TEMPLATE: BuilderStep[] = [
  { id: "s1", type: "ask_question", config: { message: "What's your name?" } },
  { id: "s2", type: "ask_question", config: { message: "And your email address?" } },
  { id: "s3", type: "send_message", config: { message: "Thanks! Connecting you to a member of our team now." } },
  { id: "s4", type: "handoff_to_agent", config: {} },
];

// Always available to reference in a Condition step alongside "answer_N"
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

  const [steps, setSteps] = useState<BuilderStep[]>(() => {
    if (existing) {
      try {
        return flowToSteps(JSON.parse(existing.flow));
      } catch {
        return [];
      }
    }
    return [];
  });
  const [pickerOpen, setPickerOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);

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
      } catch {
        // ignore
      }
    }
  }, [existing]);

  function addStep(type: BuilderStep["type"]) {
    setSteps((prev) => [...prev, { id: newStepId(), type, config: {} }]);
    setPickerOpen(false);
  }

  function updateStep(id: string, patch: Partial<BuilderStep>) {
    setSteps((prev) => prev.map((s) => (s.id === id ? { ...s, ...patch } : s)));
  }

  function removeStep(id: string) {
    setSteps((prev) => prev.filter((s) => s.id !== id));
  }

  function applyTemplate() {
    setSteps(TEMPLATE.map((s) => ({ ...s, id: newStepId() })));
  }

  const questionSteps = steps.filter((s) => s.type === "ask_question");
  const hasTerminal = steps.some((s) => s.type === "handoff_to_agent" || s.type === "close_chat");

  async function save() {
    if (!name.trim()) {
      message.error("Please name this bot flow.");
      return;
    }
    if (steps.length === 0) {
      message.error("Add at least one step.");
      return;
    }
    if (!hasTerminal) {
      message.error('End the flow with "Hand over to a real person" or "End the chat".');
      return;
    }
    if (!isGlobal && !merchantUuid) {
      message.error("Choose a merchant, or mark this as global.");
      return;
    }

    const conditions: ConditionSet = {
      logic: "and",
      rules: usePageCondition && pageContains ? [{ field: "page_url", operator: "contains", value: pageContains }] : [],
    };
    const trigger: TriggerDef = { type: "chat_start", conditions };
    const flow = stepsToFlow(steps);

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

        <Card
          title="Steps"
          extra={
            steps.length === 0 && (
              <Button size="small" onClick={applyTemplate}>
                Use a template
              </Button>
            )
          }
        >
          <Space orientation="vertical" style={{ width: "100%" }}>
            {steps.map((step, i) => (
              <StepCard
                key={step.id}
                index={i}
                step={step}
                steps={steps}
                questionSteps={questionSteps}
                integrations={integrations}
                onChange={(patch) => updateStep(step.id, patch)}
                onRemove={() => removeStep(step.id)}
              />
            ))}
            <Button icon={<PlusOutlined />} onClick={() => setPickerOpen(true)} block>
              Add step
            </Button>
          </Space>
        </Card>

        <Button type="primary" loading={submitting} onClick={save} style={{ width: 160 }}>
          Save bot flow
        </Button>
      </div>

      <div className="w-full lg:w-80">
        <Card title="Flow preview">
          <FlowChartPreview steps={steps} />
        </Card>
      </div>

      <Modal open={pickerOpen} onCancel={() => setPickerOpen(false)} footer={null} title="Add a step">
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          {STEP_TYPES.map((t) => (
            <button
              key={t.type}
              onClick={() => addStep(t.type)}
              className="rounded-lg border border-black/10 p-3 text-left hover:border-blue-500 dark:border-white/10"
            >
              <div className="font-medium">{t.label}</div>
              <div className="text-xs text-neutral-500">{t.description}</div>
            </button>
          ))}
        </div>
      </Modal>
    </div>
  );
}

function StepCard({
  index,
  step,
  steps,
  questionSteps,
  integrations,
  onChange,
  onRemove,
}: {
  index: number;
  step: BuilderStep;
  steps: BuilderStep[];
  questionSteps: BuilderStep[];
  integrations: { id: number; name: string }[];
  onChange: (patch: Partial<BuilderStep>) => void;
  onRemove: () => void;
}) {
  const label = STEP_TYPES.find((t) => t.type === step.type)?.label ?? step.type;

  return (
    <Card size="small" title={`Step ${index + 1}: ${label}`} extra={<Button type="text" danger icon={<DeleteOutlined />} onClick={onRemove} />}>
      {step.type === "send_message" && (
        <Input.TextArea
          placeholder="Message to send"
          value={String(step.config.message ?? "")}
          onChange={(e) => onChange({ config: { ...step.config, message: e.target.value } })}
        />
      )}

      {step.type === "ask_question" && (
        <AskQuestionFields step={step} index={index} steps={steps} onChange={onChange} />
      )}

      {step.type === "condition" && (
        <ConditionFields step={step} index={index} steps={steps} questionSteps={questionSteps} onChange={onChange} />
      )}

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
    </Card>
  );
}

function stepTargetOptions(steps: BuilderStep[], excludeIndex: number) {
  return steps
    .map((s, i) => ({ value: s.id, label: `Step ${i + 1}: ${STEP_TYPES.find((t) => t.type === s.type)?.label}` }))
    .filter((_, i) => i !== excludeIndex);
}

// Never a bare regex field by default (overview.md §6.0) — a plain-
// language preset picker that resolves to a regex under the hood, with
// "Custom pattern..." as the escape hatch. Retry limit + fail target only
// appear once a format is actually required.
function AskQuestionFields({
  step,
  index,
  steps,
  onChange,
}: {
  step: BuilderStep;
  index: number;
  steps: BuilderStep[];
  onChange: (patch: Partial<BuilderStep>) => void;
}) {
  const pattern = (step.config.validationPattern as string | undefined) ?? "";
  const knownPreset = VALIDATION_PRESETS.find((p) => p.value === pattern && p.value !== "custom");
  const presetValue = pattern === "" ? "" : knownPreset ? knownPreset.value : "custom";

  return (
    <Space orientation="vertical" style={{ width: "100%" }}>
      <Input.TextArea
        placeholder="Question to ask"
        value={String(step.config.message ?? "")}
        onChange={(e) => onChange({ config: { ...step.config, message: e.target.value, variable: `answer_${index}` } })}
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
            options={stepTargetOptions(steps, index)}
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
// directly (e.g. a call_integration saveResponseAs/"_ok" flag).
function ConditionFields({
  step,
  index,
  steps,
  questionSteps,
  onChange,
}: {
  step: BuilderStep;
  index: number;
  steps: BuilderStep[];
  questionSteps: BuilderStep[];
  onChange: (patch: Partial<BuilderStep>) => void;
}) {
  const legacyRule: ConditionRule[] =
    step.config.field && !step.config.rules
      ? [{ field: step.config.field as string, operator: (step.config.operator as string) ?? "contains", value: step.config.value }]
      : [];
  const rules: ConditionRule[] = (step.config.rules as ConditionRule[] | undefined) ?? legacyRule;
  const logic = (step.config.logic as "and" | "or" | undefined) ?? "and";

  const fieldOptions = [
    ...questionSteps
      .filter((q) => steps.indexOf(q) < index)
      .map((q, qi) => ({ value: `answer_${steps.indexOf(q)}`, label: `"${String(q.config.message ?? `Question ${qi + 1}`)}"` })),
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
      <Select
        placeholder="If not matched, skip to step..."
        style={{ width: "100%" }}
        value={step.falseTarget}
        onChange={(v) => onChange({ falseTarget: v })}
        options={stepTargetOptions(steps, index)}
      />
    </Space>
  );
}

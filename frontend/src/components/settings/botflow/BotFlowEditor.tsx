"use client";

import { useEffect, useState } from "react";
import { Button, Card, Checkbox, Input, Modal, Select, Space, Switch, Typography, message } from "antd";
import { DeleteOutlined, PlusOutlined } from "@ant-design/icons";
import { useRouter } from "next/navigation";

import { apiGet, apiPost, apiPatch, ApiError } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";
import type { BotFlow, ConditionSet, TriggerDef } from "@/lib/automationTypes";
import type { Merchant } from "@/lib/types";
import { FlowChartPreview } from "./FlowChartPreview";
import { STEP_TYPES, newStepId, stepsToFlow, flowToSteps, type BuilderStep } from "./stepTypes";

const TEMPLATE: BuilderStep[] = [
  { id: "s1", type: "ask_question", config: { message: "What's your name?" } },
  { id: "s2", type: "ask_question", config: { message: "And your email address?" } },
  { id: "s3", type: "send_message", config: { message: "Thanks! Connecting you to a member of our team now." } },
  { id: "s4", type: "handoff_to_agent", config: {} },
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
        <Space orientation="vertical" style={{ width: "100%" }}>
          <Input.TextArea
            placeholder="Question to ask"
            value={String(step.config.message ?? "")}
            onChange={(e) => onChange({ config: { ...step.config, message: e.target.value, variable: `answer_${index}` } })}
          />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            The visitor&apos;s reply will be remembered as &quot;the answer to this question&quot;.
          </Typography.Text>
        </Space>
      )}

      {step.type === "condition" && (
        <Space orientation="vertical" style={{ width: "100%" }}>
          <Select
            placeholder="Check the answer to..."
            style={{ width: "100%" }}
            value={step.config.field as string | undefined}
            onChange={(v) => onChange({ config: { ...step.config, field: v } })}
            options={questionSteps
              .filter((q) => steps.indexOf(q) < index)
              .map((q, qi) => ({ value: `answer_${steps.indexOf(q)}`, label: `"${String(q.config.message ?? `Question ${qi + 1}`)}"` }))}
          />
          <Space>
            <Select
              placeholder="Condition"
              style={{ width: 140 }}
              value={step.config.operator as string | undefined}
              onChange={(v) => onChange({ config: { ...step.config, operator: v } })}
              options={[
                { value: "contains", label: "contains" },
                { value: "equals", label: "is exactly" },
                { value: "not_equals", label: "is not" },
              ]}
            />
            <Input
              placeholder="value"
              style={{ width: 160 }}
              value={String(step.config.value ?? "")}
              onChange={(e) => onChange({ config: { ...step.config, value: e.target.value } })}
            />
          </Space>
          <Select
            placeholder="If not matched, skip to step..."
            style={{ width: "100%" }}
            value={step.falseTarget}
            onChange={(v) => onChange({ falseTarget: v })}
            options={steps
              .map((s, i) => ({ value: s.id, label: `Step ${i + 1}: ${STEP_TYPES.find((t) => t.type === s.type)?.label}` }))
              .filter((_, i) => i !== index)}
          />
        </Space>
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

"use client";

import { useEffect, useState } from "react";
import { Button, Card, Checkbox, Input, Segmented, Select, Space, Table, Tag, TimePicker, Typography, message } from "antd";
import { DeleteOutlined, EditOutlined } from "@ant-design/icons";
import type { Dayjs } from "dayjs";
import dayjs from "dayjs";

import { apiDelete, apiGet, apiPatch, apiPost, ApiError } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";
import { confirmAction } from "@/components/modals/confirm";
import { sanitizeHtml } from "@/lib/sanitizeHtml";
import type { AutomationRule, ConditionSet } from "@/lib/automationTypes";
import type { Merchant } from "@/lib/types";

// The plain-language sentence builder from overview.md §6.0/§6.3 — never
// a raw field/operator/value form. Two optional conditions (page, time),
// combined with "and", read/write the `condition` JSON underneath.
export function AutomationTab() {
  const { user } = useAuth();
  const isSuperAdmin = user?.role === "super_admin";

  const [rules, setRules] = useState<AutomationRule[]>([]);
  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);

  const [name, setName] = useState("");
  const [triggerType, setTriggerType] = useState<"chat_start" | "keyword_message">("chat_start");
  const [usePageCondition, setUsePageCondition] = useState(false);
  const [pageContains, setPageContains] = useState("");
  const [useTimeCondition, setUseTimeCondition] = useState(false);
  const [timeRange, setTimeRange] = useState<[Dayjs, Dayjs] | null>(null);
  const [keywords, setKeywords] = useState("");
  const [messageText, setMessageText] = useState("");
  const [isHtml, setIsHtml] = useState(false);
  const [isGlobal, setIsGlobal] = useState(false);
  const [merchantUuid, setMerchantUuid] = useState<string | undefined>(undefined);
  const [submitting, setSubmitting] = useState(false);

  function load() {
    setLoading(true);
    apiGet<{ rules: AutomationRule[] }>("/api/automation-rules")
      .then((res) => setRules(res.rules))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial load on mount.
    load();
    apiGet<{ merchants: Merchant[] }>("/api/merchants").then((res) => setMerchants(res.merchants));
  }, []);

  function describeCondition(raw: string | null): string {
    if (!raw) return "Always (no conditions)";
    try {
      const cs: ConditionSet = JSON.parse(raw);
      const parts = cs.rules.map((r) => {
        if (r.field === "page_url") return `page contains "${r.value}"`;
        if (r.field === "time_of_day") return `time between ${(r.value as string[])?.join(" and ")}`;
        if (r.field === "message") return `message contains "${r.value}"`;
        return `${r.field} ${r.operator} ${r.value}`;
      });
      return parts.length ? "If " + parts.join(cs.logic === "or" ? " or " : " and ") : "Always";
    } catch {
      return "Always";
    }
  }

  function startCreate() {
    setEditingId(null);
    setName("");
    setTriggerType("chat_start");
    setMessageText("");
    setIsHtml(false);
    setUsePageCondition(false);
    setUseTimeCondition(false);
    setPageContains("");
    setTimeRange(null);
    setKeywords("");
    setIsGlobal(false);
    setMerchantUuid(undefined);
    setCreating(true);
  }

  function startEdit(rule: AutomationRule) {
    setEditingId(rule.id);
    setName(rule.name);
    setTriggerType(rule.trigger_type ?? "chat_start");
    setMessageText(rule.message);
    setIsHtml(rule.is_html);
    setIsGlobal(rule.is_global);
    setMerchantUuid(rule.merchant_uuid ?? undefined);
    setUsePageCondition(false);
    setUseTimeCondition(false);
    setPageContains("");
    setTimeRange(null);
    setKeywords("");
    if (rule.condition) {
      try {
        const cs: ConditionSet = JSON.parse(rule.condition);
        const messageKeywords: string[] = [];
        for (const r of cs.rules) {
          if (r.field === "page_url") {
            setUsePageCondition(true);
            setPageContains(String(r.value));
          }
          if (r.field === "time_of_day") {
            const [start, end] = r.value as string[];
            setUseTimeCondition(true);
            setTimeRange([dayjs(start, "HH:mm"), dayjs(end, "HH:mm")]);
          }
          if (r.field === "message") messageKeywords.push(String(r.value));
        }
        if (messageKeywords.length) setKeywords(messageKeywords.join(", "));
      } catch {
        // ignore malformed stored condition
      }
    }
    setCreating(true);
  }

  async function submit() {
    if (!messageText.trim() || !name.trim()) {
      message.error("Please fill in the name and message.");
      return;
    }
    if (!isGlobal && !merchantUuid) {
      message.error("Choose a merchant, or mark this as global.");
      return;
    }

    let condition: ConditionSet;
    if (triggerType === "keyword_message") {
      const list = keywords.split(",").map((k) => k.trim()).filter(Boolean);
      if (!list.length) {
        message.error("Enter at least one keyword or phrase to match.");
        return;
      }
      condition = { logic: "or", rules: list.map((k) => ({ field: "message", operator: "contains", value: k })) };
    } else {
      const rules: { field: string; operator: string; value: unknown }[] = [];
      if (usePageCondition && pageContains) {
        rules.push({ field: "page_url", operator: "contains", value: pageContains });
      }
      if (useTimeCondition && timeRange) {
        rules.push({ field: "time_of_day", operator: "between", value: [timeRange[0].format("HH:mm"), timeRange[1].format("HH:mm")] });
      }
      condition = { logic: "and", rules };
    }

    setSubmitting(true);
    try {
      const payload = {
        name,
        triggerType,
        condition: condition.rules.length ? JSON.stringify(condition) : "",
        message: messageText,
        isHtml,
        isGlobal,
        isActive: true,
        merchantUuid: isGlobal ? undefined : merchantUuid,
      };
      if (editingId) {
        await apiPatch(`/api/automation-rules/${editingId}`, payload);
        message.success("Rule updated");
      } else {
        await apiPost("/api/automation-rules", payload);
        message.success("Rule created");
      }
      setCreating(false);
      setEditingId(null);
      setName("");
      setMessageText("");
      setIsHtml(false);
      setUsePageCondition(false);
      setUseTimeCondition(false);
      setPageContains("");
      setTimeRange(null);
      setKeywords("");
      load();
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "Could not save rule");
    } finally {
      setSubmitting(false);
    }
  }

  function remove(id: number) {
    confirmAction({
      title: "Delete this rule?",
      okText: "Delete",
      danger: true,
      onConfirm: async () => {
        await apiDelete(`/api/automation-rules/${id}`);
        message.success("Deleted");
        load();
      },
    });
  }

  return (
    <div className="flex flex-col gap-4">
      {!creating ? (
        <div>
          <Button type="primary" onClick={startCreate}>
            Create Rule
          </Button>
        </div>
      ) : (
        <Card
          title={
            editingId
              ? triggerType === "keyword_message" ? "Edit auto-response rule" : "Edit greeting rule"
              : triggerType === "keyword_message" ? "New auto-response rule" : "New greeting rule"
          }
        >
          <Space orientation="vertical" style={{ width: "100%", maxWidth: 480 }}>
            <Segmented
              value={triggerType}
              onChange={(v) => setTriggerType(v as "chat_start" | "keyword_message")}
              options={[
                { label: "When a chat starts", value: "chat_start" },
                { label: "When a message contains a keyword", value: "keyword_message" },
              ]}
            />
            <Input placeholder="Rule name (for your reference)" value={name} onChange={(e) => setName(e.target.value)} />

            {triggerType === "chat_start" ? (
              <>
                <Checkbox checked={usePageCondition} onChange={(e) => setUsePageCondition(e.target.checked)}>
                  Only show when the page contains
                </Checkbox>
                {usePageCondition && (
                  <Input placeholder="e.g. /pricing" value={pageContains} onChange={(e) => setPageContains(e.target.value)} />
                )}

                <Checkbox checked={useTimeCondition} onChange={(e) => setUseTimeCondition(e.target.checked)}>
                  Only show between certain hours
                </Checkbox>
                {useTimeCondition && (
                  <TimePicker.RangePicker
                    format="HH:mm"
                    value={timeRange}
                    onChange={(v) => setTimeRange(v as [Dayjs, Dayjs] | null)}
                  />
                )}
              </>
            ) : (
              <>
                <Typography.Text strong>Keywords or phrases (comma-separated)</Typography.Text>
                <Input
                  placeholder="e.g. refund, pricing, cancel order"
                  value={keywords}
                  onChange={(e) => setKeywords(e.target.value)}
                />
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  If the visitor&apos;s message contains any of these (case-insensitive), the reply below is sent
                  automatically. Only applies while no bot flow is actively driving the chat.
                </Typography.Text>
              </>
            )}

            <Typography.Text strong>{triggerType === "keyword_message" ? "Auto-reply message" : "Message to show"}</Typography.Text>
            <Input.TextArea
              rows={3}
              placeholder={
                triggerType === "keyword_message"
                  ? "e.g. Refunds take 3-5 business days once approved."
                  : "e.g. Welcome! Let us know how we can help."
              }
              value={messageText}
              onChange={(e) => setMessageText(e.target.value)}
            />
            <Checkbox checked={isHtml} onChange={(e) => setIsHtml(e.target.checked)}>
              Insert As HTML (allows formatting and inline styles, e.g. <code>&lt;b&gt;</code>,{" "}
              <code>style=&quot;color:red&quot;</code> — sanitized before display)
            </Checkbox>
            {isHtml && messageText.trim() && (
              <div>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  Preview:
                </Typography.Text>
                <div className="rounded border px-2 py-1" dangerouslySetInnerHTML={{ __html: sanitizeHtml(messageText) }} />
              </div>
            )}

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

            <Space>
              <Button onClick={() => setCreating(false)}>Cancel</Button>
              <Button type="primary" loading={submitting} onClick={submit}>
                Save
              </Button>
            </Space>
          </Space>
        </Card>
      )}

      <Table
        rowKey="id"
        loading={loading}
        dataSource={rules}
        columns={[
          { title: "Name", dataIndex: "name" },
          {
            title: "Trigger",
            key: "trigger",
            render: (_, r) =>
              r.trigger_type === "keyword_message" ? (
                <Tag color="orange">Keyword match</Tag>
              ) : (
                <Tag color="green">New chat</Tag>
              ),
          },
          { title: "Condition", key: "condition", render: (_, r) => describeCondition(r.condition) },
          { title: "Message", dataIndex: "message", ellipsis: true },
          {
            title: "Scope",
            key: "scope",
            render: (_, r) => (
              <Space>
                {r.is_global ? <Tag color="blue">Global</Tag> : <Tag>Merchant</Tag>}
                {r.is_html && <Tag color="purple">HTML</Tag>}
              </Space>
            ),
          },
          {
            title: "Actions",
            key: "actions",
            render: (_, r) => (
              <Space>
                <Button type="text" icon={<EditOutlined />} onClick={() => startEdit(r)} />
                <Button type="text" danger icon={<DeleteOutlined />} onClick={() => remove(r.id)} />
              </Space>
            ),
          },
        ]}
      />
    </div>
  );
}

"use client";

import { useState } from "react";
import { Button, Input, Segmented, Space, Typography } from "antd";

import type { WidgetConfig } from "@/lib/types";

// Shared by the merchant edit page and the top-level Embed settings
// section (overview.md §6.5/§10.1) — one merchant, two render modes:
// a floating-bubble <script> widget, or a plain <iframe> for embedding
// the chat inline on a page. Both read/write the same widget_config.
export function EmbedSection({
  code,
  config,
  onConfigChange,
}: {
  code: string;
  config: WidgetConfig;
  onConfigChange: (config: WidgetConfig) => void;
}) {
  const [embedType, setEmbedType] = useState<"widget" | "page">("widget");
  const [newOrigin, setNewOrigin] = useState("");
  const [bulkAdding, setBulkAdding] = useState(false);
  const [bulkOrigins, setBulkOrigins] = useState("");

  const origin = typeof window !== "undefined" ? window.location.origin : "";
  const widgetSnippet = `<script src="${origin}/embed.js" data-merchant-code="${code}" async></script>`;
  const pageSnippet = `<iframe src="${origin}/widget/${code}" style="width:100%;height:600px;border:none;" title="Chat"></iframe>`;
  const embedSnippet = embedType === "widget" ? widgetSnippet : pageSnippet;

  function addOrigin() {
    const value = newOrigin.trim();
    if (!value) return;
    const existing = config.allowedOrigins ?? [];
    if (!existing.includes(value)) {
      onConfigChange({ ...config, allowedOrigins: [...existing, value] });
    }
    setNewOrigin("");
  }

  function removeOrigin(value: string) {
    onConfigChange({ ...config, allowedOrigins: (config.allowedOrigins ?? []).filter((o) => o !== value) });
  }

  function addOriginsBulk() {
    const parsed = bulkOrigins
      .split(/[\n,]/)
      .map((o) => o.trim())
      .filter(Boolean);
    if (parsed.length === 0) return;
    const existing = config.allowedOrigins ?? [];
    const merged = [...existing, ...parsed.filter((o) => !existing.includes(o))];
    onConfigChange({ ...config, allowedOrigins: merged });
    setBulkOrigins("");
    setBulkAdding(false);
  }

  return (
    <Space orientation="vertical" style={{ width: "100%" }}>
      <Space wrap style={{ justifyContent: "space-between", width: "100%" }}>
        <Segmented
          value={embedType}
          onChange={(v) => setEmbedType(v as "widget" | "page")}
          options={[
            { value: "widget", label: "Widget (Floating Bubble)" },
            { value: "page", label: "Page (Inline Chat Window)" },
          ]}
        />
        <Button href={`/widget/${code}`} target="_blank">
          Preview Live Chat
        </Button>
      </Space>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
        {embedType === "widget"
          ? "A floating chat bubble that opens/closes over your page. Paste just before the closing "
          : "A plain chat window embedded directly on a page (e.g. a Contact Us page). Paste anywhere in your page's "}
        <code>&lt;{embedType === "widget" ? "/body" : "iframe"}&gt;</code>
        {embedType === "widget" ? " tag." : " placement."}
      </Typography.Paragraph>
      <Input.TextArea value={embedSnippet} readOnly rows={2} />

      {embedType === "page" && (
        <>
          <Typography.Text strong>Re-Theming This Embed</Typography.Text>
          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            There&apos;s no helper script for a page embed, so post the theme message straight to your own iframe
            element&apos;s <code>contentWindow</code> (your domain must be listed under Allowed Origins below):
          </Typography.Paragraph>
          <Input.TextArea
            readOnly
            rows={3}
            value={`document.querySelector("iframe[title='Chat']").contentWindow.postMessage(\n  { type: "livechat:theme", payload: { accentColor: "#ff0000" } },\n  "${origin}"\n);`}
          />
        </>
      )}

      <Typography.Text strong>Allowed Origins</Typography.Text>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
        Domains where this merchant&apos;s embed actually lives. Required for a &quot;page&quot; embed&apos;s parent
        site to re-theme it at runtime via <code>postMessage</code> — a domain not listed here is ignored for
        safety.
      </Typography.Paragraph>
      <Space wrap>
        {(config.allowedOrigins ?? []).map((o) => (
          <Typography.Text key={o} code>
            {o}{" "}
            <a onClick={() => removeOrigin(o)} style={{ color: "inherit" }}>
              ✕
            </a>
          </Typography.Text>
        ))}
      </Space>
      <Space wrap>
        <Space.Compact style={{ maxWidth: 360 }}>
          <Input placeholder="https://example.com" value={newOrigin} onChange={(e) => setNewOrigin(e.target.value)} onPressEnter={addOrigin} />
          <Button onClick={addOrigin}>Add</Button>
        </Space.Compact>
        <Button onClick={() => setBulkAdding((v) => !v)}>{bulkAdding ? "Cancel bulk add" : "Add Multiple"}</Button>
      </Space>
      {bulkAdding && (
        <Space orientation="vertical" style={{ width: "100%", maxWidth: 480 }}>
          <Input.TextArea
            rows={4}
            placeholder={"One per line, or comma-separated —\nhttps://example.com\nhttps://shop.example.com"}
            value={bulkOrigins}
            onChange={(e) => setBulkOrigins(e.target.value)}
          />
          <Button type="primary" onClick={addOriginsBulk}>
            Add All
          </Button>
        </Space>
      )}
    </Space>
  );
}

"use client";

import { useEffect, useRef, useState } from "react";
import { Button, Card, Input, Space, Tag, Typography, Upload, message as antMessage } from "antd";
import { PaperClipOutlined, SendOutlined } from "@ant-design/icons";

import { apiFetch, apiGet, apiPost } from "@/lib/api";
import { useSocket } from "@/lib/socket";
import { STATUS_COLOR, type ChatMessage } from "@/lib/chatTypes";

// Internal test harness (overview.md §11 Phase 2: "fed by an internal
// test harness for now — the real pre-chat form comes in Phase 3").
// This page exercises the exact same identity-resolution + chat-creation
// path a real visitor will hit later; Phase 3 just wraps it in the
// iframe embed and swaps this bare form for the real pre-chat form.
export default function WidgetTestPage() {
  const [session, setSession] = useState<{ chatUuid: string; visitorUuid: string } | null>(null);

  return (
    <div className="flex min-h-screen items-center justify-center bg-neutral-50 p-6 dark:bg-neutral-950">
      {session ? <TestChatWindow {...session} /> : <StartForm onStart={setSession} />}
    </div>
  );
}

function StartForm({ onStart }: { onStart: (s: { chatUuid: string; visitorUuid: string }) => void }) {
  const [merchantCode, setMerchantCode] = useState("");
  const [phone, setPhone] = useState("");
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function submit() {
    setSubmitting(true);
    try {
      const res = await apiPost<{ chatUuid: string; visitorUuid: string }>("/api/visitor/start", {
        merchantCode,
        phone,
        email,
        displayName,
      });
      onStart(res);
    } catch {
      antMessage.error("Could not start chat — check the merchant code");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Card title="Start a test chat" style={{ width: 360 }}>
      <Space orientation="vertical" style={{ width: "100%" }}>
        <Input placeholder="Merchant code" value={merchantCode} onChange={(e) => setMerchantCode(e.target.value)} />
        <Input placeholder="Phone" value={phone} onChange={(e) => setPhone(e.target.value)} />
        <Input placeholder="Email (optional)" value={email} onChange={(e) => setEmail(e.target.value)} />
        <Input placeholder="Name (optional)" value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
        <Button type="primary" block loading={submitting} onClick={submit}>
          Start chat
        </Button>
      </Space>
    </Card>
  );
}

function TestChatWindow({ chatUuid, visitorUuid }: { chatUuid: string; visitorUuid: string }) {
  const [status, setStatus] = useState<string>("pending");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [draft, setDraft] = useState("");
  const bottomRef = useRef<HTMLDivElement>(null);

  function load() {
    apiGet<{ status: string; messages: ChatMessage[] }>(`/api/visitor/chats/${chatUuid}?visitor=${visitorUuid}`).then((res) => {
      setStatus(res.status);
      setMessages(res.messages);
    });
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  useSocket(`/ws/visitor?visitor=${visitorUuid}&chat=${chatUuid}`, (event) => {
    if (event.type === "message") {
      setMessages((prev) => [...prev, event.data as ChatMessage]);
    }
    if (event.type === "chat_closed") {
      setStatus("closed");
    }
  });

  async function send() {
    if (!draft.trim()) return;
    const sent = await apiPost<ChatMessage>(`/api/visitor/chats/${chatUuid}/messages?visitor=${visitorUuid}`, { body: draft });
    setMessages((prev) => [...prev, sent]);
    setDraft("");
  }

  async function uploadFile(file: File) {
    const form = new FormData();
    form.append("file", file);
    const sent = await apiFetch<ChatMessage>(`/api/visitor/chats/${chatUuid}/files?visitor=${visitorUuid}`, {
      method: "POST",
      body: form,
    });
    setMessages((prev) => [...prev, sent]);
    return false;
  }

  const isClosed = status === "closed";

  return (
    <Card
      title="LiveChat"
      style={{ width: 380, height: 560, display: "flex", flexDirection: "column" }}
      styles={{ body: { flex: 1, display: "flex", flexDirection: "column", overflow: "hidden" } }}
      extra={<Tag color={STATUS_COLOR[status as keyof typeof STATUS_COLOR]}>{status}</Tag>}
    >
      <div className="flex-1 overflow-y-auto flex flex-col gap-2 pr-1">
        {messages.map((m) => (
          <div key={m.id} className={`flex ${m.sender_type === "visitor" ? "justify-end" : "justify-start"}`}>
            <div
              className={`max-w-[75%] rounded-lg px-3 py-2 ${
                m.sender_type === "visitor" ? "bg-blue-500 text-white" : "bg-neutral-100 dark:bg-neutral-800"
              }`}
            >
              {m.body}
            </div>
          </div>
        ))}
        <div ref={bottomRef} />
      </div>
      <Space.Compact style={{ width: "100%", marginTop: 12 }}>
        <Upload beforeUpload={uploadFile} showUploadList={false} disabled={isClosed}>
          <Button icon={<PaperClipOutlined />} disabled={isClosed} />
        </Upload>
        <Input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onPressEnter={send}
          disabled={isClosed}
          placeholder={isClosed ? "Chat closed" : "Type a message"}
        />
        <Button type="primary" icon={<SendOutlined />} onClick={send} disabled={isClosed} />
      </Space.Compact>
      <Typography.Text type="secondary" style={{ fontSize: 11, marginTop: 4 }}>
        Internal test harness — Phase 3 replaces this with the real embed widget.
      </Typography.Text>
    </Card>
  );
}

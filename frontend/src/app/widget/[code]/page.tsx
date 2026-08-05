"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Button, Input, Space, Spin, Typography, Upload, message as antMessage } from "antd";
import { PaperClipOutlined, SendOutlined } from "@ant-design/icons";
import { useParams, useSearchParams } from "next/navigation";

import { apiFetch, apiGet, apiPost, ApiError } from "@/lib/api";
import { useSocket } from "@/lib/socket";
import { appendMessage, messageIsHtml, type ChatMessage } from "@/lib/chatTypes";
import { sanitizeHtml } from "@/lib/sanitizeHtml";
import { THEME_PRIMARY } from "@/lib/theme";
import type { WidgetConfig } from "@/lib/types";

type Session = { chatUuid: string; visitorUuid: string };

function sessionKey(code: string) {
  return `livechat_session_${code}`;
}

export default function WidgetPage() {
  const { code } = useParams<{ code: string }>();
  const searchParams = useSearchParams();
  const passthroughToken = searchParams.get("token");
  const parentUrl = searchParams.get("parentUrl") ?? "";

  const [merchantName, setMerchantName] = useState<string | null>(null);
  const [config, setConfig] = useState<WidgetConfig>({});
  const [notFound, setNotFound] = useState(false);
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    apiGet<{ name: string; widgetConfig: string }>(`/api/visitor/merchant/${code}`)
      .then((res) => {
        setMerchantName(res.name);
        try {
          setConfig(res.widgetConfig ? JSON.parse(res.widgetConfig) : {});
        } catch {
          setConfig({});
        }
      })
      .catch(() => setNotFound(true))
      .finally(() => setLoading(false));

    const stored = localStorage.getItem(sessionKey(code));
    if (stored) {
      try {
        // eslint-disable-next-line react-hooks/set-state-in-effect -- resuming a prior session found in localStorage on first mount.
        setSession(JSON.parse(stored));
      } catch {
        // ignore corrupt storage
      }
    }
  }, [code]);

  function startSession(s: Session) {
    localStorage.setItem(sessionKey(code), JSON.stringify(s));
    setSession(s);
  }

  // Runtime theming (overview.md §6.5) — a host site can re-skin the
  // widget live via postMessage instead of us redeploying anything.
  // Trust nothing by default: a domain not in this merchant's own
  // allowedOrigins list is silently ignored, same "never trust an
  // unsigned/unlisted sender" posture as the passthrough token. Only
  // accentColor is handled here — corner is the floating bubble/iframe's
  // *position on the host page*, which this iframe's own content has no
  // say over; embed.js's LiveChatWidget.setTheme handles that half by
  // repositioning the DOM elements it already controls.
  useEffect(() => {
    function handleThemeMessage(event: MessageEvent) {
      if (!config.allowedOrigins?.includes(event.origin)) return;
      if (!event.data || event.data.type !== "livechat:theme") return;
      const accentColor = event.data.payload?.accentColor as string | undefined;
      if (!accentColor) return;
      setConfig((prev) => ({ ...prev, accentColor }));
    }
    window.addEventListener("message", handleThemeMessage);
    return () => window.removeEventListener("message", handleThemeMessage);
  }, [config.allowedOrigins]);

  const accent = config.accentColor || THEME_PRIMARY;

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Spin />
      </div>
    );
  }
  if (notFound) {
    return (
      <div className="flex h-screen items-center justify-center p-4 text-center text-neutral-500">
        Chat is unavailable right now.
      </div>
    );
  }

  return (
    <div className="flex h-screen flex-col bg-white dark:bg-neutral-900">
      <div className="flex h-14 shrink-0 items-center px-4 text-white shadow-sm" style={{ backgroundColor: accent }}>
        <span className="font-medium">{merchantName}</span>
      </div>
      {session ? (
        <ChatWindow session={session} accent={accent} />
      ) : (
        <PreChatForm
          passthroughToken={passthroughToken}
          merchantCode={code}
          parentUrl={parentUrl}
          onStarted={startSession}
          accent={accent}
        />
      )}
    </div>
  );
}

function PreChatForm({
  merchantCode,
  passthroughToken,
  parentUrl,
  onStarted,
  accent,
}: {
  merchantCode: string;
  passthroughToken: string | null;
  parentUrl: string;
  onStarted: (s: Session) => void;
  accent: string;
}) {
  const [phone, setPhone] = useState("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const attemptedPassthrough = useRef(false);

  async function submit(passthroughOnly?: boolean) {
    setSubmitting(true);
    try {
      const body = passthroughOnly
        ? { merchantCode, passthroughToken, pageUrl: parentUrl }
        : { merchantCode, phone, email, displayName: name, pageUrl: parentUrl };
      const res = await apiPost<Session>("/api/visitor/start", body);
      onStarted(res);
    } catch (err) {
      if (!passthroughOnly) {
        antMessage.error(err instanceof ApiError && err.status === 429 ? "Too many attempts — please wait a moment." : "Could not start chat");
      }
    } finally {
      setSubmitting(false);
    }
  }

  useEffect(() => {
    if (passthroughToken && !attemptedPassthrough.current) {
      attemptedPassthrough.current = true;
      submit(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [passthroughToken]);

  if (passthroughToken) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spin />
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col justify-center gap-3 p-5">
      <Typography.Title level={5} style={{ marginBottom: 0 }}>
        Let&apos;s get started
      </Typography.Title>
      <Typography.Text type="secondary">Tell us a bit about yourself so we can help you.</Typography.Text>
      <Input placeholder="Phone number" value={phone} onChange={(e) => setPhone(e.target.value)} />
      <Input placeholder="Your name" value={name} onChange={(e) => setName(e.target.value)} />
      <Input placeholder="Email (optional)" value={email} onChange={(e) => setEmail(e.target.value)} />
      <Button type="primary" block loading={submitting} disabled={!phone} onClick={() => submit(false)} style={{ backgroundColor: accent, borderColor: accent }}>
        Start chat
      </Button>
    </div>
  );
}

function ChatWindow({ session, accent }: { session: Session; accent: string }) {
  const [status, setStatus] = useState("pending");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [draft, setDraft] = useState("");
  const bottomRef = useRef<HTMLDivElement>(null);

  function load() {
    apiGet<{ status: string; messages: ChatMessage[] }>(
      `/api/visitor/chats/${session.chatUuid}?visitor=${session.visitorUuid}`,
    ).then((res) => {
      setStatus(res.status);
      setMessages(res.messages);
    });
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session.chatUuid]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const wsPath = useMemo(
    () => `/ws/visitor?visitor=${session.visitorUuid}&chat=${session.chatUuid}`,
    [session],
  );
  useSocket(wsPath, (event) => {
    if (event.type === "message") setMessages((prev) => appendMessage(prev, event.data as ChatMessage));
    if (event.type === "chat_closed") setStatus("closed");
  });

  async function send(overrideBody?: string) {
    const body = overrideBody ?? draft;
    if (!body.trim()) return;
    const sent = await apiPost<ChatMessage>(
      `/api/visitor/chats/${session.chatUuid}/messages?visitor=${session.visitorUuid}`,
      { body },
    );
    setMessages((prev) => appendMessage(prev, sent));
    if (overrideBody === undefined) setDraft("");
  }

  async function uploadFile(file: File) {
    const form = new FormData();
    form.append("file", file);
    const sent = await apiFetch<ChatMessage>(
      `/api/visitor/chats/${session.chatUuid}/files?visitor=${session.visitorUuid}`,
      { method: "POST", body: form },
    );
    setMessages((prev) => appendMessage(prev, sent));
    return false;
  }

  const isClosed = status === "closed";

  return (
    <div className="flex flex-1 flex-col overflow-hidden p-3">
      <div className="flex-1 overflow-y-auto flex flex-col gap-2 pr-1">
        {messages.length === 0 && (
          <Typography.Text type="secondary" style={{ textAlign: "center", marginTop: 16 }}>
            We&apos;ll be right with you.
          </Typography.Text>
        )}
        {messages.map((m) => (
          <div key={m.id} className={`flex ${m.sender_type === "visitor" ? "justify-end" : "justify-start"}`}>
            <div
              className="max-w-[75%] rounded-2xl px-3.5 py-2 shadow-sm"
              style={
                m.sender_type === "visitor"
                  ? { backgroundColor: accent, color: "#fff" }
                  : { backgroundColor: "rgba(0,0,0,0.05)" }
              }
            >
              {messageIsHtml(m) ? (
                <div dangerouslySetInnerHTML={{ __html: sanitizeHtml(m.body) }} />
              ) : (
                <div>{m.body}</div>
              )}
              {m.type === "quick_reply" && <QuickReplyOptions message={m} accent={accent} onPick={(v) => send(v)} />}
            </div>
          </div>
        ))}
        {isClosed && (
          <Typography.Text type="secondary" style={{ textAlign: "center", marginTop: 8 }}>
            This chat has ended. Thanks for reaching out!
          </Typography.Text>
        )}
        <div ref={bottomRef} />
      </div>
      <Space.Compact style={{ width: "100%", marginTop: 8 }}>
        <Upload beforeUpload={uploadFile} showUploadList={false} disabled={isClosed}>
          <Button icon={<PaperClipOutlined />} disabled={isClosed} />
        </Upload>
        <Input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onPressEnter={() => send()}
          disabled={isClosed}
          placeholder={isClosed ? "Chat ended" : "Type a message"}
        />
        <Button type="primary" icon={<SendOutlined />} onClick={() => send()} disabled={isClosed} style={{ backgroundColor: isClosed ? undefined : accent, borderColor: isClosed ? undefined : accent }} />
      </Space.Compact>
    </div>
  );
}

// A visitor's click just posts a normal type=text message with the
// chosen option's value as body (overview.md §4/§6.4) — no special-case
// handling needed downstream, the bot flow's ask_question node reads it
// exactly like free-typed input.
function QuickReplyOptions({ message, accent, onPick }: { message: ChatMessage; accent: string; onPick: (value: string) => void }) {
  let options: { label: string; value: string }[] = [];
  try {
    options = message.metadata ? JSON.parse(message.metadata).options ?? [] : [];
  } catch {
    // ignore
  }
  if (options.length === 0) return null;

  return (
    <div className="mt-2 flex flex-wrap gap-1">
      {options.map((opt) => (
        <button
          key={opt.value}
          onClick={() => onPick(opt.value)}
          className="rounded-full border px-2 py-1 text-xs"
          style={{ borderColor: accent, color: accent, background: "#fff" }}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}

"use client";

import { useEffect, useRef, useState } from "react";
import { Button, Card, Input, Space, Tag, Typography, Upload, message as antMessage } from "antd";
import { PaperClipOutlined, SendOutlined } from "@ant-design/icons";
import { useParams } from "next/navigation";

import { apiFetch, apiGet, apiPost } from "@/lib/api";
import { useSocket } from "@/lib/socket";
import { confirmAction } from "@/components/modals/confirm";
import { STATUS_COLOR, type ChatMessage, type ChatSummary } from "@/lib/chatTypes";

export default function ChatConversationPage() {
  const { uuid } = useParams<{ uuid: string }>();

  const [chat, setChat] = useState<ChatSummary | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);

  function load() {
    apiGet<{ chat: ChatSummary; messages: ChatMessage[] }>(`/api/chats/${uuid}`).then((res) => {
      setChat(res.chat);
      setMessages(res.messages);
    });
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [uuid]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  useSocket("/ws", (event) => {
    if (event.type === "message") {
      const m = event.data as ChatMessage;
      if (m.chat_uuid === uuid) {
        setMessages((prev) => [...prev, m]);
      }
    }
    if (event.type === "chat_closed" && (event.data as { chatUuid?: string })?.chatUuid === uuid) {
      load();
    }
  });

  async function send() {
    if (!draft.trim()) return;
    setSending(true);
    try {
      const sent = await apiPost<ChatMessage>(`/api/chats/${uuid}/messages`, { body: draft });
      setMessages((prev) => [...prev, sent]);
      setDraft("");
    } catch {
      antMessage.error("Could not send message");
    } finally {
      setSending(false);
    }
  }

  function claim() {
    confirmAction({
      title: "Claim this chat?",
      onConfirm: async () => {
        await apiPost(`/api/chats/${uuid}/claim`);
        load();
      },
    });
  }

  function close() {
    confirmAction({
      title: "Close this chat?",
      okText: "Close",
      danger: true,
      onConfirm: async () => {
        await apiPost(`/api/chats/${uuid}/close`);
        load();
      },
    });
  }

  async function uploadFile(file: File) {
    const form = new FormData();
    form.append("file", file);
    try {
      const sent = await apiFetch<ChatMessage>(`/api/chats/${uuid}/files`, { method: "POST", body: form });
      setMessages((prev) => [...prev, sent]);
    } catch {
      antMessage.error("Upload failed");
    }
    return false;
  }

  if (!chat) return null;

  // The backend is the real authority on who may reply (an Agent must be
  // the assigned PIC — see SendMessageHandler); this just avoids showing
  // an enabled input for an obviously-not-yet-active chat.
  const canReply = chat.status === "active";
  const isClosed = chat.status === "closed";
  const canClaim = chat.status === "pending";

  return (
    <div className="flex h-full flex-col gap-4">
      <Card size="small">
        <div className="flex items-center justify-between">
          <div>
            <Typography.Text strong>{chat.visitor_name}</Typography.Text>{" "}
            <span className="text-neutral-500">· {chat.merchant_name}</span>{" "}
            <Tag color={STATUS_COLOR[chat.status]}>{chat.status}</Tag>
          </div>
          <Space>
            {canClaim && (
              <Button type="primary" onClick={claim}>
                Claim
              </Button>
            )}
            {!isClosed && (
              <Button danger onClick={close}>
                Close chat
              </Button>
            )}
          </Space>
        </div>
      </Card>

      <Card className="flex-1 overflow-y-auto" styles={{ body: { display: "flex", flexDirection: "column", gap: 8 } }}>
        {messages.map((m) => (
          <MessageBubble key={m.id} message={m} />
        ))}
        <div ref={bottomRef} />
      </Card>

      <Card size="small">
        <Space.Compact style={{ width: "100%" }}>
          <Upload beforeUpload={uploadFile} showUploadList={false} disabled={isClosed}>
            <Button icon={<PaperClipOutlined />} disabled={isClosed} />
          </Upload>
          <Input
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onPressEnter={send}
            placeholder={isClosed ? "This chat is closed" : "Type a message"}
            disabled={isClosed || !canReply}
          />
          <Button type="primary" icon={<SendOutlined />} onClick={send} loading={sending} disabled={isClosed || !canReply}>
            Send
          </Button>
        </Space.Compact>
      </Card>
    </div>
  );
}

function MessageBubble({ message }: { message: ChatMessage }) {
  const isVisitor = message.sender_type === "visitor";
  return (
    <div className={`flex ${isVisitor ? "justify-start" : "justify-end"}`}>
      <div
        className={`max-w-md rounded-lg px-3 py-2 ${
          isVisitor ? "bg-neutral-100 dark:bg-neutral-800" : "bg-blue-500 text-white"
        }`}
      >
        {message.type === "file" ? <FileAttachment message={message} /> : <span>{message.body}</span>}
      </div>
    </div>
  );
}

function FileAttachment({ message }: { message: ChatMessage }) {
  let fileUuid = "";
  try {
    fileUuid = message.metadata ? JSON.parse(message.metadata).fileUuid : "";
  } catch {
    // ignore
  }
  return (
    <a href={`/api/files/${fileUuid}`} target="_blank" rel="noopener noreferrer" className="underline">
      📎 {message.body}
    </a>
  );
}

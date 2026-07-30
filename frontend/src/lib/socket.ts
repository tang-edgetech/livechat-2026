"use client";

import { useEffect, useRef } from "react";

const WS_URL = process.env.NEXT_PUBLIC_WS_URL ?? "ws://localhost:8081";

export type SocketEvent = { type: string; data: unknown };

// One push-only WebSocket connection per overview.md §2 — every action
// still goes through the AJAX wrapper in api.ts; this only receives.
// `path` connects directly to the Go backend's own WS port (not proxied
// through Next.js — WebSocket upgrades don't reliably survive the
// rewrites proxy), carrying the session cookie automatically since
// same-site cookies aren't port-scoped.
export function useSocket(path: string | null, onEvent: (event: SocketEvent) => void) {
  const onEventRef = useRef(onEvent);
  useEffect(() => {
    onEventRef.current = onEvent;
  });

  useEffect(() => {
    if (!path) return;

    const ws = new WebSocket(`${WS_URL}${path}`);
    ws.onmessage = (msg) => {
      try {
        onEventRef.current(JSON.parse(msg.data));
      } catch {
        // ignore malformed frames
      }
    };

    return () => ws.close();
  }, [path]);
}

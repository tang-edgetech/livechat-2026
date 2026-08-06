"use client";

// A short two-tone "bell" synthesized with the Web Audio API — no audio
// asset to ship/host, and it works offline. Used to alert staff to a new
// chat entering the queue or a customer message arriving (Chats list +
// an open chat) without requiring them to be staring at the screen.
let ctx: AudioContext | null = null;

function getContext(): AudioContext | null {
  if (typeof window === "undefined") return null;
  const Ctor = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  if (!Ctor) return null;
  if (!ctx) ctx = new Ctor();
  return ctx;
}

function tone(audioCtx: AudioContext, startAt: number, freq: number, duration: number) {
  const osc = audioCtx.createOscillator();
  const gain = audioCtx.createGain();
  osc.type = "sine";
  osc.frequency.value = freq;
  gain.gain.setValueAtTime(0, startAt);
  gain.gain.linearRampToValueAtTime(0.25, startAt + 0.01);
  gain.gain.exponentialRampToValueAtTime(0.0001, startAt + duration);
  osc.connect(gain);
  gain.connect(audioCtx.destination);
  osc.start(startAt);
  osc.stop(startAt + duration);
}

// Browsers block audio until a user gesture has happened on the page;
// by the time a WS event fires the agent has almost always already
// clicked/typed something (logging in, navigating), so resume() here is
// just covering the rare case the context started suspended.
export function playNotificationSound() {
  const audioCtx = getContext();
  if (!audioCtx) return;
  if (audioCtx.state === "suspended") {
    audioCtx.resume().catch(() => {});
  }
  const now = audioCtx.currentTime;
  tone(audioCtx, now, 880, 0.15);
  tone(audioCtx, now + 0.12, 1320, 0.2);
}

"use client";

import { Modal } from "antd";
import { useRouter } from "next/navigation";

import { useAuth } from "@/context/AuthContext";

// Custom app modal per overview.md §6.0 — never a native browser
// alert/confirm. Single "OK" button; clicking it logs out and redirects
// to /login. No close ("x") or overlay-dismiss — the user must acknowledge.
export function IdleTimeoutModal() {
  const { sessionExpired, acknowledgeSessionExpired } = useAuth();
  const router = useRouter();

  return (
    <Modal
      open={sessionExpired}
      title="Session expired"
      closable={false}
      mask={{ closable: false }}
      keyboard={false}
      onOk={() => {
        acknowledgeSessionExpired();
        router.push("/login");
      }}
      okText="OK"
      cancelButtonProps={{ style: { display: "none" } }}
    >
      You&apos;ve been signed out after 2 hours of inactivity. Please log in again.
    </Modal>
  );
}

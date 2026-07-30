"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { Spin } from "antd";

import { apiGet } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";

export default function RootPage() {
  const router = useRouter();
  const { user, loading } = useAuth();

  useEffect(() => {
    if (loading) return;

    apiGet<{ setupComplete: boolean }>("/api/setup/status")
      .then(({ setupComplete }) => {
        if (!setupComplete) {
          router.replace("/setup");
          return;
        }
        if (user) {
          router.replace(user.role === "agent" ? "/chats" : "/dashboard");
        } else {
          router.replace("/login");
        }
      })
      .catch(() => router.replace("/login"));
  }, [loading, user, router]);

  return (
    <div className="flex h-screen items-center justify-center">
      <Spin size="large" />
    </div>
  );
}

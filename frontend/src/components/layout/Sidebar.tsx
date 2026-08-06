"use client";

import { useState } from "react";
import {
  CommentOutlined,
  DashboardOutlined,
  LogoutOutlined,
  MessageOutlined,
  SettingOutlined,
  UserOutlined,
} from "@ant-design/icons";
import { Menu, Segmented, message } from "antd";
import { useRouter, usePathname } from "next/navigation";

import { useAuth } from "@/context/AuthContext";
import { confirmAction } from "@/components/modals/confirm";
import { apiFetch } from "@/lib/api";
import { titleCase } from "@/lib/format";

// Sticky top-left sidebar, logo at top, nav flex-1 + scrollable with a
// thin custom scrollbar, user identity + availability + Logout pinned to
// the bottom (overview.md §6.0).
export function Sidebar() {
  const { user, logout } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  const canSeeOverview = user?.role === "admin" || user?.role === "super_admin";

  const items = [
    { key: "/chats", icon: <CommentOutlined />, label: "Chats" },
    ...(canSeeOverview ? [{ key: "/dashboard", icon: <DashboardOutlined />, label: "Overview" }] : []),
    { key: "/settings", icon: <SettingOutlined />, label: "Settings" },
    { key: "/profile", icon: <UserOutlined />, label: "Profile" },
  ];

  function handleLogout() {
    confirmAction({
      title: "Log out?",
      content: "You'll need to log in again to continue.",
      okText: "Log out",
      danger: true,
      onConfirm: async () => {
        await logout();
        router.push("/login");
      },
    });
  }

  return (
    <aside className="sticky top-0 flex h-screen w-60 flex-col border-r border-black/10 bg-white dark:border-white/10 dark:bg-neutral-900">
      <div className="flex h-16 shrink-0 items-center gap-2 border-b border-black/10 px-5 text-[15px] font-semibold tracking-tight dark:border-white/10">
        <MessageOutlined />
        LiveChat
      </div>

      <nav className="thin-scrollbar flex-1 overflow-y-auto">
        <Menu
          mode="inline"
          selectedKeys={[pathname]}
          items={items}
          onClick={({ key }) => router.push(key)}
          style={{ border: "none" }}
        />
      </nav>

      <div className="shrink-0 border-t border-black/10 p-3 dark:border-white/10">
        <UserSummary />

        <Menu
          mode="inline"
          selectable={false}
          items={[{ key: "logout", icon: <LogoutOutlined />, label: "Logout", danger: true }]}
          onClick={handleLogout}
          style={{ border: "none", marginTop: 4 }}
        />
      </div>
    </aside>
  );
}

// Who's logged in, their role, and a self-set Online/Offline availability
// toggle — the widget/routing side of this lives in backend/internal/
// routing.go's manual_status filter; this is purely the "let a user say
// they're stepping away without logging out" control surface.
function UserSummary() {
  const { user, setUser } = useAuth();
  const [savingStatus, setSavingStatus] = useState(false);

  if (!user) return null;

  const initial = user.display_name.charAt(0).toUpperCase() || "?";

  async function setStatus(next: "online" | "offline") {
    if (!user || next === user.manual_status) return;
    const previous = user.manual_status;
    setSavingStatus(true);
    setUser({ ...user, manual_status: next });
    try {
      await apiFetch("/api/profile", { method: "PATCH", body: JSON.stringify({ manualStatus: next }) });
    } catch {
      setUser({ ...user, manual_status: previous });
      message.error("Could not update your status");
    } finally {
      setSavingStatus(false);
    }
  }

  return (
    <div className="mb-1 flex flex-col gap-2 px-1 pb-2">
      <div className="flex items-center gap-2.5">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-black text-[13px] font-medium text-white dark:bg-white dark:text-black">
          {initial}
        </div>
        <div className="flex min-w-0 flex-col">
          <span className="truncate text-[13px] font-medium leading-tight">{user.display_name}</span>
          <span className="text-[11px] leading-tight text-neutral-500 dark:text-neutral-400">{titleCase(user.role)}</span>
        </div>
      </div>
      <Segmented
        block
        size="small"
        disabled={savingStatus}
        value={user.manual_status}
        onChange={(v) => setStatus(v as "online" | "offline")}
        options={[
          { label: <StatusOption dot="#22C55E" label="Online" />, value: "online" },
          { label: <StatusOption dot="#A3A3A3" label="Offline" />, value: "offline" },
        ]}
      />
    </div>
  );
}

function StatusOption({ dot, label }: { dot: string; label: string }) {
  return (
    <span className="flex items-center justify-center gap-1.5 px-1">
      <span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: dot }} />
      {label}
    </span>
  );
}

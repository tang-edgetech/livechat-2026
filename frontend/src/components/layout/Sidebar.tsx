"use client";

import {
  CommentOutlined,
  DashboardOutlined,
  LogoutOutlined,
  SettingOutlined,
  UserOutlined,
} from "@ant-design/icons";
import { Menu } from "antd";
import { useRouter, usePathname } from "next/navigation";

import { useAuth } from "@/context/AuthContext";
import { confirmAction } from "@/components/modals/confirm";

// Sticky top-left sidebar, logo at top, nav flex-1 + scrollable with a
// thin custom scrollbar, Logout pinned to the bottom (overview.md §6.0).
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
      <div className="flex h-16 shrink-0 items-center px-5 text-lg font-semibold">
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

      <div className="shrink-0 border-t border-black/10 p-2 dark:border-white/10">
        <Menu
          mode="inline"
          selectable={false}
          items={[{ key: "logout", icon: <LogoutOutlined />, label: "Logout", danger: true }]}
          onClick={handleLogout}
          style={{ border: "none" }}
        />
      </div>
    </aside>
  );
}

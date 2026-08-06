"use client";

import { usePathname, useRouter } from "next/navigation";

import { useAuth } from "@/context/AuthContext";
import { SETTINGS_GROUPS } from "@/components/settings/settingsSections";

// Persistent left-nav shared by every /settings/* page — real standalone
// routes per section (e.g. /settings/files) instead of a client-only tab
// switch, so a section is linkable/bookmarkable and the browser's own
// forward/back works within Settings. Hand-rolled rather than an antd
// <Menu> so the active item can carry a left accent bar + icon, matching
// the same craft level as the root Sidebar's own nav.
export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { user } = useAuth();
  const isStaff = user?.role === "admin" || user?.role === "super_admin";

  const visibleGroups = SETTINGS_GROUPS.map((g) => ({ ...g, items: g.items.filter((i) => !i.staffOnly || isStaff) })).filter(
    (g) => g.items.length > 0,
  );

  // /settings/files -> "files"; /settings/merchants/<uuid> -> "merchants"
  // (prefix match highlights the right nav item even from a deep page).
  const activeKey = pathname.split("/")[2] ?? "general";

  return (
    <div className="flex flex-col gap-6 md:flex-row md:items-start">
      <nav className="w-full shrink-0 md:sticky md:top-6 md:w-64">
        <div className="flex flex-col gap-5 rounded-lg border border-black/10 bg-white p-3 dark:border-white/10 dark:bg-neutral-900">
          {visibleGroups.map((group) => (
            <div key={group.key} className="flex flex-col gap-1">
              <div className="flex items-center gap-1.5 px-2 pb-1 text-[11px] font-semibold uppercase tracking-wide text-neutral-400 dark:text-neutral-500">
                <span className="text-[12px]">{group.icon}</span>
                {group.label}
              </div>
              {group.items.map((item) => {
                const active = item.key === activeKey;
                return (
                  <button
                    key={item.key}
                    type="button"
                    onClick={() => router.push(`/settings/${item.key}`)}
                    className={`flex items-center gap-2.5 rounded-md border-l-2 px-2.5 py-1.5 text-left text-[13.5px] transition-colors ${
                      active
                        ? "border-l-black bg-black/[0.04] font-medium text-neutral-900 dark:border-l-white dark:bg-white/[0.06] dark:text-white"
                        : "border-l-transparent text-neutral-600 hover:bg-black/[0.03] hover:text-neutral-900 dark:text-neutral-400 dark:hover:bg-white/[0.04] dark:hover:text-neutral-100"
                    }`}
                  >
                    <span className={active ? "text-[15px]" : "text-[15px] opacity-70"}>{item.icon}</span>
                    {item.label}
                  </button>
                );
              })}
            </div>
          ))}
        </div>
      </nav>
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  );
}

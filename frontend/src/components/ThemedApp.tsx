"use client";

import { useEffect } from "react";
import { App as AntdApp, ConfigProvider } from "antd";

import { useAuth } from "@/context/AuthContext";
import { THEMES, resolveTheme } from "@/lib/theme";
import { setThemedModal } from "@/components/modals/confirm";

// Hands confirmAction's Modal.confirm a context-bound instance the moment
// antd's <App> mounts, so it renders themed (and warning-free) instead of
// falling back to the static default look.
function ThemeBridge() {
  const { modal } = AntdApp.useApp();

  useEffect(() => {
    setThemedModal(modal);
    return () => setThemedModal(null);
  }, [modal]);

  return null;
}

// The one place theme state actually lives: resolves the logged-in user's
// Appearance preference (Profile tab) to an AntD theme, and toggles the
// `dark` class on <html> in lockstep so Tailwind's `dark:` utility classes
// and AntD's ConfigProvider tokens can never disagree (globals.css's
// `@custom-variant dark` reads that same class). Anonymous routes
// (login/setup/widget) render before a user is loaded and get the "light"
// default, which is correct.
export function ThemedApp({ children }: { children: React.ReactNode }) {
  const { user } = useAuth();
  const themeKey = resolveTheme(user?.theme_preference);

  useEffect(() => {
    document.documentElement.classList.toggle("dark", themeKey === "dark");
  }, [themeKey]);

  return (
    <ConfigProvider theme={THEMES[themeKey]}>
      <AntdApp>
        <ThemeBridge />
        {children}
      </AntdApp>
    </ConfigProvider>
  );
}

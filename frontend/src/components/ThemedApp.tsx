"use client";

import { useEffect } from "react";
import { ConfigProvider } from "antd";

import { useAuth } from "@/context/AuthContext";
import { THEMES, resolveTheme } from "@/lib/theme";

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

  return <ConfigProvider theme={THEMES[themeKey]}>{children}</ConfigProvider>;
}

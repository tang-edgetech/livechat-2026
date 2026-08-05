"use client";

import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";

import { apiGet, apiPost, ApiError } from "@/lib/api";

export type CurrentUser = {
  uuid: string;
  display_name: string;
  email: string;
  role: "super_admin" | "admin" | "agent";
  theme_preference: string;
};

type AuthContextValue = {
  user: CurrentUser | null;
  loading: boolean;
  sessionExpired: boolean;
  refresh: () => Promise<void>;
  setUser: (user: CurrentUser | null) => void;
  acknowledgeSessionExpired: () => void;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

// How often we proactively check /api/auth/me while a tab sits open and
// idle — this is what surfaces the 2-hour idle-logout modal (overview.md
// §6.0) even if the user never triggers another request themselves. The
// real enforcement is server-side (session.Validate); this is just so the
// UI notices promptly.
const HEARTBEAT_MS = 60_000;

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<CurrentUser | null>(null);
  const [loading, setLoading] = useState(true);
  const [sessionExpired, setSessionExpired] = useState(false);
  const hadUser = useRef(false);

  const refresh = useCallback(async () => {
    try {
      const me = await apiGet<CurrentUser>("/api/auth/me");
      setUser(me);
      hadUser.current = true;
      setSessionExpired(false);
    } catch (err) {
      setUser(null);
      if (hadUser.current && err instanceof ApiError && err.status === 401) {
        setSessionExpired(true);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial auth check on mount, then a heartbeat; both are intentional fetch-and-setState.
    refresh();
    const interval = setInterval(refresh, HEARTBEAT_MS);
    return () => clearInterval(interval);
  }, [refresh]);

  const acknowledgeSessionExpired = useCallback(() => {
    hadUser.current = false;
    setSessionExpired(false);
  }, []);

  // A deliberate logout must reset `hadUser` before the next heartbeat
  // tick, otherwise that tick's 401 (session already gone) gets
  // misread as an idle timeout and pops the "Session expired" modal
  // right after the user just chose to log out themselves.
  const logout = useCallback(async () => {
    try {
      await apiPost("/api/auth/logout");
    } finally {
      hadUser.current = false;
      setSessionExpired(false);
      setUser(null);
    }
  }, []);

  return (
    <AuthContext.Provider
      value={{ user, loading, sessionExpired, refresh, setUser, acknowledgeSessionExpired, logout }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}

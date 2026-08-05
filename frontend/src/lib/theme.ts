import { theme as antdTheme, type ThemeConfig } from "antd";

// Phase 10 redesign, benchmarked against Vercel's own dashboard: minimal,
// geometric, confident monochrome + one restrained accent — not a colored
// brand wash. Primary buttons/CTAs are neutral ink (near-black in light
// mode, near-white in dark mode), never a color chip; the one accent blue
// is reserved for links/focus/small highlights so it still reads as an
// accent. Crisp 1px borders are the primary depth cue (globals.css's
// `.ant-card`/`.ant-table`/etc. contrast override stays in place and isn't
// fought here); shadows are reserved for true overlays (dropdowns/modals),
// not resting cards.
//
// Known tradeoff (carried over from the first redesign pass): static
// `message.*`/`Modal.confirm` (used throughout via the shared
// `confirmAction`, see components/modals/confirm.tsx) don't consume this
// dynamic theme — they're called imperatively from event handlers, not
// JSX, so they can't trivially move to the `App.useApp()` hook pattern
// that would theme-match them. They keep antd's stock look (functionally
// unaffected) plus a harmless known console warning. Migrating that is a
// separate, real architectural change — not bundled into this redesign.
export type ThemeKey = "light" | "dark" | "violet";

export const THEME_KEYS: ThemeKey[] = ["light", "dark", "violet"];

export const THEME_LABELS: Record<ThemeKey, string> = {
  light: "Light",
  dark: "Dark",
  violet: "Violet",
};

// Vercel's actual scale — a tight neutral ramp, not an AntD default gray.
const NEUTRAL = {
  0: "#FFFFFF",
  50: "#FAFAFA",
  100: "#EAEAEA",
  400: "#999999",
  600: "#666666",
  800: "#333333",
  950: "#0A0A0A",
};

const ACCENT_BLUE = "#0070F3";
const ACCENT_VIOLET = "#6C5CE7";
const ACCENT_VIOLET_HOVER = "#8477F0";
const ACCENT_VIOLET_ACTIVE = "#5445D0";

const baseComponents = (radius: number) => ({
  Menu: { itemBorderRadius: radius - 2, itemMarginInline: 8 },
  Button: { borderRadius: radius, fontWeight: 500, primaryShadow: "none" },
  Card: { borderRadiusLG: radius + 2 },
  Tag: { borderRadiusSM: radius - 2 },
  Table: { borderRadiusLG: radius },
});

export const THEMES: Record<ThemeKey, ThemeConfig> = {
  light: {
    algorithm: antdTheme.defaultAlgorithm,
    token: {
      colorPrimary: NEUTRAL[950],
      colorLink: ACCENT_BLUE,
      colorLinkHover: "#3291FF",
      colorInfo: ACCENT_BLUE,
      colorBgLayout: NEUTRAL[50],
      borderRadius: 6,
      fontFamily: "var(--font-geist-sans), Arial, Helvetica, sans-serif",
    },
    components: {
      ...baseComponents(6),
      Menu: { ...baseComponents(6).Menu, itemSelectedBg: NEUTRAL[100], itemSelectedColor: NEUTRAL[950], itemHoverBg: NEUTRAL[50] },
    },
  },
  dark: {
    algorithm: antdTheme.darkAlgorithm,
    token: {
      colorPrimary: NEUTRAL[0],
      colorLink: "#3291FF",
      colorLinkHover: "#5CA8FF",
      colorInfo: "#3291FF",
      colorBgLayout: "#000000",
      borderRadius: 6,
      fontFamily: "var(--font-geist-sans), Arial, Helvetica, sans-serif",
    },
    components: {
      ...baseComponents(6),
      Menu: { ...baseComponents(6).Menu, itemSelectedBg: "#1A1A1A", itemSelectedColor: NEUTRAL[0], itemHoverBg: "#111111" },
    },
  },
  violet: {
    algorithm: antdTheme.defaultAlgorithm,
    token: {
      colorPrimary: ACCENT_VIOLET,
      colorLink: ACCENT_VIOLET,
      colorLinkHover: ACCENT_VIOLET_HOVER,
      colorInfo: ACCENT_VIOLET,
      colorBgLayout: NEUTRAL[50],
      borderRadius: 8,
      fontFamily: "var(--font-geist-sans), Arial, Helvetica, sans-serif",
    },
    components: {
      ...baseComponents(8),
      Menu: { ...baseComponents(8).Menu, itemSelectedBg: "#EEEBFD", itemSelectedColor: ACCENT_VIOLET_ACTIVE, itemHoverBg: "#F5F3FE" },
    },
  },
};

export function resolveTheme(key: string | undefined | null): ThemeKey {
  return THEME_KEYS.includes(key as ThemeKey) ? (key as ThemeKey) : "light";
}

// Small swatch chips for the Appearance picker (Profile page) — a real
// color preview beats a text label alone.
export const THEME_SWATCHES: Record<ThemeKey, { bg: string; primary: string; accent: string }> = {
  light: { bg: NEUTRAL[0], primary: NEUTRAL[950], accent: ACCENT_BLUE },
  dark: { bg: NEUTRAL[950], primary: NEUTRAL[0], accent: "#3291FF" },
  violet: { bg: NEUTRAL[0], primary: ACCENT_VIOLET, accent: ACCENT_VIOLET },
};

// Legacy export kept for any not-yet-updated call site — resolves to the
// default Light preset. New code should read the resolved theme from
// ThemedApp/AuthContext instead of importing a static constant.
export const THEME_PRIMARY = NEUTRAL[950];
export const THEME_ACCENT = ACCENT_BLUE;

import type { ThemeConfig } from "antd";

// Phase 9's "bold & colorful" redesign — the app previously ran on stock
// AntD with zero ConfigProvider theme. This governs color/radius/shadow
// only, never border color: globals.css's `.ant-card`/`.ant-table`/etc.
// border-contrast override (from the earlier border-contrast work) stays
// in place and isn't fought here.
//
// Known tradeoff: static `message.*`/`Modal.confirm` (used throughout via
// the shared `confirmAction`, see components/modals/confirm.tsx) don't
// consume this dynamic theme — they're called imperatively from event
// handlers, not JSX, so they can't trivially move to the `App.useApp()`
// hook pattern that would theme-match them. They keep antd's stock look
// (functionally unaffected) plus a harmless known console warning.
// Migrating that is a separate, real architectural change — not bundled
// into this redesign.
const PRIMARY = "#6C5CE7";
const PRIMARY_HOVER = "#8477F0";
const PRIMARY_ACTIVE = "#5445D0";
const ACCENT = "#F59E0B";

export const themeConfig: ThemeConfig = {
  token: {
    colorPrimary: PRIMARY,
    colorLink: PRIMARY,
    colorLinkHover: PRIMARY_HOVER,
    colorInfo: PRIMARY,
    borderRadius: 10,
    fontFamily: "var(--font-geist-sans), Arial, Helvetica, sans-serif",
  },
  components: {
    Menu: {
      itemSelectedBg: "#EEEBFD",
      itemSelectedColor: PRIMARY_ACTIVE,
      itemActiveBg: "#F5F3FE",
      itemHoverBg: "#F5F3FE",
      itemBorderRadius: 8,
      itemMarginInline: 8,
    },
    Button: {
      borderRadius: 8,
      fontWeight: 500,
      primaryShadow: "0 2px 6px rgba(108, 92, 231, 0.35)",
    },
    Card: {
      borderRadiusLG: 14,
      boxShadowTertiary: "0 2px 10px rgba(17, 24, 39, 0.06)",
    },
    Tag: {
      borderRadiusSM: 6,
      defaultBg: "#F5F3FE",
      defaultColor: PRIMARY_ACTIVE,
    },
    Table: {
      borderRadiusLG: 12,
      headerBg: "#FAFAFB",
    },
  },
};

export const THEME_PRIMARY = PRIMARY;
export const THEME_ACCENT = ACCENT;

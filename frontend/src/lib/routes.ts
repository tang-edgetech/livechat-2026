// Back-navigation convention (overview.md §6.0): the in-app back control
// never uses browser history — every page declares its parent route
// explicitly here, and back navigates there via router.push. Add an entry
// whenever a new page is introduced. Prefix rules are checked before the
// exact-match table, longest prefix first.
const EXACT_PARENT: Record<string, string> = {
  "/dashboard": "/dashboard",
  "/profile": "/dashboard",
  "/settings": "/dashboard",
  "/chats": "/chats",
};

const PREFIX_PARENT: [string, string][] = [
  ["/settings/users/", "/settings/users"],
  ["/settings/merchants/", "/settings/merchants"],
  ["/settings/bot-flows/", "/settings/bot"],
  // Every standalone settings section (general/files/embed/system/
  // canned-messages/automation/bot/users/merchants/visitors/integration/
  // audit) backs out to the dashboard, not to /settings itself (which is
  // just a redirect to the default section) — this catch-all must stay
  // last so the more specific rules above win first.
  ["/settings/", "/dashboard"],
  ["/chats/", "/chats"],
];

export function getParentRoute(pathname: string): string {
  for (const [prefix, parent] of PREFIX_PARENT) {
    if (pathname.startsWith(prefix)) return parent;
  }
  return EXACT_PARENT[pathname] ?? "/dashboard";
}

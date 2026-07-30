// Back-navigation convention (overview.md §6.0): the in-app back control
// never uses browser history — every page declares its parent route
// explicitly here, and back navigates there via router.push. Add an entry
// whenever a new page is introduced. Prefix rules are checked before the
// exact-match table, longest prefix first.
const EXACT_PARENT: Record<string, string> = {
  "/dashboard": "/dashboard",
  "/profile": "/dashboard",
  "/settings": "/dashboard",
};

const PREFIX_PARENT: [string, string][] = [
  ["/settings/users/", "/settings"],
  ["/settings/merchants/", "/settings"],
];

export function getParentRoute(pathname: string): string {
  for (const [prefix, parent] of PREFIX_PARENT) {
    if (pathname.startsWith(prefix)) return parent;
  }
  return EXACT_PARENT[pathname] ?? "/dashboard";
}

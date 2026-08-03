// Small formatting helpers shared across screens that display raw
// snake_case enum values (role, status, etc.) as plain-language labels
// (overview.md §6.0).
export function titleCase(value: string): string {
  return value
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}

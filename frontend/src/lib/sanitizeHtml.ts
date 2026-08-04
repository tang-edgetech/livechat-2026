import DOMPurify from "dompurify";

// Canned Messages / Automation rules can opt into HTML (message.metadata.isHtml
// / canned_message.is_html / automation_rule.is_html) so staff can style a
// message (bold, links, inline `style=`) instead of getting plain escaped
// text. DOMPurify's default profile is already a curated-safe tag/attribute
// set — this just adds `style`/`target` on top for the flexibility that was
// asked for. `<script>`, `on*` handlers, and `javascript:` URIs are stripped
// regardless of what's added here. Sanitize at render time, not just on
// save, since render is the actual enforcement point.
export function sanitizeHtml(html: string): string {
  return DOMPurify.sanitize(html, { ADD_ATTR: ["style", "target"] });
}

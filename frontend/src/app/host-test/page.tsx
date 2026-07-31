// Simulates a merchant's own website with the embed snippet dropped in —
// proves the actual delivery mechanism (overview.md §10.1: script tag +
// iframe), not just the widget page in isolation. Still rendered inside
// this app's own shell (Next.js can't easily serve a fully separate raw
// HTML document from within the same app), but the chat bubble/iframe
// you see here comes entirely from embed.js executing as a real browser
// would run it on any third-party site.
export default function HostTestPage() {
  return (
    <div style={{ fontFamily: "sans-serif", padding: 40 }}>
      <h1>Acme Hardware Co.</h1>
      <p>This is a stand-in for a merchant&apos;s own website, unrelated to the LiveChat app itself.</p>
      <p>The chat bubble in the corner comes entirely from the embed script below — same as a real customer site would use.</p>
      <script src="/embed.js" data-merchant-code="widgettest" async />
    </div>
  );
}

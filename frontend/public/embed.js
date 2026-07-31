(function () {
  // LiveChat embed snippet (overview.md §10.1). Vanilla JS, no
  // dependencies — this file is served as-is to arbitrary third-party
  // sites, so it can't assume anything about the host page's stack.
  //
  // Usage on a merchant's site:
  //   <script src="https://<this-app-domain>/embed.js" data-merchant-code="acme" async></script>
  //   Optional: data-token="<passthrough token>" for a logged-in visitor.

  var currentScript = document.currentScript;
  if (!currentScript) return;

  var merchantCode = currentScript.getAttribute("data-merchant-code");
  if (!merchantCode) {
    console.error("LiveChat embed: missing data-merchant-code attribute");
    return;
  }
  var token = currentScript.getAttribute("data-token") || "";
  var origin = new URL(currentScript.src).origin;

  var corner = currentScript.getAttribute("data-corner") || "bottom-right";
  var sideStyle = corner === "bottom-left" ? "left:20px;" : "right:20px;";

  var bubble = document.createElement("button");
  bubble.setAttribute("aria-label", "Open chat");
  bubble.style.cssText =
    "position:fixed;bottom:20px;" + sideStyle +
    "width:56px;height:56px;border-radius:50%;border:none;cursor:pointer;" +
    "background:#1677ff;color:#fff;font-size:24px;box-shadow:0 4px 12px rgba(0,0,0,0.2);z-index:2147483000;";
  bubble.textContent = "💬";

  var iframe = document.createElement("iframe");
  var src = origin + "/widget/" + encodeURIComponent(merchantCode);
  if (token) src += "?token=" + encodeURIComponent(token);
  iframe.src = src;
  iframe.style.cssText =
    "position:fixed;bottom:88px;" + sideStyle +
    "width:360px;height:520px;max-width:calc(100vw - 40px);max-height:calc(100vh - 120px);" +
    "border:none;border-radius:12px;box-shadow:0 8px 24px rgba(0,0,0,0.25);" +
    "z-index:2147483000;display:none;background:#fff;";
  iframe.title = "LiveChat";

  var open = false;
  bubble.addEventListener("click", function () {
    open = !open;
    iframe.style.display = open ? "block" : "none";
    bubble.textContent = open ? "✕" : "💬";
  });

  document.body.appendChild(iframe);
  document.body.appendChild(bubble);
})();

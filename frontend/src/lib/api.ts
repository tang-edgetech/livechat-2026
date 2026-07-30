// Relative path — Next.js rewrites (next.config.ts) proxy this to the Go
// backend server-side, so the browser only ever sees one origin and the
// session cookie is always same-site. See next.config.ts for why.
const API_BASE_URL = "";

export class ApiError extends Error {
  status: number;
  body: unknown;

  constructor(status: number, body: unknown) {
    const message =
      body && typeof body === "object" && "error" in body
        ? String((body as { error: unknown }).error)
        : `Request failed with status ${status}`;
    super(message);
    this.status = status;
    this.body = body;
  }
}

// Every action in this app goes through here — plain fetch, JSON in/out,
// session cookie carried via credentials:'include'. No page navigation,
// no polling: this is the single AJAX entry point every screen calls
// (overview.md §2/§6.0).
export async function apiFetch<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(`${API_BASE_URL}${path}`, {
    ...options,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(options.headers ?? {}),
    },
  });

  const contentType = res.headers.get("content-type") ?? "";
  const body = contentType.includes("application/json") ? await res.json() : await res.text();

  if (!res.ok) {
    throw new ApiError(res.status, body);
  }
  return body as T;
}

export function apiGet<T>(path: string) {
  return apiFetch<T>(path, { method: "GET" });
}

export function apiPost<T>(path: string, data?: unknown) {
  return apiFetch<T>(path, { method: "POST", body: data !== undefined ? JSON.stringify(data) : undefined });
}

export function apiPatch<T>(path: string, data?: unknown) {
  return apiFetch<T>(path, { method: "PATCH", body: data !== undefined ? JSON.stringify(data) : undefined });
}

export function apiDelete<T>(path: string) {
  return apiFetch<T>(path, { method: "DELETE" });
}

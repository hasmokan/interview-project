export interface SharedFile {
  file_id: string;
  name: string;
  mime_type: string;
  size_bytes: number;
  created_at: number;
  share_status: "shared" | "unshared";
  share_url: string | null;
}
export interface Session {
  username: string;
  csrf_token: string;
  max_upload_bytes: number;
}
export interface FilePage {
  items: SharedFile[];
  total: number;
  page: number;
  page_size: number;
}
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}
let csrf = "";
export async function api<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const method = options.method || "GET";
  if (!["GET", "HEAD"].includes(method) && path !== "/api/login" && !csrf)
    await getSession();
  const headers = new Headers(options.headers);
  if (csrf) headers.set("X-CSRF-Token", csrf);
  const response = await fetch(path, {
    ...options,
    headers,
    credentials: "same-origin",
    cache: "no-store",
  });
  const value = await response.json();
  if (!response.ok) {
    if (
      response.status === 401 &&
      path !== "/api/login" &&
      !path.startsWith("/api/public/")
    ) {
      csrf = "";
      window.location.assign("/login");
    }
    throw new ApiError(value.message || "操作失败，请重试", response.status);
  }
  return value as T;
}
export async function getSession() {
  const session = await api<Session>("/api/session");
  csrf = session.csrf_token;
  return session;
}
export function sizeLabel(size: number) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / 1024 / 1024).toFixed(1)} MB`;
}
export function previewable(mime: string) {
  return [
    "image/png",
    "image/jpeg",
    "image/gif",
    "image/webp",
    "application/pdf",
  ].includes(mime);
}

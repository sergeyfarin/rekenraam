import { invoke } from "@tauri-apps/api/core";

const rawApiBaseUrl = import.meta.env.PUBLIC_API_BASE_URL?.trim() ?? "";

function getApiBaseUrl(): string | null {
  if (!rawApiBaseUrl) {
    return null;
  }

  return rawApiBaseUrl.endsWith("/") ? rawApiBaseUrl.slice(0, -1) : rawApiBaseUrl;
}

async function parseError(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as { detail?: string | { msg?: string }[] };
    if (typeof body.detail === "string") {
      return body.detail;
    }
    if (Array.isArray(body.detail) && body.detail.length > 0) {
      return body.detail.map((item) => item.msg ?? "request failed").join(", ");
    }
  } catch {
    // Ignore JSON parse failures and fall through to status text.
  }

  return `${response.status} ${response.statusText}`.trim();
}

export async function apiGet<T>(path: string): Promise<T> {
  const baseUrl = getApiBaseUrl();
  if (baseUrl === null) {
    throw new Error("PUBLIC_API_BASE_URL is not configured");
  }

  const response = await fetch(`${baseUrl}${path}`);
  if (!response.ok) {
    throw new Error(await parseError(response));
  }

  return (await response.json()) as T;
}

export async function apiGetWithTauriFallback<T>(path: string, command: string, args?: Record<string, unknown>): Promise<T> {
  try {
    return await apiGet<T>(path);
  } catch {
    return await invoke<T>(command, args);
  }
}
const rawApiBaseUrl = import.meta.env.PUBLIC_API_BASE_URL?.trim() ?? "";

export type ApiValidationError = {
  loc?: (string | number)[];
  msg: string;
  type?: string;
};

export class ApiError extends Error {
  readonly status: number;
  readonly statusText: string;
  readonly detail: string;
  readonly validationErrors: ApiValidationError[];
  readonly requestId: string | null;
  readonly responseText: string;

  constructor(input: {
    status: number;
    statusText: string;
    detail: string;
    validationErrors?: ApiValidationError[];
    requestId?: string | null;
    responseText?: string;
  }) {
    super(input.detail || `${input.status} ${input.statusText}`.trim());
    this.name = "ApiError";
    this.status = input.status;
    this.statusText = input.statusText;
    this.detail = input.detail || `${input.status} ${input.statusText}`.trim();
    this.validationErrors = input.validationErrors ?? [];
    this.requestId = input.requestId ?? null;
    this.responseText = input.responseText ?? "";
  }
}

function getApiBaseUrl(): string | null {
  if (rawApiBaseUrl) {
    return rawApiBaseUrl.endsWith("/") ? rawApiBaseUrl.slice(0, -1) : rawApiBaseUrl;
  }

  if (typeof window !== "undefined") {
    const { protocol, origin } = window.location;
    if (protocol === "http:" || protocol === "https:") {
      return `${origin}/api/v1`;
    }
  }

  return null;
}

export function hasApiBaseUrl(): boolean {
  return getApiBaseUrl() !== null;
}

function normalizeValidationErrors(detail: unknown): ApiValidationError[] {
  if (!Array.isArray(detail)) {
    return [];
  }
  return detail
    .map((item) => {
      if (item && typeof item === "object") {
        const row = item as Record<string, unknown>;
        const msg = typeof row.msg === "string" ? row.msg : "request failed";
        const loc = Array.isArray(row.loc)
          ? row.loc.filter((part): part is string | number => typeof part === "string" || typeof part === "number")
          : undefined;
        const type = typeof row.type === "string" ? row.type : undefined;
        return { loc, msg, type };
      }
      return { msg: String(item) };
    });
}

async function parseError(response: Response): Promise<ApiError> {
  const requestId = response.headers.get("x-request-id") ?? response.headers.get("x-correlation-id");
  let responseText = "";
  let detail = "";
  let validationErrors: ApiValidationError[] = [];

  try {
    responseText = await response.text();
    const body = responseText ? JSON.parse(responseText) as { detail?: unknown } : {};
    if (typeof body.detail === "string") {
      detail = body.detail;
    } else {
      validationErrors = normalizeValidationErrors(body.detail);
      if (validationErrors.length > 0) {
        detail = validationErrors.map((item) => item.msg).join(", ");
      }
    }
  } catch {
    // Ignore parse failures and fall through to status text / raw text.
  }

  return new ApiError({
    status: response.status,
    statusText: response.statusText,
    detail: detail || responseText || `${response.status} ${response.statusText}`.trim(),
    validationErrors,
    requestId,
    responseText,
  });
}

async function apiJson<T>(path: string, init?: RequestInit): Promise<T> {
  const baseUrl = getApiBaseUrl();
  if (baseUrl === null) {
    throw new Error("PUBLIC_API_BASE_URL is not configured");
  }

  const response = await fetch(`${baseUrl}${path}`, {
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
    ...init,
  });
  if (!response.ok) {
    throw await parseError(response);
  }

  const text = await response.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

export async function apiGet<T>(path: string): Promise<T> {
  return apiJson<T>(path);
}

export async function apiPost<TResponse, TBody>(path: string, body: TBody): Promise<TResponse> {
  return apiJson<TResponse>(path, {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export async function apiPut<TResponse, TBody>(path: string, body: TBody): Promise<TResponse> {
  return apiJson<TResponse>(path, {
    method: "PUT",
    body: JSON.stringify(body),
  });
}

export async function apiDelete<TResponse>(path: string): Promise<TResponse> {
  const baseUrl = getApiBaseUrl();
  if (baseUrl === null) {
    throw new Error("PUBLIC_API_BASE_URL is not configured");
  }

  const response = await fetch(`${baseUrl}${path}`, { method: "DELETE", credentials: "include" });
  if (!response.ok) {
    throw await parseError(response);
  }

  const text = await response.text();
  return (text ? JSON.parse(text) : undefined) as TResponse;
}

export async function apiText(path: string, init?: RequestInit): Promise<string> {
  const baseUrl = getApiBaseUrl();
  if (baseUrl === null) {
    throw new Error("PUBLIC_API_BASE_URL is not configured");
  }

  const response = await fetch(`${baseUrl}${path}`, {
    credentials: "include",
    ...init,
    headers: {
      ...(init?.headers ?? {}),
    },
  });
  if (!response.ok) {
    throw await parseError(response);
  }

  return response.text();
}

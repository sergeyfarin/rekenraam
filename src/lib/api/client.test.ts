import { describe, expect, it, vi, afterEach } from "vitest";
import { ApiError, apiGet } from "$lib/api/client";
import { formatApiError } from "$lib/utils";

afterEach(() => {
  vi.restoreAllMocks();
});

function mockFetch(response: Response) {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response));
}

describe("api client errors", () => {
  it("throws ApiError for string detail responses", async () => {
    mockFetch(new Response(JSON.stringify({ detail: "locked range" }), {
      status: 409,
      statusText: "Conflict",
      headers: { "content-type": "application/json", "x-request-id": "req-123" },
    }));

    await expect(apiGet("/example")).rejects.toMatchObject({
      name: "ApiError",
      status: 409,
      detail: "locked range",
      requestId: "req-123",
    });
  });

  it("preserves FastAPI validation errors", async () => {
    mockFetch(new Response(JSON.stringify({
      detail: [
        { loc: ["body", "email"], msg: "Input should be a valid email", type: "value_error" },
      ],
    }), {
      status: 422,
      statusText: "Unprocessable Entity",
      headers: { "content-type": "application/json" },
    }));

    try {
      await apiGet("/example");
      throw new Error("expected ApiError");
    } catch (error) {
      expect(error).toBeInstanceOf(ApiError);
      expect(formatApiError(error)).toBe("email: Input should be a valid email");
    }
  });

  it("formats common status classes for UI display", () => {
    expect(formatApiError(new ApiError({ status: 401, statusText: "Unauthorized", detail: "nope" })))
      .toMatch(/session has expired/i);
    expect(formatApiError(new ApiError({ status: 403, statusText: "Forbidden", detail: "nope" })))
      .toMatch(/permission/i);
    expect(formatApiError(new ApiError({ status: 500, statusText: "Server Error", detail: "boom", requestId: "abc" })))
      .toContain("abc");
  });
});

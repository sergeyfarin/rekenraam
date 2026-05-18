import { beforeEach, describe, expect, it, vi } from "vitest";
import { apiPost, hasApiBaseUrl } from "$lib/api/client";
import { refreshFxRatesNow } from "./commodities";

vi.mock("$lib/api/client", () => ({
  apiDelete: vi.fn(),
  apiGet: vi.fn(),
  apiPost: vi.fn(),
  apiPut: vi.fn(),
  hasApiBaseUrl: vi.fn(() => true),
}));

describe("commodities API client", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(hasApiBaseUrl).mockReturnValue(true);
    vi.mocked(apiPost).mockResolvedValue({ ok: true });
  });

  it("posts the requested book id for manual FX refresh", async () => {
    await refreshFxRatesNow(42);

    expect(apiPost).toHaveBeenCalledWith("/pricing/refresh/run", { book_id: 42 });
  });
});
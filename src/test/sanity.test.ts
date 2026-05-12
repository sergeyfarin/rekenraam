import { describe, expect, it } from "vitest";

describe("vitest sanity", () => {
  it("runs", () => {
    expect(1 + 1).toBe(2);
  });

  it("has jsdom DOM globals available", () => {
    const div = document.createElement("div");
    div.textContent = "ok";
    expect(div.textContent).toBe("ok");
  });
});

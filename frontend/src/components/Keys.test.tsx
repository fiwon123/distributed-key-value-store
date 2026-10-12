import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Keys } from "./Keys";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("Keys", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("renders the keys returned by the API", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ keys: ["alpha", "beta"] })));

    render(<Keys />);

    expect(await screen.findByText("alpha")).toBeInTheDocument();
    expect(screen.getByText("beta")).toBeInTheDocument();
  });

  it("shows an empty state when there are no keys", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ keys: [] })));

    render(<Keys />);

    expect(await screen.findByText("no keys")).toBeInTheDocument();
  });

  it("shows the error message when listing fails", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ error: "unavailable" }, 503)));

    render(<Keys />);

    expect(await screen.findByText("unavailable")).toBeInTheDocument();
  });
});

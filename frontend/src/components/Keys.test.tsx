import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

    render(<Keys onSelect={() => {}} />);

    expect(await screen.findByRole("button", { name: "alpha" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "beta" })).toBeInTheDocument();
  });

  it("calls onSelect with the clicked key", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ keys: ["alpha", "beta"] })));
    const onSelect = vi.fn();
    const user = userEvent.setup();

    render(<Keys onSelect={onSelect} />);
    await user.click(await screen.findByRole("button", { name: "beta" }));

    expect(onSelect).toHaveBeenCalledWith("beta");
  });

  it("shows an empty state when there are no keys", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ keys: [] })));

    render(<Keys onSelect={() => {}} />);

    expect(await screen.findByText("no keys")).toBeInTheDocument();
  });

  it("shows the error message when listing fails", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ error: "unavailable" }, 503)));

    render(<Keys onSelect={() => {}} />);

    expect(await screen.findByText("unavailable")).toBeInTheDocument();
  });
});

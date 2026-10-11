import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { KVForm } from "./KVForm";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("KVForm", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("puts the key and value and reports the outcome", async () => {
    const fetchMock = vi.fn().mockResolvedValue(json({ key: "greeting", value: "hello" }));
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();

    render(<KVForm />);
    await user.type(screen.getByLabelText("key"), "greeting");
    await user.type(screen.getByLabelText("value"), "hello");
    await user.click(screen.getByRole("button", { name: "PUT" }));

    expect(await screen.findByText("PUT greeting")).toBeInTheDocument();
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/v1/kv/greeting");
    expect(init.method).toBe("PUT");
    expect(init.body).toBe(JSON.stringify({ value: "hello" }));
  });

  it("gets a value and shows it", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ key: "greeting", value: "hello" })));
    const user = userEvent.setup();

    render(<KVForm />);
    await user.type(screen.getByLabelText("key"), "greeting");
    await user.click(screen.getByRole("button", { name: "GET" }));

    expect(await screen.findByText("hello")).toBeInTheDocument();
  });

  it("reports a missing key", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ error: "key not found" }, 404)));
    const user = userEvent.setup();

    render(<KVForm />);
    await user.type(screen.getByLabelText("key"), "missing");
    await user.click(screen.getByRole("button", { name: "GET" }));

    expect(await screen.findByText("key not found")).toBeInTheDocument();
  });

  it("deletes a key and reports the outcome", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 204 })));
    const user = userEvent.setup();

    render(<KVForm />);
    await user.type(screen.getByLabelText("key"), "greeting");
    await user.click(screen.getByRole("button", { name: "DELETE" }));

    expect(await screen.findByText("DELETE greeting")).toBeInTheDocument();
  });

  it("shows the error message when a put fails", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ error: "internal error" }, 500)));
    const user = userEvent.setup();

    render(<KVForm />);
    await user.type(screen.getByLabelText("key"), "k");
    await user.type(screen.getByLabelText("value"), "v");
    await user.click(screen.getByRole("button", { name: "PUT" }));

    expect(await screen.findByText("internal error")).toBeInTheDocument();
  });
});

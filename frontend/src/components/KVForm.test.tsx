import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { KVForm } from "./KVForm";
import { useKVConsole } from "../hooks/useKVConsole";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Harness renders the form wired to the real console hook (the form itself is
// presentational; see App.tsx).
function Harness() {
  return <KVForm state={useKVConsole()} />;
}

describe("KVForm", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("puts the key and value, reports the outcome, and clears the form", async () => {
    const fetchMock = vi.fn().mockResolvedValue(json({ key: "greeting", value: "hello" }));
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();

    render(<Harness />);
    await user.type(screen.getByLabelText("key"), "greeting");
    await user.type(screen.getByLabelText("value"), "hello");
    await user.click(screen.getByRole("button", { name: "PUT" }));

    expect(await screen.findByText("PUT greeting = hello")).toBeInTheDocument();
    expect(screen.getByLabelText("key")).toHaveValue("");
    expect(screen.getByLabelText("value")).toHaveValue("");
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/v1/kv/greeting");
    expect(init.method).toBe("PUT");
    expect(init.body).toBe(JSON.stringify({ value: "hello" }));
  });

  it("disables PUT until both a key and a value are present", async () => {
    vi.stubGlobal("fetch", vi.fn());
    const user = userEvent.setup();

    render(<Harness />);
    const put = screen.getByRole("button", { name: "PUT" });
    expect(put).toBeDisabled();

    await user.type(screen.getByLabelText("key"), "greeting");
    expect(put).toBeDisabled();

    await user.type(screen.getByLabelText("value"), "hello");
    expect(put).toBeEnabled();
  });

  it("gets a value, reports the call, and loads it into the form", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ key: "greeting", value: "hello" })));
    const user = userEvent.setup();

    render(<Harness />);
    await user.type(screen.getByLabelText("key"), "greeting");
    await user.click(screen.getByRole("button", { name: "GET" }));

    expect(await screen.findByText("GET greeting: hello")).toBeInTheDocument();
    expect(screen.getByLabelText("key")).toHaveValue("greeting");
    expect(screen.getByLabelText("value")).toHaveValue("hello");
  });

  it("reports a missing key on GET", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ error: "key not found" }, 404)));
    const user = userEvent.setup();

    render(<Harness />);
    await user.type(screen.getByLabelText("key"), "missing");
    await user.click(screen.getByRole("button", { name: "GET" }));

    expect(await screen.findByText("key not found")).toBeInTheDocument();
  });

  it("deletes a key, reports the outcome, and clears the form", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 204 })));
    const user = userEvent.setup();

    render(<Harness />);
    await user.type(screen.getByLabelText("key"), "greeting");
    await user.type(screen.getByLabelText("value"), "hello");
    await user.click(screen.getByRole("button", { name: "DELETE" }));

    expect(await screen.findByText("DELETE greeting")).toBeInTheDocument();
    expect(screen.getByLabelText("key")).toHaveValue("");
    expect(screen.getByLabelText("value")).toHaveValue("");
  });

  it("reports a missing key on DELETE", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ error: "key not found" }, 404)));
    const user = userEvent.setup();

    render(<Harness />);
    await user.type(screen.getByLabelText("key"), "missing");
    await user.click(screen.getByRole("button", { name: "DELETE" }));

    expect(await screen.findByText("key not found")).toBeInTheDocument();
  });

  it("shows the error message when a put fails", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ error: "internal error" }, 500)));
    const user = userEvent.setup();

    render(<Harness />);
    await user.type(screen.getByLabelText("key"), "k");
    await user.type(screen.getByLabelText("value"), "v");
    await user.click(screen.getByRole("button", { name: "PUT" }));

    expect(await screen.findByText("internal error")).toBeInTheDocument();
  });
});

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function statusPayload(id: string, role: string) {
  return { id, role, term: 3, leader: "node1", commitIndex: 2, lastApplied: 2, peers: 1 };
}

// stubClusterFetch stands in for the cluster/status/key endpoints App polls.
function stubClusterFetch() {
  const mock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url === "/cluster") {
      return json({ self: "node1", peers: ["node2"] });
    }
    if (url === "/v1/status") {
      return json(statusPayload("node1", "leader"));
    }
    if (url === "/node/node2/v1/status") {
      return json(statusPayload("node2", "follower"));
    }
    if (url === "/v1/kv") {
      return json({ keys: [] });
    }
    return json({ error: "not found" }, 404);
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

describe("App", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("renders a card per cluster node with its role", async () => {
    stubClusterFetch();

    render(<App />);

    expect(await screen.findByRole("heading", { name: "node1" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "node2" })).toBeInTheDocument();
    expect(await screen.findByText("leader", { selector: "span.badge" })).toBeInTheDocument();
    expect(await screen.findByText("follower", { selector: "span.badge" })).toBeInTheDocument();
  });

  it("switches between the cluster and API tabs", async () => {
    stubClusterFetch();
    const user = userEvent.setup();

    render(<App />);

    // Cluster is the default tab.
    expect(await screen.findByRole("heading", { name: "node1" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "API" }));
    expect(await screen.findByRole("heading", { name: "API playground" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "node1" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cluster" }));
    expect(await screen.findByRole("heading", { name: "node1" })).toBeInTheDocument();
  });

  it("shows the About tab with usage instructions", async () => {
    stubClusterFetch();
    const user = userEvent.setup();

    render(<App />);

    await user.click(screen.getByRole("button", { name: "About" }));

    expect(await screen.findByRole("heading", { name: "About" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "How to use" })).toBeInTheDocument();
  });

  it("toggles the theme and reflects it on the document", async () => {
    stubClusterFetch();
    const user = userEvent.setup();

    render(<App />);

    // No stored theme and no OS signal in jsdom → light.
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");

    await user.click(screen.getByRole("button", { name: /switch to dark mode/i }));

    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(localStorage.getItem("theme")).toBe("dark");
    expect(screen.getByRole("button", { name: /switch to light mode/i })).toBeInTheDocument();
  });
});

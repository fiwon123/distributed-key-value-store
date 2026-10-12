import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
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

describe("App", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders a card per cluster node with its role", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
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
        return json({ error: "not found" }, 404);
      }),
    );

    render(<App />);

    expect(await screen.findByRole("heading", { name: "node1" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "node2" })).toBeInTheDocument();
    expect(await screen.findByText("leader", { selector: "span.badge" })).toBeInTheDocument();
    expect(await screen.findByText("follower", { selector: "span.badge" })).toBeInTheDocument();
  });

  it("switches between the cluster and API tabs", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
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
        return json({ error: "not found" }, 404);
      }),
    );
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
});

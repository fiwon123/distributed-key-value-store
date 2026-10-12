import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Playground } from "./Playground";

function json(body: unknown, status = 200, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

// stubFetch answers /cluster for the target-node selector and returns `response`
// for every other request, recording the calls so tests can inspect them.
function stubFetch(response: Response) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    if (String(input) === "/cluster") {
      return json({ self: "node1", peers: ["node2"] });
    }
    return response;
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function requestCalls(fetchMock: ReturnType<typeof vi.fn>): [string, RequestInit][] {
  return fetchMock.mock.calls
    .map((c) => c as [string, RequestInit])
    .filter(([url]) => url !== "/cluster");
}

describe("Playground", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("sends a GET and shows the status and pretty-printed body", async () => {
    const fetchMock = stubFetch(json({ id: "node1", role: "leader" }));
    const user = userEvent.setup();

    render(<Playground />);
    await user.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByTestId("playground-status")).toHaveTextContent("200");
    expect(screen.getByTestId("playground-body")).toHaveTextContent(/"id": "node1"/);

    const calls = requestCalls(fetchMock);
    expect(calls).toHaveLength(1);
    expect(calls[0][0]).toBe("/v1/status");
    expect(calls[0][1].method).toBe("GET");
  });

  it("shows X-Raft-Leader on a 503 without retrying", async () => {
    const fetchMock = stubFetch(
      json({ error: "write unavailable" }, 503, { "X-Raft-Leader": "node2" }),
    );
    const user = userEvent.setup();

    render(<Playground />);
    await user.selectOptions(screen.getByLabelText("method"), "PUT");
    await user.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByTestId("playground-status")).toHaveTextContent("503");
    expect(screen.getByTestId("playground-leader")).toHaveTextContent("node2");

    // No retry through the proxy: exactly one request was sent.
    expect(requestCalls(fetchMock)).toHaveLength(1);
  });

  it("targets a peer node through the /node/{id} proxy", async () => {
    const fetchMock = stubFetch(json({ id: "node2" }));
    const user = userEvent.setup();

    render(<Playground />);
    await screen.findByRole("option", { name: "node2" });
    await user.selectOptions(screen.getByLabelText("target"), "node2");
    await user.click(screen.getByRole("button", { name: "Send" }));

    const calls = requestCalls(fetchMock);
    expect(calls[0][0]).toBe("/node/node2/v1/status");
  });

  it("sends a PUT with the JSON body", async () => {
    const fetchMock = stubFetch(json({ key: "greeting", value: "hi" }));
    const user = userEvent.setup();

    render(<Playground />);
    await user.selectOptions(screen.getByLabelText("method"), "PUT");
    fireEvent.change(screen.getByLabelText("path"), { target: { value: "/v1/kv/greeting" } });
    fireEvent.change(screen.getByLabelText("body"), { target: { value: '{"value":"hi"}' } });
    await user.click(screen.getByRole("button", { name: "Send" }));

    const calls = requestCalls(fetchMock);
    expect(calls[0][0]).toBe("/v1/kv/greeting");
    expect(calls[0][1].method).toBe("PUT");
    expect(calls[0][1].body).toBe('{"value":"hi"}');
  });

  it("shows a network error instead of swallowing it", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input) === "/cluster") {
          return json({ self: "node1", peers: [] });
        }
        throw new Error("network down");
      }),
    );
    const user = userEvent.setup();

    render(<Playground />);
    await user.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText("network down")).toBeInTheDocument();
  });
});

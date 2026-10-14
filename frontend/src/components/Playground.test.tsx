import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Playground } from "./Playground";

function json(body: unknown, status = 200, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

// A trimmed OpenAPI document, enough to exercise the spec-driven presets.
const SPEC = {
  openapi: "3.0.3",
  paths: {
    "/v1/kv/{key}": {
      put: { summary: "Set a key to a value.", responses: { "200": {}, "400": {}, "503": {} } },
      get: { summary: "Read a key.", responses: { "200": {}, "404": {} } },
      delete: { summary: "Delete a key.", responses: { "204": {}, "404": {} } },
    },
    "/v1/status": {
      get: { summary: "Current node and cluster status.", responses: { "200": {} } },
    },
  },
};

const SPEC_URL = "/v1/openapi.json";

// stubFetch answers /cluster for the target selector and the OpenAPI spec,
// returning `response` for every other (actual) request, recording the calls.
function stubFetch(response: Response, spec: unknown = SPEC) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url === "/cluster") return json({ self: "node1", peers: ["node2"] });
    if (url === SPEC_URL) return json(spec);
    return response;
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function requestCalls(fetchMock: ReturnType<typeof vi.fn>): [string, RequestInit][] {
  return fetchMock.mock.calls
    .map((c) => c as [string, RequestInit])
    .filter(([url]) => url !== "/cluster" && url !== SPEC_URL);
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
    fireEvent.change(screen.getByLabelText("path"), { target: { value: "/v1/kv/greeting" } });
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
    fireEvent.change(screen.getByLabelText("path"), { target: { value: "/v1/kv/greeting" } });
    await user.selectOptions(screen.getByLabelText("method"), "PUT");
    fireEvent.change(screen.getByLabelText("body"), { target: { value: '{"value":"hi"}' } });
    await user.click(screen.getByRole("button", { name: "Send" }));

    const calls = requestCalls(fetchMock);
    expect(calls[0][0]).toBe("/v1/kv/greeting");
    expect(calls[0][1].method).toBe("PUT");
    expect(calls[0][1].body).toBe('{"value":"hi"}');
  });

  it("links to the Swagger UI for the full contract", () => {
    stubFetch(json({}));

    render(<Playground />);

    expect(screen.getByRole("link", { name: "Swagger UI" })).toHaveAttribute("href", "/docs");
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

  it("derives presets from the spec and fills method + path on select", async () => {
    const fetchMock = stubFetch(json({ ok: true }));
    const user = userEvent.setup();

    render(<Playground />);
    // The preset list is populated from the document, not a hardcoded list.
    await screen.findByRole("option", { name: /PUT \/v1\/kv\/\{key\}/ });

    await user.selectOptions(screen.getByLabelText("preset"), "PUT /v1/kv/{key}");
    expect(screen.getByLabelText("method")).toHaveValue("PUT");
    // The templated param is stripped so the user completes the key.
    expect(screen.getByLabelText("path")).toHaveValue("/v1/kv/");

    await user.click(screen.getByRole("button", { name: "Send" }));
    const calls = requestCalls(fetchMock);
    expect(calls[0][0]).toBe("/v1/kv/");
    expect(calls[0][1].method).toBe("PUT");
  });

  it("restricts the method to those documented for the matched path", async () => {
    stubFetch(json({}));

    render(<Playground />);
    // /v1/status is GET-only in the spec.
    await screen.findByRole("option", { name: /GET \/v1\/status/ });

    await waitFor(() => {
      const options = [...screen.getByLabelText("method").querySelectorAll("option")].map(
        (o) => (o as HTMLOptionElement).value,
      );
      expect(options).toEqual(["GET"]);
    });

    // A free-text path the spec does not describe keeps the full method list.
    fireEvent.change(screen.getByLabelText("path"), { target: { value: "/custom" } });
    await waitFor(() => {
      const options = [...screen.getByLabelText("method").querySelectorAll("option")].map(
        (o) => (o as HTMLOptionElement).value,
      );
      expect(options).toEqual(["GET", "PUT", "DELETE"]);
    });
  });

  it("shows the summary and declared response codes for the matched endpoint", async () => {
    stubFetch(json({}));

    render(<Playground />);
    expect(await screen.findByTestId("playground-doc")).toHaveTextContent(
      "Current node and cluster status.",
    );
    expect(screen.getByTestId("playground-doc")).toHaveTextContent("declares 200");
  });

  it("falls back to built-in presets when the spec cannot be fetched", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "/cluster") return json({ self: "node1", peers: [] });
        if (url === SPEC_URL) throw new Error("spec unavailable");
        return json({ id: "node1", role: "leader" });
      }),
    );
    const user = userEvent.setup();

    render(<Playground />);
    // The built-in preset is still offered.
    await screen.findByRole("option", { name: /GET \/v1\/status/ });

    await user.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByTestId("playground-status")).toHaveTextContent("200");
  });
});

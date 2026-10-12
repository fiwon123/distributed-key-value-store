import { afterEach, describe, expect, it, vi } from "vitest";
import { deleteKey, fetchCluster, fetchStatus, getKey, putKey } from "./api";

function json(body: unknown, status = 200, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

describe("api", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("retries a write once through X-Raft-Leader's /node proxy", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        json({ error: "write unavailable" }, 503, { "X-Raft-Leader": "node2" }),
      )
      .mockResolvedValueOnce(json({ key: "k", value: "v" }));
    vi.stubGlobal("fetch", fetchMock);

    await putKey("k", "v");

    expect(fetchMock).toHaveBeenCalledTimes(2);
    const [url, init] = fetchMock.mock.calls[1] as [string, RequestInit];
    expect(url).toBe("/node/node2/v1/kv/k");
    expect(init.method).toBe("PUT");
    expect(init.body).toBe(JSON.stringify({ value: "v" }));
  });

  it("does not retry a 503 without a leader header", async () => {
    const fetchMock = vi.fn().mockResolvedValue(json({ error: "write unavailable" }, 503));
    vi.stubGlobal("fetch", fetchMock);

    await expect(putKey("k", "v")).rejects.toThrow("write unavailable");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("reads a value, retrying through the leader proxy when needed", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(json({ error: "unavailable" }, 503, { "X-Raft-Leader": "node3" }))
      .mockResolvedValueOnce(json({ key: "k", value: "from-leader" }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getKey("k")).resolves.toBe("from-leader");
    expect(fetchMock.mock.calls[1][0]).toBe("/node/node3/v1/kv/k");
  });

  it("maps a 404 on get to a key-not-found error", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ error: "key not found" }, 404)));

    await expect(getKey("missing")).rejects.toThrow("key not found");
  });

  it("deletes without an error body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(deleteKey("k")).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("surfaces the API error message for a failed request", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ error: "internal error" }, 500)));

    await expect(putKey("k", "v")).rejects.toThrow("internal error");
  });

  it("propagates a cluster discovery error", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ error: "boom" }, 500)));

    await expect(fetchCluster()).rejects.toThrow("boom");
  });

  it("returns parsed status", async () => {
    const status = {
      id: "node1",
      role: "leader",
      term: 3,
      leader: "node1",
      commitIndex: 2,
      lastApplied: 2,
      peers: 1,
    };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(status)));

    await expect(fetchStatus("/v1/status")).resolves.toEqual(status);
  });
});

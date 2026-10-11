import type { ClusterInfo, Status } from "./types";

async function errorMessage(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string };
    return body.error ?? res.statusText;
  } catch {
    return res.statusText || `HTTP ${res.status}`;
  }
}

// request issues a same-origin call. On 503 (not the leader) it retries once
// through the /node/{id} proxy named by the X-Raft-Leader header.
async function request(method: string, path: string, body?: unknown): Promise<Response> {
  const init: RequestInit = { method };
  if (body !== undefined) {
    init.headers = { "Content-Type": "application/json" };
    init.body = JSON.stringify(body);
  }
  const res = await fetch(path, init);
  if (res.status !== 503) return res;

  const leader = res.headers.get("X-Raft-Leader");
  if (!leader) return res;
  return fetch(`/node/${encodeURIComponent(leader)}${path}`, init);
}

export async function fetchCluster(): Promise<ClusterInfo> {
  const res = await fetch("/cluster");
  if (!res.ok) throw new Error(await errorMessage(res));
  return (await res.json()) as ClusterInfo;
}

export async function fetchStatus(path: string): Promise<Status> {
  const res = await fetch(path);
  if (!res.ok) throw new Error(await errorMessage(res));
  return (await res.json()) as Status;
}

export interface KeyValue {
  key: string;
  value: string;
}

export async function putKey(key: string, value: string): Promise<void> {
  const res = await request("PUT", `/v1/kv/${encodeURIComponent(key)}`, { value });
  if (!res.ok) throw new Error(await errorMessage(res));
}

export async function getKey(key: string): Promise<string> {
  const res = await request("GET", `/v1/kv/${encodeURIComponent(key)}`);
  if (res.status === 404) {
    throw new Error("key not found");
  }
  if (!res.ok) {
    throw new Error(await errorMessage(res));
  }
  const body = (await res.json()) as { value: string };
  return body.value;
}

export async function deleteKey(key: string): Promise<void> {
  const res = await request("DELETE", `/v1/kv/${encodeURIComponent(key)}`);
  if (!res.ok) {
    throw new Error(await errorMessage(res));
  }
}

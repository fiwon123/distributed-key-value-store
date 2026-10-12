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

// --- OpenAPI spec view --------------------------------------------------------
// A minimal view of GET /v1/openapi.json, used to drive the API playground.
// The full contract is enforced server-side by the Go httpapi tests; here we
// only need each path's methods, summaries, and declared status codes.

export interface Endpoint {
  method: string; // upper-case HTTP method, e.g. "GET"
  path: string; // OpenAPI path template, e.g. "/v1/kv/{key}"
  summary: string;
  responses: number[]; // declared status codes, ascending
}

// HTTP methods recognised in a path item, in a stable display order.
const SPEC_METHODS = ["GET", "PUT", "POST", "DELETE", "PATCH"] as const;

interface OpenAPIOperation {
  summary?: string;
  responses?: Record<string, unknown>;
}

interface OpenAPIPathItem {
  get?: OpenAPIOperation;
  put?: OpenAPIOperation;
  post?: OpenAPIOperation;
  delete?: OpenAPIOperation;
  patch?: OpenAPIOperation;
}

interface OpenAPIDoc {
  paths?: Record<string, OpenAPIPathItem>;
}

// endpointsFromSpec flattens an OpenAPI document into the endpoint list the
// playground offers as presets.
export function endpointsFromSpec(doc: OpenAPIDoc): Endpoint[] {
  const out: Endpoint[] = [];
  for (const [path, item] of Object.entries(doc.paths ?? {})) {
    for (const method of SPEC_METHODS) {
      const op = item[method.toLowerCase() as keyof OpenAPIPathItem];
      if (!op) continue;
      out.push({
        method,
        path,
        summary: op.summary ?? "",
        responses: Object.keys(op.responses ?? {})
          .map(Number)
          .filter((code) => Number.isInteger(code))
          .toSorted((a, b) => a - b),
      });
    }
  }
  return out;
}

export async function fetchEndpoints(): Promise<Endpoint[]> {
  const res = await fetch("/v1/openapi.json");
  if (!res.ok) throw new Error(await errorMessage(res));
  return endpointsFromSpec((await res.json()) as OpenAPIDoc);
}

// pathMatchesTemplate reports whether a concrete request path matches an
// OpenAPI path template, treating each "{param}" segment as any non-empty
// segment (e.g. /v1/kv/greeting matches /v1/kv/{key}).
export function pathMatchesTemplate(template: string, path: string): boolean {
  const t = template.split("/");
  const p = path.split("/");
  if (t.length !== p.length) return false;
  return t.every((segment, i) => {
    const isParam = segment.startsWith("{") && segment.endsWith("}");
    return isParam ? p[i] !== "" : segment === p[i];
  });
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

// RawResponse is the unprocessed result of a request, surfaced to the API
// playground. Unlike request(), nothing is retried: a 503 from a non-leader is
// returned as-is so the caller can see the status and the X-Raft-Leader header.
export interface RawResponse {
  status: number;
  statusText: string;
  ok: boolean;
  leader: string;
  body: string;
}

export async function rawRequest(
  method: string,
  path: string,
  body?: string,
): Promise<RawResponse> {
  const init: RequestInit = { method };
  if (body) {
    init.headers = { "Content-Type": "application/json" };
    init.body = body;
  }
  const res = await fetch(path, init);
  return {
    status: res.status,
    statusText: res.statusText,
    ok: res.ok,
    leader: res.headers.get("X-Raft-Leader") ?? "",
    body: await res.text(),
  };
}

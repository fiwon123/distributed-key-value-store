import { useEffect, useMemo, useState } from "react";
import {
  type Endpoint,
  fetchCluster,
  fetchEndpoints,
  pathMatchesTemplate,
  rawRequest,
  type RawResponse,
} from "../api";

// Used until the spec loads and whenever it cannot be fetched, so the
// playground always offers something useful.
const FALLBACK_ENDPOINTS: Endpoint[] = [
  {
    method: "PUT",
    path: "/v1/kv/{key}",
    summary: "Set a key to a value.",
    responses: [200, 400, 500, 503],
  },
  {
    method: "GET",
    path: "/v1/kv/{key}",
    summary: "Read a key.",
    responses: [200, 404, 500, 503],
  },
  {
    method: "DELETE",
    path: "/v1/kv/{key}",
    summary: "Delete a key.",
    responses: [204, 404, 500, 503],
  },
  {
    method: "GET",
    path: "/v1/status",
    summary: "Current node and cluster status.",
    responses: [200],
  },
  {
    method: "GET",
    path: "/v1/openapi.json",
    summary: "This OpenAPI document.",
    responses: [200],
  },
];

// Methods offered for a path the spec does not describe.
const FALLBACK_METHODS = ["GET", "PUT", "DELETE"];

// presetPath turns a path template into a fillable prefix: the user completes
// any "{param}" (e.g. /v1/kv/{key} -> /v1/kv/).
function presetPath(template: string): string {
  return template.replace(/\{[^}]*\}/g, "");
}

// pretty renders a JSON body indented; anything else is shown verbatim.
function pretty(body: string): string {
  try {
    return JSON.stringify(JSON.parse(body), null, 2);
  } catch {
    return body;
  }
}

export function Playground() {
  const [endpoints, setEndpoints] = useState<Endpoint[]>(FALLBACK_ENDPOINTS);
  const [method, setMethod] = useState<string>("GET");
  const [path, setPath] = useState("/v1/status");
  const [body, setBody] = useState('{"value":"hello"}');
  const [node, setNode] = useState("");
  const [nodes, setNodes] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<RawResponse>();
  const [error, setError] = useState("");

  // The target selector is a convenience; if discovery fails, requests still
  // go to the local node.
  useEffect(() => {
    let cancelled = false;
    fetchCluster()
      .then((info) => {
        if (!cancelled) setNodes([info.self, ...info.peers]);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  // The presets come from the node's own contract. Keep the built-in fallback
  // if the spec cannot be fetched, so the playground never goes empty.
  useEffect(() => {
    let cancelled = false;
    fetchEndpoints()
      .then((derived) => {
        if (!cancelled && derived.length > 0) setEndpoints(derived);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  // Endpoints whose path template matches what the user is typing.
  const matched = useMemo(
    () => endpoints.filter((e) => pathMatchesTemplate(e.path, path)),
    [endpoints, path],
  );

  // Documented methods for the current path, or the fallback list for a
  // free-text path the spec does not describe.
  const availableMethods = useMemo(
    () => (matched.length > 0 ? [...new Set(matched.map((e) => e.method))] : FALLBACK_METHODS),
    [matched],
  );

  // Derive the method to use during render rather than correcting state in an
  // effect: when the path narrows to a path with a different method set (e.g.
  // /v1/status only allows GET), the effective method follows without an extra
  // render.
  const effectiveMethod = availableMethods.includes(method) ? method : availableMethods[0];
  const current = matched.find((e) => e.method === effectiveMethod);

  async function send() {
    setBusy(true);
    setError("");
    setResult(undefined);
    try {
      const target = node ? `/node/${encodeURIComponent(node)}${path}` : path;
      setResult(
        await rawRequest(effectiveMethod, target, effectiveMethod === "PUT" ? body : undefined),
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="playground">
      <h2>API playground</h2>
      <p className="muted playground__intro">
        Send a request straight through and read the raw response — including Raft's{" "}
        <code>503</code> + <code>X-Raft-Leader</code> redirect when you target a follower. For the
        full contract with schemas and try-it-out forms, open the{" "}
        <a href="/docs" target="_blank" rel="noreferrer">
          Swagger UI
        </a>
        .
      </p>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void send();
        }}
      >
        <label>
          method
          <select value={effectiveMethod} onChange={(e) => setMethod(e.target.value)}>
            {availableMethods.map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
        </label>
        <label>
          target
          <select value={node} onChange={(e) => setNode(e.target.value)}>
            <option value="">this node</option>
            {nodes.map((id) => (
              <option key={id} value={id}>
                {id}
              </option>
            ))}
          </select>
        </label>
        <label>
          preset
          <select
            value=""
            onChange={(e) => {
              const chosen = endpoints.find((x) => `${x.method} ${x.path}` === e.target.value);
              if (!chosen) return;
              setMethod(chosen.method);
              setPath(presetPath(chosen.path));
            }}
          >
            <option value="">custom…</option>
            {endpoints.map((e) => (
              <option key={`${e.method} ${e.path}`} value={`${e.method} ${e.path}`}>
                {e.method} {e.path}
                {e.summary ? ` — ${e.summary}` : ""}
              </option>
            ))}
          </select>
        </label>
        <label className="playground__path">
          path
          <input
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder="/v1/status"
            required
          />
        </label>
        {effectiveMethod === "PUT" && (
          <label className="playground__body">
            body
            <textarea value={body} onChange={(e) => setBody(e.target.value)} rows={3} />
          </label>
        )}
        <div className="actions">
          <button type="submit" disabled={busy}>
            Send
          </button>
        </div>
      </form>

      {current && (
        <p className="muted playground__doc" data-testid="playground-doc">
          {current.summary}
          {current.summary ? " · " : ""}
          declares {current.responses.length > 0 ? current.responses.join(", ") : "no responses"}
        </p>
      )}

      {error && <p className="error">{error}</p>}

      {result && (
        <div className="response">
          <p data-testid="playground-status">
            <span className={result.ok ? "ok" : "error"}>
              {result.status}
              {result.statusText ? ` ${result.statusText}` : ""}
            </span>
            {result.leader && (
              <span className="muted" data-testid="playground-leader">
                {" · "}X-Raft-Leader: <strong>{result.leader}</strong>
              </span>
            )}
          </p>
          <pre className="response__body" data-testid="playground-body">
            {pretty(result.body)}
          </pre>
        </div>
      )}
    </section>
  );
}

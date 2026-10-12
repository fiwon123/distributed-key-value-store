import { useEffect, useState } from "react";
import { fetchCluster, rawRequest, type RawResponse } from "../api";

const METHODS = ["GET", "PUT", "DELETE"] as const;

// The documented endpoints, offered as one-click path presets. The key form is
// left for the user to finish (e.g. /v1/kv/greeting).
const PATH_PRESETS = ["/v1/kv/", "/v1/status", "/v1/openapi.json"];

// pretty renders a JSON body indented; anything else is shown verbatim.
function pretty(body: string): string {
  try {
    return JSON.stringify(JSON.parse(body), null, 2);
  } catch {
    return body;
  }
}

export function Playground() {
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

  async function send() {
    setBusy(true);
    setError("");
    setResult(undefined);
    try {
      const target = node ? `/node/${encodeURIComponent(node)}${path}` : path;
      setResult(await rawRequest(method, target, method === "PUT" ? body : undefined));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="playground">
      <h2>API playground</h2>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void send();
        }}
      >
        <label>
          method
          <select value={method} onChange={(e) => setMethod(e.target.value)}>
            {METHODS.map((m) => (
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
          <select value="" onChange={(e) => e.target.value && setPath(e.target.value)}>
            <option value="">custom…</option>
            {PATH_PRESETS.map((p) => (
              <option key={p} value={p}>
                {p}
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
        {method === "PUT" && (
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

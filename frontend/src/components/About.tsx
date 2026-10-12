export function About() {
  return (
    <div style={{ maxWidth: "800px", margin: "0 auto", padding: "1rem" }}>
      <h2>About</h2>
      <p>
        Distributed Key-Value Store is a minimal, educational distributed key-value store built from
        scratch with a hand-rolled Raft consensus algorithm.
      </p>

      <h3>Quick Start</h3>
      <ol>
        <li>
          <strong>Start a node</strong>: run <code>go run ./backend/cmd/kvnode</code> for a single
          node, or <code>make cluster-smoke</code> for a local 3-node cluster.
        </li>
        <li>
          <strong>Open the dashboard</strong>: browse to <code>http://localhost:8080</code>.
        </li>
        <li>
          <strong>View the cluster</strong>: the <em>Cluster</em> tab shows each node's role, term,
          leader, and commit/applied indices.
        </li>
        <li>
          <strong>Interact with KV</strong>: use the <em>API</em> tab to PUT, GET, and DELETE keys,
          or call the API directly:
          <pre
            style={{
              background: "var(--bg-primary)",
              padding: "0.75rem",
              borderRadius: "4px",
              overflow: "auto",
            }}
          >
            {`curl -X PUT http://localhost:8080/v1/kv/greeting -H 'Content-Type: application/json' -d '{"value":"hello"}'
curl http://localhost:8080/v1/kv/greeting
curl -X DELETE http://localhost:8080/v1/kv/greeting`}
          </pre>
        </li>
        <li>
          <strong>Read the docs</strong>: Swagger UI lives at <code>/docs</code>, and the raw
          contract at <code>/v1/openapi.json</code>.
        </li>
      </ol>

      <h3>Features</h3>
      <ul>
        <li>Single-node key-value store with an HTTP API</li>
        <li>Hand-rolled Raft leader election and log replication</li>
        <li>On-disk log, automatic snapshots, and log compaction</li>
        <li>Embedded dashboard with an API playground and Swagger UI</li>
      </ul>

      <p style={{ marginTop: "1rem" }}>
        Learn more on{" "}
        <a
          href="https://github.com/fiwon123/distributed-key-value-store"
          target="_blank"
          rel="noreferrer"
        >
          GitHub
        </a>
        .
      </p>
    </div>
  );
}

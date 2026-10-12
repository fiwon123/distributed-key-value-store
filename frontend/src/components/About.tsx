export function About() {
  return (
    <div style={{ maxWidth: "800px", margin: "0 auto", padding: "1rem" }}>
      <h2>About</h2>
      <p>
        Distributed Key-Value Store is a minimal, educational distributed key-value store built from
        scratch with a hand-rolled Raft consensus algorithm.
      </p>

      <h3>Quick Start Tutorial</h3>
      <ol>
        <li>
          <strong>Start a node</strong>: Run <code>go run ./backend/cmd/kvnode</code> for a single
          node, or use <code>make cluster-smoke</code> for a local 3-node cluster.
        </li>
        <li>
          <strong>Access the dashboard</strong>: Open <code>http://localhost:8080</code> in your
          browser to view the cluster status.
        </li>
        <li>
          <strong>View cluster</strong>: Use the <em>Cluster</em> tab to see node roles, terms,
          leader, and commit/applied indices.
        </li>
        <li>
          <strong>Interact with KV</strong>: Use the <em>API</em> tab (Playground) to PUT, GET, and
          DELETE keys via the HTTP API. You can also use <code>curl</code>:
          <pre
            style={{
              background: "var(--bg-primary)",
              padding: "0.75rem",
              borderRadius: "4px",
              overflow: "auto",
            }}
          >
            {`curl -X PUT http://localhost:8080/v1/kv/greeting -d '{"value":"hello"}' -H "Content-Type: application/json"
curl http://localhost:8080/v1/kv/greeting
curl -X DELETE http://localhost:8080/v1/kv/greeting`}
          </pre>
        </li>
        <li>
          <strong>Check status</strong>: GET <code>/v1/status</code> returns node state (role, term,
          leader, commit index, last applied, peer count).
        </li>
        <li>
          <strong>API Docs</strong>: Visit <code>/docs</code> for Swagger UI documentation of the
          HTTP API.
        </li>
      </ol>

      <h3>Features</h3>
      <ul>
        <li>Single-node KV with HTTP API</li>
        <li>Raft leader election and log replication</li>
        <li>Persistence, snapshots, and log compaction</li>
        <li>
          Embedded web dashboard with Swagger UI at <code>/docs</code>
        </li>
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

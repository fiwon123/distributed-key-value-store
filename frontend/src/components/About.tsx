export function About() {
  return (
    <div style={{ maxWidth: "800px", margin: "0 auto", padding: "1rem" }}>
      <h2>About</h2>
      <p>
        Distributed Key-Value Store is a minimal, educational distributed key-value store built from
        scratch with a hand-rolled Raft consensus algorithm.
      </p>

      <h3>How to Use</h3>
      <ol>
        <li>
          <strong>Start a node</strong>: Run a single node or a small cluster (3 nodes by default).
        </li>
        <li>
          <strong>View cluster</strong>: Use the <em>Cluster</em> tab to see node roles, terms,
          leader, and commit/applied indices.
        </li>
        <li>
          <strong>Interact with KV</strong>: Use the <em>API</em> tab (Playground) to PUT, GET, and
          DELETE keys via the HTTP API.
        </li>
        <li>
          <strong>Proxy to peers</strong>: The dashboard can proxy requests to peer nodes when
          needed.
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

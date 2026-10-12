export function About() {
  return (
    <div className="about">
      <h2>About</h2>
      <p className="about__lead">
        A minimal, educational distributed key-value store built from scratch with a hand-rolled
        Raft consensus algorithm. Writes go through the leader, commit to a quorum, and apply to an
        in-memory state machine on every node.
      </p>

      <h3>How to use</h3>
      <ol>
        <li>
          <strong>Start a node</strong>: run <code>go run ./backend/cmd/kvnode</code> for a single
          node, or <code>make cluster-smoke</code> for a local 3-node cluster.
        </li>
        <li>
          <strong>Open the dashboard</strong>: browse to <code>http://localhost:8080</code>.
        </li>
        <li>
          <strong>Watch the cluster</strong>: the <em>Cluster</em> tab shows each node's role, term,
          leader, and commit/applied indices, refreshing live.
        </li>
        <li>
          <strong>Read and write keys</strong>: use the key/value form on the <em>Cluster</em> tab,
          or the <em>API</em> tab to issue raw requests with a JSON body.
        </li>
        <li>
          <strong>Or call the API directly</strong>:
          <pre>
            <code>{`curl -X PUT http://localhost:8080/v1/kv/greeting \\
  -H 'Content-Type: application/json' -d '{"value":"hello"}'

curl http://localhost:8080/v1/kv/greeting
curl -X DELETE http://localhost:8080/v1/kv/greeting`}</code>
          </pre>
        </li>
        <li>
          <strong>Read the docs</strong>: Swagger UI lives at <code>/docs</code> and the raw
          contract at <code>/v1/openapi.json</code>.
        </li>
      </ol>

      <h3>Features</h3>
      <ul>
        <li>Single-node key-value store with an HTTP API</li>
        <li>Hand-rolled Raft leader election and log replication</li>
        <li>On-disk log, automatic snapshots, and log compaction</li>
        <li>Embedded dashboard with light/dark themes, an API playground, and Swagger UI</li>
      </ul>

      <p>
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

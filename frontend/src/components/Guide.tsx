export function Guide() {
  return (
    <div className="guide">
      <h2>Guide</h2>
      <p className="guide__lead">
        A minimal, educational distributed key-value store built from scratch with a hand-rolled
        Raft consensus algorithm. Writes go through the leader, commit to a quorum, and apply to an
        in-memory state machine on every node.
      </p>

      <h3>Using the dashboard</h3>
      <p className="muted">No terminal required — everything here happens on this page.</p>
      <ol>
        <li>
          <strong>Open the cluster view</strong>: the <em>Cluster</em> tab shows one card per node
          with its live role (leader / follower), term, and commit/applied indices.
        </li>
        <li>
          <strong>Write a key</strong>: type a key (e.g. <code>greeting</code>) and a value (e.g.{" "}
          <code>hello</code>) in the <em>keys</em> form, then press <strong>PUT</strong>. The value
          is committed through Raft and applied on every node.
        </li>
        <li>
          <strong>Read it back</strong>: click the key under <em>stored keys</em> — the form loads
          its value and reports the result. Or type a key and press <strong>GET</strong>.
        </li>
        <li>
          <strong>Update it</strong>: edit the value and press <strong>PUT</strong> again.
        </li>
        <li>
          <strong>Delete it</strong>: press <strong>DELETE</strong> — the key is removed from every
          node and the form clears. Deleting a key that does not exist reports{" "}
          <em>key not found</em>.
        </li>
        <li>
          <strong>See the raw API</strong>: the <em>API</em> tab sends requests straight through and
          shows Raft's leader redirect (a <code>503</code> with an <code>X-Raft-Leader</code> hint)
          when you target a follower.
        </li>
      </ol>

      <h3>Running a cluster</h3>
      <p className="muted">
        A single node cannot demonstrate failover. Start more than one and the Cluster tab shows
        every node automatically.
      </p>
      <details>
        <summary>CLI (optional)</summary>
        <pre>
          <code>{`# start a local 3-node cluster
make cluster-smoke

# or drive the API directly
curl -X PUT http://localhost:8080/v1/kv/greeting \\
  -H 'Content-Type: application/json' -d '{"value":"hello"}'
curl http://localhost:8080/v1/kv/greeting
curl -X DELETE http://localhost:8080/v1/kv/greeting`}</code>
        </pre>
      </details>

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

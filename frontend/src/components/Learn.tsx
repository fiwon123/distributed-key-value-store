// The Learn tab: the theory behind a Raft-based replicated store, with small
// inline SVG visuals so there is no extra dependency. Kept honest about what
// this implementation actually provides (a CP store with leader reads).

function CapTriangle() {
  return (
    <svg
      className="learn__figure"
      viewBox="0 0 420 260"
      role="img"
      aria-label="CAP triangle. A system keeps two of consistency, availability, and partition tolerance during a partition. This store is CP: consistency plus partition tolerance."
    >
      <line className="learn__side" x1="40" y1="220" x2="380" y2="220" />
      <line className="learn__side" x1="40" y1="220" x2="210" y2="40" />
      <line className="learn__emph" x1="210" y1="40" x2="380" y2="220" />
      <circle className="learn__dot" cx="210" cy="40" r="5" />
      <circle className="learn__dot" cx="40" cy="220" r="5" />
      <circle className="learn__dot" cx="380" cy="220" r="5" />
      <text className="learn__label" x="210" y="26" textAnchor="middle">
        Consistency
      </text>
      <text className="learn__label" x="34" y="240" textAnchor="start">
        Availability
      </text>
      <text className="learn__label" x="386" y="240" textAnchor="end">
        Partition tolerance
      </text>
      <text className="learn__sub" x="250" y="120" textAnchor="middle">
        CP
      </text>
      <text className="learn__sub" x="250" y="138" textAnchor="middle">
        this store
      </text>
    </svg>
  );
}

function QuorumDiagram() {
  return (
    <svg
      className="learn__figure learn__figure--wide"
      viewBox="0 0 560 150"
      role="img"
      aria-label="Quorum intersection in a five-node cluster. Two quorums of three nodes overlap on the middle node, so no two conflicting majorities can both succeed."
    >
      <rect className="learn__qa" x="20" y="30" rx="10" width="235" height="90" />
      <text className="learn__sub learn__sub--accent" x="30" y="22">
        Quorum A — f+1 = 3
      </text>
      <rect className="learn__qb" x="195" y="30" rx="10" width="235" height="90" />
      <text className="learn__sub learn__sub--candidate" x="520" y="22" textAnchor="end">
        Quorum B — f+1 = 3
      </text>
      <circle className="learn__node" cx="75" cy="75" r="16" />
      <circle className="learn__node" cx="150" cy="75" r="16" />
      <circle className="learn__overlap" cx="235" cy="75" r="16" />
      <circle className="learn__node" cx="320" cy="75" r="16" />
      <circle className="learn__node" cx="395" cy="75" r="16" />
      <text className="learn__index" x="75" y="79" textAnchor="middle">
        1
      </text>
      <text className="learn__index" x="150" y="79" textAnchor="middle">
        2
      </text>
      <text className="learn__index" x="235" y="79" textAnchor="middle">
        3
      </text>
      <text className="learn__index" x="320" y="79" textAnchor="middle">
        4
      </text>
      <text className="learn__index" x="395" y="79" textAnchor="middle">
        5
      </text>
      <text className="learn__sub" x="235" y="142" textAnchor="middle">
        the overlap (node 3) is what makes safety hold
      </text>
    </svg>
  );
}

export function Learn() {
  return (
    <div className="learn">
      <h2>Theoretical Foundations</h2>
      <p className="guide__lead">
        A Raft-based key-value store sits at the intersection of several areas of theoretical
        computer science. This tab summarizes the ideas the code exists to demonstrate — and says
        plainly where this implementation sits.
      </p>

      <h3>Consensus Theory</h3>
      <p>
        The core problem is <strong>distributed consensus</strong>: getting multiple nodes to agree
        on a sequence of operations despite failures. Raft is a crash-fault-tolerant (CFT) protocol
        — it assumes nodes either work correctly or stop, never act maliciously (unlike Byzantine
        fault tolerance).
      </p>
      <div className="learn__columns">
        <div>
          <h4>Safety — What Must Never Happen</h4>
          <ul>
            <li>
              <strong>Election Safety</strong> — at most one leader per term.
            </li>
            <li>
              <strong>Leader Append-Only</strong> — a leader never overwrites existing log entries.
            </li>
            <li>
              <strong>Log Matching</strong> — a shared index and term implies identical prior
              entries.
            </li>
            <li>
              <strong>Leader Completeness</strong> — every committed entry appears in all future
              leaders' logs.
            </li>
          </ul>
        </div>
        <div>
          <h4>Liveness — What Must Eventually Happen</h4>
          <ul>
            <li>A leader is eventually elected.</li>
            <li>Log entries are eventually committed.</li>
          </ul>
        </div>
      </div>

      <h3>Replicated State Machines</h3>
      <p>
        Raft implements <strong>state machine replication</strong>: each node runs the same
        deterministic state machine (the KV map) and applies the same commands in the same order. If
        all nodes start in the same initial state and apply the same operations, they reach the same
        final state — that equivalence is the fundamental correctness argument.
      </p>

      <h3>CAP Theorem</h3>
      <p>
        The CAP theorem (Brewer's theorem, formalized by Gilbert &amp; Lynch, 2002) states that
        during a network partition a system must choose between <strong>C</strong>onsistency and{" "}
        <strong>A</strong>vailability. This store is a <strong>CP</strong> system: it keeps
        consistency and partition tolerance, and gives up availability when a majority is
        unreachable — writes are rejected rather than accepted best-effort.
      </p>
      <CapTriangle />
      <p>
        A refinement is <strong>PACELC</strong>: even when there <strong>E</strong>lse is no
        partition, there is a tradeoff between <strong>L</strong>atency and <strong>C</strong>
        onsistency — waiting for a majority acknowledgement is slower than answering from one node,
        but stronger.
      </p>

      <h3>Quorum / Majority Logic</h3>
      <p>
        The structural backbone is <strong>quorum intersection</strong>: in a cluster of{" "}
        <code>2f+1</code> nodes, any two quorums of size <code>f+1</code> must overlap. That overlap
        is what guarantees safety — no two conflicting leaders or commits can both succeed.
      </p>
      <QuorumDiagram />
      <p className="muted">
        This implementation requires <code>⌊N/2⌋ + 1</code> acknowledgements —{" "}
        <code>(peers+1)/2 + 1</code> in <code>Node.majority()</code> — which is correct for even
        cluster sizes too.
      </p>

      <h3>Consistency Models</h3>
      <p>What clients are promised, from strongest to weakest:</p>
      <table className="learn__table">
        <thead>
          <tr>
            <th>Model</th>
            <th>Guarantee</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>Linearizability (strongest)</td>
            <td>Operations appear to happen atomically in real-time order.</td>
          </tr>
          <tr>
            <td>Sequential consistency</td>
            <td>Some global order consistent with each client's program order.</td>
          </tr>
          <tr>
            <td>Causal consistency</td>
            <td>Preserves happens-before relationships.</td>
          </tr>
          <tr>
            <td>Eventual consistency (weakest)</td>
            <td>All replicas converge eventually.</td>
          </tr>
        </tbody>
      </table>
      <p>
        Raft provides <strong>linearizable writes</strong> (committed entries are totally ordered).
        Making reads linearizable in general also needs ReadIndex or lease reads, which this MVP
        does not do — reads are served by the leader instead.
      </p>

      <h3>Formal Verification &amp; Model Checking</h3>
      <p>
        The Raft paper and implementations use <strong>TLA+</strong> (Leslie Lamport's specification
        language) to specify and verify safety properties. Model checkers exhaustively explore the
        state space to prove no safety violation is possible.
      </p>

      <h3>The Asynchronous Model</h3>
      <p>
        Raft assumes an <strong>asynchronous network</strong> (no bound on message delay) and uses
        randomized election timeouts to break symmetry. This connects to the{" "}
        <strong>FLP impossibility</strong> result (Fischer, Lynch &amp; Paterson, 1985):
        deterministic consensus is impossible in a purely asynchronous system with even one crash
        fault — randomized timeouts are the standard workaround.
      </p>

      <p className="muted">
        In summary: consensus (safety + liveness), state machine replication, quorum arithmetic,
        CAP/PACELC tradeoffs, linearizability, and the asynchronous model with randomized liveness.
        The same material lives in the project README.
      </p>
    </div>
  );
}

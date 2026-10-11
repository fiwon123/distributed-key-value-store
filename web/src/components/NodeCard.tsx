import type { Status } from "../types";

interface Props {
  id: string;
  status?: Status;
  error?: string;
}

export function NodeCard({ id, status, error }: Props) {
  const leader = status?.role === "leader";
  return (
    <section className={leader ? "node node--leader" : "node"} data-testid={`node-${id}`}>
      <header className="node__header">
        <h2>{id}</h2>
        {status ? <span className={`badge badge--${status.role}`}>{status.role}</span> : null}
      </header>
      {error ? (
        <p className="error">{error}</p>
      ) : status ? (
        <dl className="facts">
          <dt>term</dt>
          <dd>{status.term}</dd>
          <dt>leader</dt>
          <dd>{status.leader || "—"}</dd>
          <dt>commit</dt>
          <dd>{status.commitIndex}</dd>
          <dt>applied</dt>
          <dd>{status.lastApplied}</dd>
          <dt>peers</dt>
          <dd>{status.peers}</dd>
        </dl>
      ) : (
        <p className="muted">loading…</p>
      )}
    </section>
  );
}

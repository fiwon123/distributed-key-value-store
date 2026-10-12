import { useEffect, useState } from "react";
import { fetchCluster, fetchStatus } from "./api";
import { KVForm } from "./components/KVForm";
import { NodeCard } from "./components/NodeCard";
import { Playground } from "./components/Playground";
import type { ClusterInfo, Status } from "./types";

type Tab = "cluster" | "api";

function statusPath(self: string, id: string): string {
  return id === self ? "/v1/status" : `/node/${encodeURIComponent(id)}/v1/status`;
}

function describe(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

interface Polled {
  id: string;
  status?: Status;
  error?: string;
}

export default function App() {
  const [tab, setTab] = useState<Tab>("cluster");
  const [cluster, setCluster] = useState<ClusterInfo>();
  const [clusterError, setClusterError] = useState("");
  const [statuses, setStatuses] = useState<Record<string, Status>>({});
  const [statusErrors, setStatusErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const info = await fetchCluster();
        if (!cancelled) {
          setCluster(info);
          setClusterError("");
        }
      } catch (err) {
        if (!cancelled) {
          setClusterError(describe(err));
        }
      }
    }
    void load();
    const timer = setInterval(() => void load(), 5000);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    if (!cluster) {
      return;
    }
    const { self, peers } = cluster;
    const ids = [self, ...peers];
    let cancelled = false;

    async function poll() {
      const results: Polled[] = await Promise.all(
        ids.map(async (id) => {
          try {
            return { id, status: await fetchStatus(statusPath(self, id)) };
          } catch (err) {
            return { id, error: describe(err) };
          }
        }),
      );
      if (cancelled) {
        return;
      }
      const nextStatuses: Record<string, Status> = {};
      const nextErrors: Record<string, string> = {};
      for (const result of results) {
        if (result.status) {
          nextStatuses[result.id] = result.status;
        } else {
          nextErrors[result.id] = result.error ?? "unavailable";
        }
      }
      setStatuses(nextStatuses);
      setStatusErrors(nextErrors);
    }

    void poll();
    const timer = setInterval(() => void poll(), 1000);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [cluster]);

  const ids = cluster ? [cluster.self, ...cluster.peers] : [];

  return (
    <main className="app">
      <header className="app__header">
        <h1>distributed key-value store</h1>
        {cluster ? (
          <p className="muted">
            local node <strong>{cluster.self}</strong>
          </p>
        ) : (
          <p className="muted">connecting…</p>
        )}
      </header>

      {clusterError && <p className="error">cluster: {clusterError}</p>}

      <nav className="tabs">
        <button
          type="button"
          className={tab === "cluster" ? "tab tab--active" : "tab"}
          onClick={() => setTab("cluster")}
        >
          Cluster
        </button>
        <button
          type="button"
          className={tab === "api" ? "tab tab--active" : "tab"}
          onClick={() => setTab("api")}
        >
          API
        </button>
      </nav>

      {tab === "cluster" ? (
        <>
          <div className="nodes">
            {ids.map((id) => (
              <NodeCard key={id} id={id} status={statuses[id]} error={statusErrors[id]} />
            ))}
          </div>

          <KVForm />
        </>
      ) : (
        <Playground />
      )}
    </main>
  );
}

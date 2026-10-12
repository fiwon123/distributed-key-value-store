import { useEffect, useState } from "react";
import { fetchCluster, fetchStatus } from "./api";
import { KVForm } from "./components/KVForm";
import { Keys } from "./components/Keys";
import { NodeCard } from "./components/NodeCard";
import { Playground } from "./components/Playground";
import { About } from "./components/About";
import { useTheme } from "./hooks/useTheme";
import type { ClusterInfo, Status } from "./types";
import "./styles.css";

type Tab = "cluster" | "api" | "about";

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
  const { theme, toggleTheme } = useTheme();

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
  }, []);

  useEffect(() => {
    if (!cluster) {
      return;
    }
    let cancelled = false;

    async function poll() {
      if (!cluster) return;
      const self = cluster.self;
      const ids = [self, ...cluster.peers];
      const results = await Promise.all(
        ids.map(async (id): Promise<Polled> => {
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
      <header className="header">
        <div className="header-content">
          <div className="logo">
            <img src="/favicon.svg" alt="Logo" />
            <span>distributed key-value store</span>
            {cluster && (
              <span className="muted">
                {" "}
                (local node <strong>{cluster.self}</strong>)
              </span>
            )}
          </div>
          <button
            className="theme-toggle"
            onClick={toggleTheme}
            aria-label="Toggle theme"
            title="Toggle theme"
          >
            {theme === "dark" ? "☀️" : "🌙"}
          </button>
        </div>
      </header>

      {clusterError && <p className="error">cluster: {clusterError}</p>}

      <div className="main">
        <nav className="nav tabs">
          <button
            type="button"
            className={`nav-tab tab ${tab === "cluster" ? "active tab--active" : ""}`}
            onClick={() => setTab("cluster")}
            data-testid="cluster-tab"
          >
            Cluster
          </button>
          <button
            type="button"
            className={`nav-tab tab ${tab === "api" ? "active tab--active" : ""}`}
            onClick={() => setTab("api")}
            data-testid="api-tab"
          >
            API
          </button>
          <button
            type="button"
            className={`nav-tab tab ${tab === "about" ? "active tab--active" : ""}`}
            onClick={() => setTab("about")}
            data-testid="about-tab"
          >
            About
          </button>
        </nav>

        <div className="content">
          {tab === "cluster" ? (
            <>
              <div className="nodes">
                {ids.map((id) => (
                  <NodeCard key={id} id={id} status={statuses[id]} error={statusErrors[id]} />
                ))}
              </div>

              <KVForm />
              <Keys />
            </>
          ) : tab === "api" ? (
            <Playground />
          ) : (
            <About />
          )}
        </div>
      </div>

      <footer className="footer">
        <div className="footer-content">
          <div className="footer-links">
            <a
              href="https://github.com/fiwon123/distributed-key-value-store"
              target="_blank"
              rel="noreferrer"
              className="footer-link"
            >
              GitHub
            </a>
            <a href="/docs" target="_blank" rel="noreferrer" className="footer-link">
              API Docs
            </a>
          </div>
          <p className="footer-text">© 2026 Distributed Key-Value Store</p>
        </div>
      </footer>
    </main>
  );
}

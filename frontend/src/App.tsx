import { useEffect, useState } from "react";
import { fetchCluster, fetchStatus } from "./api";
import { KVForm } from "./components/KVForm";
import { Keys } from "./components/Keys";
import { NodeCard } from "./components/NodeCard";
import { Playground } from "./components/Playground";
import { Guide } from "./components/Guide";
import { Learn } from "./components/Learn";
import { useKVConsole } from "./hooks/useKVConsole";
import { useTheme } from "./hooks/useTheme";
import type { ClusterInfo, Status } from "./types";
import "./styles.css";

type Tab = "cluster" | "api" | "guide" | "learn";

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

// Sun / moon glyphs for the theme toggle, drawn inline so there is no icon
// dependency and rendering is consistent across platforms.
function ThemeIcon({ theme }: { theme: "light" | "dark" }) {
  return theme === "dark" ? (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" />
    </svg>
  ) : (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
    </svg>
  );
}

export default function App() {
  const [tab, setTab] = useState<Tab>("cluster");
  const [cluster, setCluster] = useState<ClusterInfo>();
  const [clusterError, setClusterError] = useState("");
  const [statuses, setStatuses] = useState<Record<string, Status>>({});
  const [statusErrors, setStatusErrors] = useState<Record<string, string>>({});
  const { theme, toggleTheme } = useTheme();
  const kv = useKVConsole();

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
  const nextTheme = theme === "dark" ? "light" : "dark";

  return (
    <div className="app">
      <header className="header">
        <div className="header-content">
          <div className="logo">
            <img src="/favicon.svg" alt="" width="28" height="28" />
            <span className="logo__title">Distributed Key-Value Store</span>
            {cluster && (
              <span className="logo__node muted">
                local node <strong>{cluster.self}</strong>
              </span>
            )}
          </div>
          <button
            type="button"
            className="theme-toggle"
            onClick={toggleTheme}
            aria-label={`Switch to ${nextTheme} mode`}
            title={`Switch to ${nextTheme} mode`}
          >
            <ThemeIcon theme={theme} />
          </button>
        </div>
      </header>

      <nav className="tabs" aria-label="Dashboard sections">
        <button
          type="button"
          className={tab === "cluster" ? "tab tab--active" : "tab"}
          aria-pressed={tab === "cluster"}
          onClick={() => setTab("cluster")}
        >
          Cluster
        </button>
        <button
          type="button"
          className={tab === "api" ? "tab tab--active" : "tab"}
          aria-pressed={tab === "api"}
          onClick={() => setTab("api")}
        >
          API
        </button>
        <button
          type="button"
          className={tab === "guide" ? "tab tab--active" : "tab"}
          aria-pressed={tab === "guide"}
          onClick={() => setTab("guide")}
        >
          Guide
        </button>
        <button
          type="button"
          className={tab === "learn" ? "tab tab--active" : "tab"}
          aria-pressed={tab === "learn"}
          onClick={() => setTab("learn")}
        >
          Learn
        </button>
      </nav>

      <main className="content">
        {clusterError && <p className="error">cluster: {clusterError}</p>}

        {tab === "cluster" ? (
          <>
            <p className="cluster__lead">
              A replicated key-value store: writes go through the <strong>leader</strong>, commit to
              a majority, and apply on every node — so committed data survives a node failure.{" "}
              <button type="button" className="link" onClick={() => setTab("guide")}>
                How it works
              </button>
            </p>

            {cluster && cluster.peers.length === 0 && (
              <details className="callout">
                <summary>
                  <h3>How to run more nodes?</h3>
                </summary>
                <div className="callout__body" role="note">
                  <p>
                    This node was started without <code>-peers</code>, so it is a cluster of one and
                    elected itself leader. Start more nodes and this tab shows them all:
                  </p>
                  <pre>
                    <code>make cluster-local-up</code>
                  </pre>
                  <p className="muted">
                    For a different size, add <code>NODE_COUNT=5</code>. Then open{" "}
                    <code>http://127.0.0.1:9081</code> — one leader and the rest followers.
                  </p>
                </div>
              </details>
            )}

            <div className="nodes">
              {ids.map((id) => (
                <NodeCard key={id} id={id} status={statuses[id]} error={statusErrors[id]} />
              ))}
            </div>

            <KVForm state={kv} />
            <Keys onSelect={kv.select} busy={kv.busy} />
          </>
        ) : tab === "api" ? (
          <Playground />
        ) : tab === "guide" ? (
          <Guide />
        ) : (
          <Learn />
        )}
      </main>

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
    </div>
  );
}

import { useEffect, useState } from "react";
import { listKeys } from "../api";

// Keys lists the keys currently in the replicated store, refreshing
// periodically so it reflects puts and deletes made elsewhere. Clicking a key
// loads it into the console (GET), so it can be read, edited, or deleted.
export function Keys({
  onSelect,
  busy = false,
}: {
  onSelect: (key: string) => void;
  busy?: boolean;
}) {
  const [keys, setKeys] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const next = await listKeys();
        if (!cancelled) {
          setKeys(next);
          setError("");
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err));
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void load();
    const timer = setInterval(() => void load(), 2000);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, []);

  return (
    <section className="keys">
      <h2>stored keys</h2>
      {error ? (
        <p className="error">{error}</p>
      ) : loading ? (
        <p className="muted">loading…</p>
      ) : keys.length === 0 ? (
        <p className="muted">no keys</p>
      ) : (
        <ul className="key-list">
          {keys.map((key) => (
            <li key={key}>
              <button
                type="button"
                className="key-list__item"
                onClick={() => onSelect(key)}
                disabled={busy}
                title={`Load ${key}`}
              >
                {key}
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

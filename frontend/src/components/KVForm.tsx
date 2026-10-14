import type { KVConsole } from "../hooks/useKVConsole";

// KVForm is presentational: all state and operations live in useKVConsole, so
// the stored-keys list can drive the same console (see App.tsx).
export function KVForm({ state }: { state: KVConsole }) {
  const { key, value, busy, outcome, setKey, setValue, put, get, remove } = state;

  return (
    <section className="kv">
      <h2>keys</h2>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          put();
        }}
      >
        <label>
          key
          <input
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder="greeting"
            required
          />
        </label>
        <label>
          value
          <input
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder="hello"
            required
          />
        </label>
        <div className="actions">
          <button type="submit" disabled={busy || key === "" || value === ""}>
            PUT
          </button>
          <button type="button" disabled={busy} onClick={get}>
            GET
          </button>
          <button type="button" disabled={busy} onClick={remove}>
            DELETE
          </button>
        </div>
      </form>
      {outcome && (
        <p className={outcome.tone === "ok" ? "ok" : "error"} aria-live="polite" role="status">
          {outcome.text}
        </p>
      )}
    </section>
  );
}

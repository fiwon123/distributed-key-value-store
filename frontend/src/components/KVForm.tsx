import { useState } from "react";
import { deleteKey, getKey, putKey } from "../api";

type Outcome = { tone: "ok" | "err"; text: string } | undefined;

export function KVForm() {
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<Outcome>();

  async function run(op: () => Promise<string>) {
    setBusy(true);
    setOutcome(undefined);
    try {
      setOutcome({ tone: "ok", text: await op() });
    } catch (err) {
      setOutcome({ tone: "err", text: err instanceof Error ? err.message : String(err) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="kv">
      <h2>keys</h2>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void run(async () => {
            await putKey(key, value);
            return `PUT ${key}`;
          });
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
          <input value={value} onChange={(e) => setValue(e.target.value)} placeholder="hello" />
        </label>
        <div className="actions">
          <button type="submit" disabled={busy}>
            PUT
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => void run(async () => await getKey(key))}
          >
            GET
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() =>
              void run(async () => {
                await deleteKey(key);
                return `DELETE ${key}`;
              })
            }
          >
            DELETE
          </button>
        </div>
      </form>
      {outcome && <p className={outcome.tone === "ok" ? "ok" : "error"}>{outcome.text}</p>}
    </section>
  );
}

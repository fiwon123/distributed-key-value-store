import { useCallback, useRef, useState } from "react";
import { deleteKey, getKey, putKey } from "../api";

export type Outcome = { tone: "ok" | "err"; text: string } | undefined;

// KVConsole is the state and operations shared by the key/value form and the
// stored-keys list. It lives in a hook so clicking a key in the list can drive
// the same GET (and fill the same inputs) the form's GET button uses.
export interface KVConsole {
  key: string;
  value: string;
  busy: boolean;
  outcome: Outcome;
  setKey: (value: string) => void;
  setValue: (value: string) => void;
  put: () => void;
  get: () => void;
  remove: () => void;
  select: (key: string) => void;
}

export function useKVConsole(): KVConsole {
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<Outcome>();

  // busyRef guards against overlapping operations without making every
  // operation depend on `busy` (which would recreate the callbacks each render).
  const busyRef = useRef(false);
  const run = useCallback(async (op: () => Promise<string>) => {
    if (busyRef.current) {
      return;
    }
    busyRef.current = true;
    setBusy(true);
    setOutcome(undefined);
    try {
      setOutcome({ tone: "ok", text: await op() });
    } catch (err) {
      setOutcome({ tone: "err", text: err instanceof Error ? err.message : String(err) });
    } finally {
      busyRef.current = false;
      setBusy(false);
    }
  }, []);

  // put always requires a value: a PUT with an empty value is almost never what
  // the user means, and clearing the form afterwards avoids re-writing the same
  // pair by accident.
  const put = useCallback(() => {
    if (key === "" || value === "") {
      setOutcome({ tone: "err", text: "enter a key and a value" });
      return;
    }
    void run(async () => {
      await putKey(key, value);
      const message = `PUT ${key} = ${value}`;
      setKey("");
      setValue("");
      return message;
    });
  }, [key, value, run]);

  // runGet loads a key's value into the form so it can be edited and re-PUT.
  const runGet = useCallback(
    (target: string) => {
      if (target === "") {
        setOutcome({ tone: "err", text: "enter a key" });
        return;
      }
      void run(async () => {
        const loaded = await getKey(target);
        setKey(target);
        setValue(loaded);
        return `GET ${target}: ${loaded}`;
      });
    },
    [run],
  );

  const get = useCallback(() => runGet(key), [runGet, key]);

  const remove = useCallback(() => {
    if (key === "") {
      setOutcome({ tone: "err", text: "enter a key" });
      return;
    }
    void run(async () => {
      await deleteKey(key);
      const message = `DELETE ${key}`;
      setKey("");
      setValue("");
      return message;
    });
  }, [key, run]);

  // select is what the stored-keys list calls: GET the clicked key.
  const select = useCallback((target: string) => runGet(target), [runGet]);

  return { key, value, busy, outcome, setKey, setValue, put, get, remove, select };
}

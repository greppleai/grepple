import { useMemo, useState } from "react";

type SearchPanelProps<T> = {
  entries: readonly T[];
  identify(entry: T): string;
  render(entry: T): React.ReactNode;
};

/** ADVANCED_DOC: maintains filtering state while preserving generic entry types. */
export function SearchPanel<T>({ entries, identify, render }: SearchPanelProps<T>) {
  const [query, setQuery] = useState("");
  const normalized = query.trim().toLocaleLowerCase();

  // Compute visible entries only when the query changes.
  const visibleEntries = useMemo(
    () => entries.filter((entry) => identify(entry).toLocaleLowerCase().includes(normalized)),
    [entries, identify, normalized],
  );

  return (
    <section aria-label="Search results">
      <input value={query} onChange={(event) => setQuery(event.currentTarget.value)} />
      <ul>
        {visibleEntries.map((entry) => (
          <li key={identify(entry)}>{render(entry)}</li>
        ))}
      </ul>
      {/* ADVANCED_END */}
    </section>
  );
}

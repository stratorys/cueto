// Saved lenses: named queries persisted client-side. Lenses are view state, not
// model state, so they live in localStorage - never in the content-addressed
// version files (immutable, hashed) or data.cue (comments are dropped on the
// eval round-trip). A server-side sidecar is a later upgrade path.

export interface Lens {
  id: string;
  name: string;
  query: string;
}

const STORAGE_KEY = "cueto.lenses";

export function loadLenses(): Lens[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(
      (l): l is Lens =>
        l && typeof l.id === "string" && typeof l.name === "string" && typeof l.query === "string",
    );
  } catch {
    return [];
  }
}

function persist(lenses: Lens[]): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(lenses));
  } catch {
    // Storage unavailable or full: lenses are non-critical, so fail silently.
  }
}

// Save a new lens (or overwrite one with the same id) and return the full list.
export function saveLens(lens: Lens): Lens[] {
  const lenses = loadLenses().filter((l) => l.id !== lens.id);
  lenses.push(lens);
  persist(lenses);
  return lenses;
}

export function deleteLens(id: string): Lens[] {
  const lenses = loadLenses().filter((l) => l.id !== id);
  persist(lenses);
  return lenses;
}

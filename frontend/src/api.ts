// HTTP client for the Go CUE backend.
// Base URL is configurable; defaults to the local backend on :8091.

const BASE = import.meta.env.VITE_API_URL ?? "http://localhost:8091";

export interface EvalOk {
  ok: true;
  diagram: unknown;
}

export interface EvalErr {
  ok: false;
  error: string;
}

// evalCue evaluates data.cue against schema.cue and returns the diagram JSON,
// or a validation error. Network failures surface as an error result too.
export async function evalCue(data: string): Promise<EvalOk | EvalErr> {
  let response: Response;
  try {
    response = await fetch(`${BASE}/eval`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ data }),
    });
  } catch (error) {
    return { ok: false, error: `Cannot reach backend: ${String(error)}` };
  }

  if (response.ok) {
    return { ok: true, diagram: await response.json() };
  }
  const body = (await response.json().catch(() => ({}))) as { error?: string };
  return { ok: false, error: body.error ?? `HTTP ${response.status}` };
}

export async function formatCue(source: string): Promise<string> {
  const response = await fetch(`${BASE}/format`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ source }),
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? `HTTP ${response.status}`);
  }
  const body = (await response.json()) as { formatted: string };
  return body.formatted;
}

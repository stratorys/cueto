// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// HTTP client for the Go CUE backend.
// Base URL is configurable; defaults to the local backend on :8091.

import type { Diagram, DiagramEdge, DiagramNode, EditorFile, Provenance } from "./model";

const BASE = import.meta.env.VITE_API_URL ?? "http://localhost:8091";

// One structured error from the backend. line/column are 1-based positions in
// the data.cue text (0 when the error carries no position); kind is one of
// "parse" | "schema" | "incomplete" | "internal".
export interface Diagnostic {
  message: string;
  line: number;
  column: number;
  kind: string;
  // Present on policy/drift findings: the rule name and the graph element the
  // finding anchors to (for click-to-highlight).
  rule?: string;
  nodeId?: string;
  edgeId?: string;
}

// One inlay hint from /eval: ghost text at a 1-based data.cue position. "type"
// annotates a written field with its schema constraint; "optional" lists a
// struct's declared-but-unset optional fields.
export interface Hint {
  line: number;
  column: number;
  label: string;
  kind: string;
}

export interface EvalOk {
  ok: true;
  diagram: unknown;
  hints: Hint[];
}

// Multi-file eval also reports provenance: which file authored each element.
export interface EvalFilesOk {
  ok: true;
  diagram: unknown;
  hints: Hint[];
  provenance: Provenance;
}

// Result of /rewrite: the file's new text after splicing canvas edits.
export interface RewriteOk {
  ok: true;
  content: string;
}

// Result of /repl: the concrete value of a standalone CUE snippet, as JSON.
export interface ReplOk {
  ok: true;
  result: unknown;
}

// One entry a REPL query can reference: a builtin, or a member of an imported
// package. isFunc marks callables (strings.ToUpper) apart from value constants
// (math.Pi).
export interface CueMember {
  name: string;
  isFunc: boolean;
}

// One importable standard-library package. path is the import path
// (encoding/json); name is the identifier it binds to by default (json).
export interface CuePackage {
  path: string;
  name: string;
  members: CueMember[];
}

// The static CUE reference backing the REPL's autocomplete and browser: builtin
// functions and every importable package with its members.
export interface CueMeta {
  builtins: CueMember[];
  packages: CuePackage[];
}

export interface EvalErr {
  ok: false;
  error: string;
  diagnostics: Diagnostic[];
}

export interface SaveOk {
  ok: true;
  version: string;
}

// Result of /vet: ok:true when the diagram passes schema + any opted-in policies
// (and drift, when facts are supplied); otherwise diagnostics carry the findings.
export interface VetOk {
  ok: true;
  passes: boolean;
  diagnostics: Diagnostic[];
}

export interface ImportOk {
  ok: true;
  facts: string;
}

export interface FormatOk {
  ok: true;
  formatted: string;
}

// One saved version: its content-hash id and when it was first saved (ISO 8601).
export interface VersionMeta {
  version: string;
  savedAt: string;
}

export interface VersionsOk {
  ok: true;
  versions: VersionMeta[];
}

export interface VersionDataOk {
  ok: true;
  data: string;
}

// One project: its slug id, display name, and timestamps (ISO 8601).
export interface ProjectMeta {
  id: string;
  name: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface ProjectsOk {
  ok: true;
  projects: ProjectMeta[];
}

export interface ProjectOk {
  ok: true;
  project: ProjectMeta;
}

// A bare success with no payload (project delete).
export interface OkResult {
  ok: true;
}

// What a new project's canvas starts from: empty, or a copy of the seed sample.
export type ProjectSeed = "blank" | "sample";

// Render diagnostics into a single human-readable string, prefixing positions
// when present.
function summarize(diagnostics: Diagnostic[]): string {
  return diagnostics
    .map((d) => (d.line ? `${d.line}:${d.column} ${d.message}` : d.message))
    .join("\n");
}

// The one JSON trust boundary. Parse a response body into the expected shape,
// falling back to an empty object when the body is absent or malformed. The
// `as T` here is the single place an untyped payload is shaped; every endpoint
// reads its reply through this helper rather than casting inline.
async function readJson<T>(response: Response): Promise<T> {
  return (await response.json().catch(() => ({}))) as T;
}

// Build an EvalErr from an error response body. Falls back to the legacy `error`
// field, then to the HTTP status, so a transition or proxy error still surfaces.
function errorResult(
  body: { diagnostics?: Diagnostic[]; error?: string },
  status: number,
): EvalErr {
  const diagnostics = body.diagnostics ?? [];
  const error = diagnostics.length
    ? summarize(diagnostics)
    : (body.error ?? `HTTP ${status}`);
  return { ok: false, error, diagnostics };
}

// post sends a JSON body and shapes the reply into a discriminated result: the
// onOk parser builds the success payload, while a network failure or any error
// status both collapse into an EvalErr. Every endpoint routes through here so the
// transport rules (base URL, headers, error decoding) live in one place.
async function post<T>(
  path: string,
  body: object,
  onOk: (response: Response) => Promise<T>,
): Promise<({ ok: true } & T) | EvalErr> {
  let response: Response;
  try {
    response = await fetch(`${BASE}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  } catch (error) {
    return { ok: false, error: `Cannot reach backend: ${String(error)}`, diagnostics: [] };
  }
  if (response.ok) {
    return { ok: true, ...(await onOk(response)) };
  }
  const errorBody = await readJson<{ diagnostics?: Diagnostic[]; error?: string }>(response);
  return errorResult(errorBody, response.status);
}

// get is the read-only sibling of post: same error shaping, no request body.
// Used by the version-history endpoints.
async function get<T>(
  path: string,
  onOk: (response: Response) => Promise<T>,
): Promise<({ ok: true } & T) | EvalErr> {
  let response: Response;
  try {
    response = await fetch(`${BASE}${path}`);
  } catch (error) {
    return { ok: false, error: `Cannot reach backend: ${String(error)}`, diagnostics: [] };
  }
  if (response.ok) {
    return { ok: true, ...(await onOk(response)) };
  }
  const errorBody = await readJson<{ diagnostics?: Diagnostic[]; error?: string }>(response);
  return errorResult(errorBody, response.status);
}

// sendJSON is post generalized to any mutating method (PATCH/DELETE), with an
// optional body. Same transport + error shaping as post/get.
async function sendJSON<T>(
  method: string,
  path: string,
  body: object | undefined,
  onOk: (response: Response) => Promise<T>,
): Promise<({ ok: true } & T) | EvalErr> {
  let response: Response;
  try {
    response = await fetch(`${BASE}${path}`, {
      method,
      headers: body === undefined ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (error) {
    return { ok: false, error: `Cannot reach backend: ${String(error)}`, diagnostics: [] };
  }
  if (response.ok) {
    return { ok: true, ...(await onOk(response)) };
  }
  const errorBody = await readJson<{ diagnostics?: Diagnostic[]; error?: string }>(response);
  return errorResult(errorBody, response.status);
}

// evalCue evaluates data.cue against schema.cue and returns the diagram JSON,
// or structured diagnostics. Network failures surface as an error result too.
export function evalCue(data: string): Promise<EvalOk | EvalErr> {
  return post("/eval", { data }, async (response) => {
    const body = await readJson<{ diagram?: unknown; hints?: Hint[] }>(response);
    return { diagram: body.diagram, hints: body.hints ?? [] };
  });
}

// evalFiles evaluates the multi-file package (all files unify into one diagram)
// and returns the diagram JSON, inlay hints, and per-element provenance. The
// single-file evalCue above stays for callers that have not moved to the file set.
export function evalFiles(files: EditorFile[]): Promise<EvalFilesOk | EvalErr> {
  const body = { files: files.map((f) => ({ name: f.name, content: f.text })) };
  return post("/eval", body, async (response) => {
    const parsed = await readJson<{
      diagram?: unknown;
      hints?: Hint[];
      provenance?: Provenance;
    }>(response);
    return {
      diagram: parsed.diagram,
      hints: parsed.hints ?? [],
      provenance: parsed.provenance ?? { nodes: {}, edges: "" },
    };
  });
}

// evalExpr runs a REPL entry and returns its concrete value as JSON, or
// diagnostics on a compile/concreteness error. When files are given, source is
// evaluated as a single expression against those editor files overlaid on the
// schema, so it can reference the live `diagram` (e.g. `diagram.nodes.x.owner`);
// otherwise it is a standalone snippet. Nothing is persisted either way: the input
// never joins the editor files, the schema, or any saved version.
export function evalExpr(source: string, files?: EditorFile[]): Promise<ReplOk | EvalErr> {
  const body = files?.length
    ? { source, files: files.map((f) => ({ name: f.name, content: f.text })) }
    : { source };
  return post("/repl", body, async (response) => {
    const parsed = await readJson<{ result?: unknown }>(response);
    return { result: parsed.result ?? null };
  });
}

// fetchCueMeta returns the static CUE reference (builtins + importable packages
// with their members) for the REPL's autocomplete and reference browser. It is
// version-static, so callers fetch it once.
export function fetchCueMeta(): Promise<({ ok: true } & CueMeta) | EvalErr> {
  return get("/cue/meta", async (response) => readJson<CueMeta>(response));
}

// rewriteFile splices canvas edits into one editable file's source and returns
// the new text, preserving hand-written CUE and comments. `nodes` maps a node id
// to its CUE struct body (upserted); `deletes` lists ids to remove; `edges`, when
// present, is the CUE list text replacing the edge list.
export function rewriteFile(op: {
  name: string;
  content: string;
  nodes?: Record<string, string>;
  deletes?: string[];
  edges?: string;
}): Promise<RewriteOk | EvalErr> {
  return post("/rewrite", op, async (response) => {
    const body = await readJson<{ content?: string }>(response);
    return { content: body.content ?? "" };
  });
}

// saveCue validates data and, when valid, persists it as an immutable version of
// the given project, returning the version id. Invalid data comes back as
// diagnostics.
export function saveCue(projectId: string, data: string): Promise<SaveOk | EvalErr> {
  return post(`/projects/${projectId}/save`, { data }, async (response) => {
    const body = await readJson<{ version?: string }>(response);
    return { version: body.version ?? "" };
  });
}

// vetCue validates data against the schema and any opted-in policy packs. When
// `facts` (imported infra) is supplied, it also reports drift. A well-formed but
// non-passing diagram comes back as ok:true, passes:false with diagnostics; only
// a transport/operational failure is an EvalErr.
export function vetCue(data: string, facts?: string): Promise<VetOk | EvalErr> {
  const body = facts === undefined ? { data } : { data, facts };
  return post("/vet", body, async (response) => {
    const parsed = await readJson<{ ok?: boolean; diagnostics?: Diagnostic[] }>(response);
    return { passes: parsed.ok ?? false, diagnostics: parsed.diagnostics ?? [] };
  });
}

// vetFiles is the multi-file sibling of vetCue: it validates the whole package
// (all files unify) against the schema, policies, and optional drift facts.
export function vetFiles(files: EditorFile[], facts?: string): Promise<VetOk | EvalErr> {
  const mapped = files.map((f) => ({ name: f.name, content: f.text }));
  const body = facts === undefined ? { files: mapped } : { files: mapped, facts };
  return post("/vet", body, async (response) => {
    const parsed = await readJson<{ ok?: boolean; diagnostics?: Diagnostic[] }>(response);
    return { passes: parsed.ok ?? false, diagnostics: parsed.diagnostics ?? [] };
  });
}

// importCompose parses docker-compose YAML into normalized infra facts (CUE/JSON)
// to check the diagram against with vetCue(data, facts).
export function importCompose(source: string): Promise<ImportOk | EvalErr> {
  return post("/import/compose", { source }, async (response) => {
    const body = await readJson<{ facts?: string }>(response);
    return { facts: body.facts ?? "" };
  });
}

// formatCue runs `cue fmt` over the source and returns the formatted text.
// Formatting is semantics-preserving, so the caller need not re-evaluate.
export function formatCue(source: string): Promise<FormatOk | EvalErr> {
  return post("/format", { source }, async (response) => {
    const body = await readJson<{ formatted?: string }>(response);
    return { formatted: body.formatted ?? "" };
  });
}

// listVersions returns a project's saved versions newest-first for the history view.
export function listVersions(projectId: string): Promise<VersionsOk | EvalErr> {
  return get(`/projects/${projectId}/versions`, async (response) => {
    const body = await readJson<{ versions?: VersionMeta[] }>(response);
    return { versions: body.versions ?? [] };
  });
}

// readVersion returns one of a project's versions' stored data.cue text by its
// content hash.
export function readVersion(projectId: string, id: string): Promise<VersionDataOk | EvalErr> {
  return get(`/projects/${projectId}/versions/${id}`, async (response) => {
    const body = await readJson<{ data?: string }>(response);
    return { data: body.data ?? "" };
  });
}

// listProjects returns the registered projects (newest-updated first).
export function listProjects(): Promise<ProjectsOk | EvalErr> {
  return get("/projects", async (response) => {
    const body = await readJson<{ projects?: ProjectMeta[] }>(response);
    return { projects: body.projects ?? [] };
  });
}

// createProject registers a new project seeded "blank" or "sample".
export function createProject(name: string, seed: ProjectSeed): Promise<ProjectOk | EvalErr> {
  return post("/projects", { name, seed }, async (response) => {
    const body = await readJson<{ project?: ProjectMeta }>(response);
    return { project: body.project ?? { id: "", name } };
  });
}

// renameProject changes a project's display name.
export function renameProject(id: string, name: string): Promise<ProjectOk | EvalErr> {
  return sendJSON("PATCH", `/projects/${id}`, { name }, async (response) => {
    const body = await readJson<{ project?: ProjectMeta }>(response);
    return { project: body.project ?? { id, name } };
  });
}

// deleteProject removes a project and its version store.
export function deleteProject(id: string): Promise<OkResult | EvalErr> {
  return sendJSON("DELETE", `/projects/${id}`, undefined, async () => ({}));
}

// readSeed returns the on-disk seed data.cue text, the mount-time fallback when
// no saved version exists yet.
export function readSeed(): Promise<VersionDataOk | EvalErr> {
  return get("/seed", async (response) => {
    const body = await readJson<{ data?: string }>(response);
    return { data: body.data ?? "" };
  });
}

// Map the backend /eval JSON to the frontend Diagram model. The backend returns
// nodes as a struct keyed by id (matching the CUE schema); the frontend uses a
// node array. Edges already match one-to-one.
interface EvalDiagram {
  nodes?: Record<string, Omit<DiagramNode, "id"> & { id?: string }>;
  edges?: DiagramEdge[];
  policies?: string[];
}

export function fromEval(raw: unknown, provenance?: Provenance): Diagram {
  const source = (raw ?? {}) as EvalDiagram;
  const nodes: DiagramNode[] = Object.entries(source.nodes ?? {}).map(
    ([id, node]) => {
      const owner = provenance?.nodes[id];
      return owner ? { ...node, id, sourceFile: owner } : { ...node, id };
    },
  );
  const edgeOwner = provenance?.edges;
  const edges: DiagramEdge[] = (source.edges ?? []).map((edge) =>
    edgeOwner ? { ...edge, sourceFile: edgeOwner } : edge,
  );
  return source.policies?.length
    ? { nodes, edges, policies: source.policies }
    : { nodes, edges };
}

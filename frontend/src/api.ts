// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// HTTP client for the Go CUE backend.
// Base URL is configurable; defaults to the local backend on :8091.

import type { Diagram, DiagramEdge, DiagramNode, EditorFile, Provenance } from "./model";

const BASE = import.meta.env.VITE_API_URL ?? "http://localhost:8091";

// The current project id, woven into every project-scoped request path. Every
// module-touching endpoint (eval, vet, repl, save, history, file, tree) is served
// under /projects/:id, so the id is set once at boot and on each project switch
// via setProject, module-level like BASE. Module-independent endpoints
// (/cue/meta, /format, /rewrite, /projects) do not use it.
let projectId = "";

export function setProject(id: string): void {
  projectId = id;
}

// Whether a project is currently selected. Callers guard project-scoped work
// (eval, save) on this so nothing hits /projects//… before a project is open.
export function hasProject(): boolean {
  return projectId !== "";
}

function proj(): string {
  return `/projects/${encodeURIComponent(projectId)}`;
}

// One structured error from the backend. line/column are 1-based positions in
// the data.cue text (0 when the error carries no position); kind is one of
// "parse" | "schema" | "incomplete" | "internal".
export interface Diagnostic {
  message: string;
  line: number;
  column: number;
  kind: string;
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

// One trace entry from /eval: the machine-readable reason an inferred element
// exists. Present only when the rendered view was derived (registries + key-set
// references), not authored; a declared view carries no trace. It backs the legend
// and the "why is this here" inspector.
export interface TraceEntry {
  // The node id or edge id the entry explains (matches the diagram element id).
  element: string;
  kind: "node" | "edge";
  // Which detector produced it: a registry node, a key-set reference edge, or an
  // explicit @ref attribute edge.
  rule: "registry" | "key-set-ref" | "attr-ref";
  // The registry field (node), or "source.field -> target" (edge).
  detail: string;
}

// One legend row from /eval: a discovered registry, the node kind it renders as in
// the current view, and how many nodes it contributes. Mirrors the backend
// evaluation.LegendEntry. Present only when the rendered view was derived; a declared
// view carries no legend. Backs the canvas legend overlay.
export interface LegendEntry {
  // The registry field name (the domain node kind).
  field: string;
  // How the registry draws in the current view: one table (model view) or one node
  // per member (instance view).
  kind: "table" | "entity";
  // Node count the registry contributes to this view.
  count: number;
}

export interface EvalOk {
  ok: true;
  diagram: unknown;
  hints: Hint[];
  // Names of every discovered diagram view; empty for a knowledge-only module.
  views: string[];
  // Per-element inference trace; empty unless the rendered view was derived.
  trace: TraceEntry[];
}

// Multi-file eval also reports provenance: which file authored each element.
export interface EvalFilesOk {
  ok: true;
  diagram: unknown;
  hints: Hint[];
  provenance: Provenance;
  // Names of every discovered diagram view; empty for a knowledge-only module.
  views: string[];
  // Per-element inference trace; empty unless the rendered view was derived.
  trace: TraceEntry[];
  // Registry legend for the rendered view; empty unless the view was derived.
  legend: LegendEntry[];
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

export interface FormatOk {
  ok: true;
  formatted: string;
}

// One point in a file's git history: the commit hash, its subject line, and when
// it was authored (ISO 8601). The workspace-mode analogue of VersionMeta.
export interface CommitMeta {
  version: string;
  label: string;
  at: string;
}

export interface HistoryOk {
  ok: true;
  entries: CommitMeta[];
}

// A workspace file read: its text plus the content token used for optimistic
// concurrency on the next save.
export interface WorkspaceFileOk {
  ok: true;
  data: string;
  version: string;
}

// One project: its slug id and display name.
export interface ProjectMeta {
  id: string;
  name: string;
}

export interface ProjectsOk {
  ok: true;
  projects: ProjectMeta[];
}

export interface ProjectOk {
  ok: true;
  project: ProjectMeta;
}

// A bare success with no payload (file delete).
export interface OkResult {
  ok: true;
}

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

// Build an EvalErr from an error response body. Falls back to a plain `error`
// field, then to the HTTP status, so a transition or proxy error still surfaces.
function errorResult(
  body: { diagnostics?: Diagnostic[]; error?: string },
  status: number,
): EvalErr {
  const diagnostics = body.diagnostics ?? [];
  const error = diagnostics.length ? summarize(diagnostics) : (body.error ?? `HTTP ${status}`);
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

// evalCue evaluates the project's data against the diagram schema and returns the diagram JSON,
// or structured diagnostics. Network failures surface as an error result too.
export function evalCue(data: string): Promise<EvalOk | EvalErr> {
  return post(proj() + "/eval", { data }, async (response) => {
    const body = await readJson<{
      diagram?: unknown;
      hints?: Hint[];
      views?: string[];
      trace?: TraceEntry[];
    }>(response);
    return {
      diagram: body.diagram,
      hints: body.hints ?? [],
      views: body.views ?? [],
      trace: body.trace ?? [],
    };
  });
}

// evalFiles evaluates the multi-file package (all files unify into one diagram)
// and returns the diagram JSON, inlay hints, per-element provenance, and the names
// of every discovered view. view names which of those the backend renders; an
// empty or unknown view renders the default. The single-file evalCue above stays
// for callers that have not moved to the file set.
export function evalFiles(files: EditorFile[], view = ""): Promise<EvalFilesOk | EvalErr> {
  const body = { files: files.map((f) => ({ name: f.name, content: f.text })), view };
  return post(proj() + "/eval", body, async (response) => {
    const parsed = await readJson<{
      diagram?: unknown;
      hints?: Hint[];
      provenance?: Provenance;
      views?: string[];
      trace?: TraceEntry[];
      legend?: LegendEntry[];
    }>(response);
    return {
      diagram: parsed.diagram,
      hints: parsed.hints ?? [],
      provenance: parsed.provenance ?? { nodes: {}, edges: "" },
      views: parsed.views ?? [],
      trace: parsed.trace ?? [],
      legend: parsed.legend ?? [],
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
  return post(proj() + "/repl", body, async (response) => {
    const parsed = await readJson<{ result?: unknown }>(response);
    return { result: parsed.result ?? null };
  });
}

// fetchReplKeys returns the dotted identifier field paths of every top-level data
// field in the editor files (people, people.george, diagram, diagram.nodes, ...),
// computed on the backend from the parsed CUE value. It feeds the REPL's
// autocomplete over the whole data, not just the diagram. An invalid/incomplete
// diagram comes back as an EvalErr, so callers keep their last good key set.
export function fetchReplKeys(
  files: EditorFile[],
): Promise<{ ok: true; keys: string[] } | EvalErr> {
  const body = { files: files.map((f) => ({ name: f.name, content: f.text })) };
  return post(proj() + "/repl/keys", body, async (response) => {
    const parsed = await readJson<{ keys?: string[] }>(response);
    return { keys: parsed.keys ?? [] };
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

// formatCue runs `cue fmt` over the source and returns the formatted text.
// Formatting is semantics-preserving, so the caller need not re-evaluate.
export function formatCue(source: string): Promise<FormatOk | EvalErr> {
  return post("/format", { source }, async (response) => {
    const body = await readJson<{ formatted?: string }>(response);
    return { formatted: body.formatted ?? "" };
  });
}

// listProjects returns the projects under the root, each a git repo plus a CUE
// module, sorted by id. The picker's data source.
export function listProjects(): Promise<ProjectsOk | EvalErr> {
  return get("/projects", async (response) => {
    const body = await readJson<{ projects?: ProjectMeta[] }>(response);
    return { projects: body.projects ?? [] };
  });
}

// createProject git-initializes a new project directory under the root, scaffolds a
// minimal module, and makes one initial commit, returning its id. A name that
// collides with an existing project comes back as an error result (HTTP 409).
export function createProject(name: string): Promise<ProjectOk | EvalErr> {
  return post("/projects", { name }, async (response) => {
    const body = await readJson<{ project?: ProjectMeta }>(response);
    return { project: body.project ?? { id: "", name } };
  });
}

// getTree returns the current project's .cue files as workspace-relative slash
// paths (subdirectories included), for the file tree.
export function getTree(): Promise<{ ok: true; files: string[] } | EvalErr> {
  return get(proj() + "/tree", async (response) => {
    const body = await readJson<{ files?: string[] }>(response);
    return { files: body.files ?? [] };
  });
}

// saveWorkspaceFile writes a validated buffer to the real file at path in the
// current project, returning the new content token. baseVersion is the token the
// client loaded; a concurrent on-disk change comes back as an error result (HTTP
// 409) with a "changed on disk" diagnostic, and nothing is written.
export function saveWorkspaceFile(
  path: string,
  data: string,
  baseVersion: string,
): Promise<SaveOk | EvalErr> {
  return post(proj() + "/save", { path, data, baseVersion }, async (response) => {
    const body = await readJson<{ version?: string }>(response);
    return { version: body.version ?? "" };
  });
}

// deleteWorkspaceFile removes path from the current project's working tree. The
// removal shows in git status for the user to commit; cueto never commits it.
export function deleteWorkspaceFile(path: string): Promise<OkResult | EvalErr> {
  return sendJSON(
    "DELETE",
    `${proj()}/file?path=${encodeURIComponent(path)}`,
    undefined,
    async () => ({}),
  );
}

// listWorkspaceHistory returns the git commits that touched path, newest first. A
// project with no commits touching it comes back with an empty list.
export function listWorkspaceHistory(path: string): Promise<HistoryOk | EvalErr> {
  return get(`${proj()}/history?path=${encodeURIComponent(path)}`, async (response) => {
    const body = await readJson<{ entries?: CommitMeta[] }>(response);
    return { entries: body.entries ?? [] };
  });
}

// readWorkspaceFile returns the content of path plus its content token. With no
// commit it reads the current working-tree file (and the token is the save base);
// with a commit (a full git hash) it reads the blob at that commit.
export function readWorkspaceFile(path: string, commit = ""): Promise<WorkspaceFileOk | EvalErr> {
  const query = commit
    ? `?path=${encodeURIComponent(path)}&commit=${encodeURIComponent(commit)}`
    : `?path=${encodeURIComponent(path)}`;
  return get(`${proj()}/file${query}`, async (response) => {
    const body = await readJson<{ data?: string; version?: string }>(response);
    return { data: body.data ?? "", version: body.version ?? "" };
  });
}

// Map the backend /eval JSON to the frontend Diagram model. The backend returns
// nodes as a struct keyed by id (matching the CUE schema); the frontend uses a
// node array. Edges already match one-to-one.
interface EvalDiagram {
  nodes?: Record<string, Omit<DiagramNode, "id"> & { id?: string }>;
  edges?: DiagramEdge[];
}

export function fromEval(raw: unknown, provenance?: Provenance): Diagram {
  const source = (raw ?? {}) as EvalDiagram;
  const nodes: DiagramNode[] = Object.entries(source.nodes ?? {}).map(([id, node]) => {
    const owner = provenance?.nodes[id];
    return owner ? { ...node, id, sourceFile: owner } : { ...node, id };
  });
  const edgeOwner = provenance?.edges;
  const edges: DiagramEdge[] = (source.edges ?? []).map((edge) =>
    edgeOwner ? { ...edge, sourceFile: edgeOwner } : edge,
  );
  return { nodes, edges };
}

# cueto

A visual editor and evaluation server for architecture diagrams whose single source of truth is CUE, so a diagram, its schema, and its governance policies converge into one unified, machine-checkable value.

## Why this exists

Architecture diagrams drift from the systems they describe and carry no enforceable rules: they are pictures, not data. cueto explores the opposite premise - a diagram *is* data under a CUE schema. The shape, the constraints, and the policies are the same value, so "is this diagram valid?" and "does it satisfy our governance rules?" become decidable by unification instead of review by eye.

## What it demonstrates

- **Architecture pattern** - a hand-owned schema (`schema.cue`) that is never machine-written, with a concrete instance (`data.cue`) overlaid per request; the canvas only ever round-trips the data, the schema stays authoritative.
- **Workflow design** - the same model is edited two ways (visual canvas and CUE code) kept in sync through a source map, then evaluated, validated, formatted, and saved as immutable versions.
- **Knowledge model** - the schema separates *rendering* (`type`, `shape`, colors) from *governance* metadata (`role`, `owner`, `region`, `zone`), so rules like "no service crosses the PCI boundary" are expressible against the same nodes you draw.
- **Evaluation** - governance ships as importable policy packs that emit violation lists; a CI gate unifies each count with `0`, turning any violation into a nonzero `cue vet`.
- **Observability** - evaluation returns structured diagnostics with source positions and host paths scrubbed, plus provenance and hints, rather than opaque errors.
- **Production trade-offs** - untrusted CUE is evaluated in-process under body-size, output-size, per-request deadline, and concurrency bounds, behind explicit server timeouts and graceful shutdown.

## What it is not

This is not a production framework.
This is not a complete product.
This is a reference implementation / design study.

## Architecture

```mermaid
flowchart LR
  subgraph fe["frontend/ (Vue + Vite)"]
    canvas["Canvas (Vue Flow)"]
    editor["CUE editor (CodeMirror)"]
    panels["Analysis / Query / Policy / History panels"]
  end

  subgraph be["backend/ (Go + gin)"]
    api["/eval /vet /save /format\n/rewrite /import/compose /versions"]
    eval["CUE evaluator (bounded, in-process)"]
    versions[("versions/ (immutable snapshots)")]
  end

  subgraph cue["cue/ (source of truth)"]
    schema["schema.cue (authoritative)"]
    data["data.cue (instance)"]
    policy["policy packs + CI gate"]
  end

  canvas <--> editor
  editor --> api
  panels --> api
  api --> eval
  eval --> schema
  eval --> data
  eval --> versions
  policy -. "make check (cue vet ./...)" .-> cue
```

## How it works

1. `cue/schema.cue` defines the diagram shape and its governance fields. It is hand-owned and never rewritten by the app.
2. `cue/data.cue` is the concrete instance. The canvas round-trips only this file; the schema stays fixed.
3. On `/eval`, the backend loads the schema fresh from disk, overlays the request's editable files, unifies them, and returns the concrete diagram as JSON - or structured diagnostics on failure - all under size, output, deadline, and concurrency bounds.
4. Canvas edits are spliced back into CUE text via `/rewrite`, and `/format` normalizes it with `cue fmt`, so the code and the picture never disagree.
5. `/vet` runs the policy harness: each pack a diagram opts into (via `policies: [...]`) produces a list of violations. The separate `citool` gate unifies each violation count with `0`, so `cue vet ./...` (and `make check`) fails CI on any breach.
6. `/import/compose` turns a `docker-compose.yml` into normalized facts; `/vet` then reports drift between the drawn topology and that live topology.
7. `/save` writes the validated instance as an immutable, content-addressed version; `/versions` lists and reads them.

## Knowledge as code

The REPL pane turns the diagram into a queryable value. Every entry evaluates a CUE expression against the live model in the editor - nothing is saved, the schema and files are untouched - so a structured question gets a deterministic answer by evaluation, not retrieval.

Author a domain as data under a schema, and derive the graph from it:

```cue
package diagram

#Person: {
	name:   string
	mother: string | *""
	father: string | *""
	role:   string | *""
	year:   int
}

people: [ID=string]: #Person
people: {
	george:   {name: "George McFly", role: "parent", year: 1938}
	lorraine: {name: "Lorraine Baines", role: "parent", year: 1938}
	marty:    {name: "Marty McFly", role: "traveler", mother: "lorraine", father: "george", year: 1968}
	doc:      {name: "Dr. Emmett Brown", role: "inventor", year: 1920}
}

diagram: #Diagram & {
	nodes: {
		for pid, p in people {
			(pid): {type: "entity", label: p.name, data: {role: p.role, year: p.year}}
		}
	}
	edges: [
		for pid, p in people if p.mother != "" {
			{id: "m_\(pid)", source: p.mother, target: pid, kind: "arrow", label: "mother"}
		},
		for pid, p in people if p.father != "" {
			{id: "f_\(pid)", source: p.father, target: pid, kind: "arrow", label: "father"}
		},
	]
}
```

Now "who is Marty's mother?" is a path lookup, not a guess. Type the expression in the REPL and evaluate it:

```
> people[people.marty.mother].name
"Lorraine Baines"
```

The answer comes from the compiled value: `marty.mother` is checked against the same schema that renders the graph, so a dangling name is a build error, not a hallucination. An agent wired to this endpoint answers from evaluated fact instead of retrieved text - the graph you draw and the knowledge you query are one CUE value. This is the [knowledge-as-code](https://stratorys.com/knowledge-as-code) bet applied to a single diagram.

![REPL querying the McFly family diagram](docs/repl-knowledge-as-code.png)

## Run locally

Prerequisites: Go 1.26+, the [`cue`](https://cuelang.org) CLI (for `make check`), Node + pnpm.

Backend:

```
cp backend/.env.example backend/.env
cd backend
go run .
```

Frontend (in a second shell):

```
cp frontend/.env.example frontend/.env
cd frontend
pnpm install
pnpm run dev
```

Run the architecture CI check:

```
make check
```

Tests:

```
cd backend
go test ./...
```

```
cd frontend
pnpm run test
```

## Related writing

- [Coming soon](https://stratorys.com)

## License

Mozilla Public License v2.0 (MPL v2.0). See [LICENSE](LICENSE). Copyright 2026, Lucas Jahier - Stratorys.

package main

import (
	"encoding/json"
	"sort"

	"github.com/goccy/go-yaml"
)

// Importers turn a live-infra source into normalized #Actual facts (as a CUE/JSON
// value) that the drift harness checks the diagram against. docker-compose is the
// first source: services + depends_on become services + links. Untrusted input is
// bounded by the same body cap as every other endpoint.

// actualFacts mirrors infra.#Actual. It is emitted as JSON, which is a subset of
// CUE, so the drift overlay can unify it directly with infra.#Actual.
type actualFacts struct {
	Source   string                   `json:"source"`
	Services map[string]actualService `json:"services"`
	Links    []actualLink             `json:"links"`
}

type actualService struct {
	Name  string `json:"name"`
	Image string `json:"image,omitempty"`
}

type actualLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// composeFile is the subset of a docker-compose file the importer reads.
type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Image     string      `yaml:"image"`
	DependsOn composeDeps `yaml:"depends_on"`
}

// composeDeps normalizes depends_on, which may be a short list form
// (["db", "cache"]) or a long map form ({db: {condition: ...}}), to service names.
type composeDeps []string

func (d *composeDeps) UnmarshalYAML(b []byte) error {
	var list []string
	if err := yaml.Unmarshal(b, &list); err == nil {
		*d = list
		return nil
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err == nil {
		names := make([]string, 0, len(m))
		for name := range m {
			names = append(names, name)
		}
		sort.Strings(names)
		*d = names
		return nil
	}
	// An absent or unrecognized depends_on contributes no links, not an error.
	return nil
}

// importCompose parses docker-compose YAML into #Actual facts serialized as JSON.
// A parse failure comes back as a kindImport diagnostic (host paths scrubbed),
// never a host-path leak.
func (e *cueEvaluator) importCompose(source string) (string, []Diagnostic, error) {
	var file composeFile
	if err := yaml.Unmarshal([]byte(source), &file); err != nil {
		return "", []Diagnostic{{Message: scrub(err.Error(), e.cueDir), Kind: kindImport}}, nil
	}

	facts := actualFacts{
		Source:   "compose",
		Services: make(map[string]actualService, len(file.Services)),
	}
	// Sort service names so links (and thus the emitted facts) are deterministic.
	names := make([]string, 0, len(file.Services))
	for name := range file.Services {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		service := file.Services[name]
		facts.Services[name] = actualService{Name: name, Image: service.Image}
		for _, dep := range service.DependsOn {
			// source depends on target: matches the diagram's source->target edge.
			facts.Links = append(facts.Links, actualLink{Source: name, Target: dep})
		}
	}

	out, err := json.Marshal(facts)
	if err != nil {
		return "", nil, err
	}
	return string(out), nil, nil
}

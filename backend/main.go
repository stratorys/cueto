// Command backend serves CUE evaluation for the diagram app.
//
// The hand-owned schema.cue is loaded from disk (CUE_DIR, default ../cue).
// The editable data.cue is supplied per request and overlaid on top, so the
// canvas only ever round-trips data.cue while schema.cue stays authoritative.
package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"
)

type server struct {
	cueDir string
	ctx    *cue.Context
}

func main() {
	cueDir := os.Getenv("CUE_DIR")
	if cueDir == "" {
		cueDir = "../cue"
	}
	abs, err := filepath.Abs(cueDir)
	if err != nil {
		log.Fatalf("Resolve CUE_DIR: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8091"
	}

	s := &server{cueDir: abs, ctx: cuecontext.New()}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /eval", s.handleEval)
	mux.HandleFunc("POST /vet", s.handleVet)
	mux.HandleFunc("POST /format", s.handleFormat)

	log.Printf("Listening on :%s, schema dir %s", port, abs)
	if err := http.ListenAndServe(":"+port, cors(mux)); err != nil {
		log.Fatalf("Serve: %v", err)
	}
}

// build evaluates schema.cue + the provided data.cue and returns the `diagram`
// value. The data text is overlaid so schema.cue is always read fresh from disk.
func (s *server) build(dataSrc string) (cue.Value, error) {
	overlay := map[string]load.Source{
		filepath.Join(s.cueDir, "data.cue"): load.FromString(dataSrc),
	}
	cfg := &load.Config{Dir: s.cueDir, Overlay: overlay}

	instances := load.Instances([]string{"."}, cfg)
	if len(instances) == 0 {
		return cue.Value{}, errors.New("no CUE instance loaded")
	}
	if err := instances[0].Err; err != nil {
		return cue.Value{}, err
	}

	value := s.ctx.BuildInstance(instances[0])
	if err := value.Err(); err != nil {
		return cue.Value{}, err
	}
	return value.LookupPath(cue.ParsePath("diagram")), nil
}

// handleEval returns the concrete diagram as JSON: { nodes: {...}, edges: [...] }.
func (s *server) handleEval(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	diagram, err := s.build(req.Data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := diagram.Validate(cue.Concrete(true)); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	out, err := diagram.MarshalJSON()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

// handleVet validates data.cue against schema.cue without returning the value.
func (s *server) handleVet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	diagram, err := s.build(req.Data)
	if err == nil {
		err = diagram.Validate(cue.Concrete(true))
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": errString(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleFormat runs `cue fmt` over the provided source.
func (s *server) handleFormat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	formatted, err := format.Source([]byte(req.Source))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"formatted": string(formatted)})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": errString(err)})
}

// errString renders CUE errors with their positions when available.
func errString(err error) string {
	var cerr cueerrors.Error
	if errors.As(err, &cerr) {
		return cueerrors.Details(cerr, nil)
	}
	return err.Error()
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

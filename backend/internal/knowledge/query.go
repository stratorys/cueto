package knowledge

import (
	"context"
	"encoding/json"

	"github.com/stratorys/cueto/backend/internal/evaluation"
)

// Query evaluates a CUE expression against the same guarded module source used
// by Compile. It is a convenience on the concrete compiler, not part of the
// phase-one Compiler interface.
func (c *CueCompiler) Query(ctx context.Context, request CompileRequest, expression string) (json.RawMessage, []Diagnostic, error) {
	return c.engine.EvalQuery(ctx, sourceFrom(request), expression)
}

var _ = evaluation.ErrTimeout

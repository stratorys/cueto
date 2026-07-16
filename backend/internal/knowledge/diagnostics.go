package knowledge

import "github.com/stratorys/cueto/backend/internal/diag"

// Diagnostic preserves the existing structured, source-scrubbed diagnostic
// contract while giving the compiler its own public vocabulary.
type Diagnostic = diag.Diagnostic

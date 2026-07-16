package knowledge

// Health reports whether the complete module passes CUE validity checks. Compile
// can still return a selected package value when a sibling is unhealthy, which is
// useful for editors; callers that need a CI gate inspect Health.Valid.
type Health struct {
	Valid       bool
	Diagnostics []Diagnostic
}

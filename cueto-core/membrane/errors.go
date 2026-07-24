package membrane

import "fmt"

// LoadError reports that a directory yielded no loadable CUE instance. Native CUE load
// and build failures are returned as-is, not wrapped in this type.
type LoadError struct {
	Dir    string
	Reason string
}

func (e *LoadError) Error() string {
	return fmt.Sprintf("membrane: cannot load %q: %s", e.Dir, e.Reason)
}

// PathError reports that a lookup path could not be parsed.
type PathError struct {
	Path string
	Err  error
}

func (e *PathError) Error() string {
	return fmt.Sprintf("membrane: invalid path %q: %v", e.Path, e.Err)
}

func (e *PathError) Unwrap() error { return e.Err }

// NotFoundError reports that a valid path resolved to no existing value.
type NotFoundError struct {
	Path string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("membrane: no value at path %q", e.Path)
}

// NoOriginError reports that a value has no valid source position.
type NoOriginError struct {
	Path string
}

func (e *NoOriginError) Error() string {
	return fmt.Sprintf("membrane: no source origin for path %q", e.Path)
}

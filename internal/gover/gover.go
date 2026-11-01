// Package gover tells tests which "go" directive to put in the scratch
// modules they create (fixtures for compile-fail, vet and generator tests).
package gover

import (
	"runtime"
	"strings"
)

// Min is the oldest language version QLX supports.
const Min = "1.27"

// Directive returns the go directive for a scratch module: the running
// toolchain's major.minor, never older than Min unless the toolchain itself is
// older (so a developer on an older toolchain still gets a buildable module).
func Directive() string {
	v := strings.TrimPrefix(runtime.Version(), "go")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return Min
	}
	mm := parts[0] + "." + parts[1]
	if i := strings.IndexAny(mm, " -+"); i >= 0 { // devel, rc suffixes
		mm = mm[:i]
	}
	return mm
}

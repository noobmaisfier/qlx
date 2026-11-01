package main

import (
	"fmt"
	"io"

	"github.com/noobmaisfier/qlx/internal/vet"
)

// cmdVet runs the static checks over the packages given (default ./...).
// It exits non-zero when anything is found, so it can gate CI.
func cmdVet(patterns []string, out io.Writer) (int, error) {
	ds, err := vet.Run(".", patterns)
	if err != nil {
		return 2, err
	}
	for _, d := range ds {
		fmt.Fprintln(out, d)
	}
	if len(ds) > 0 {
		return 1, fmt.Errorf("%d finding(s)", len(ds))
	}
	return 0, nil
}

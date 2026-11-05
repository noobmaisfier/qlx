package qlxvet_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/noobmaisfier/qlx/analyzer/qlxvet"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), qlxvet.Analyzer, "./src/app", "./src/app2", "./src/shape", "./src/mapping", "./src/gaps", "./src/repo", "./src/gm/use")
}

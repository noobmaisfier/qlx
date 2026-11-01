// Command qlxvet runs the QLX analyzers standalone or as a vet tool:
//
//	go vet -vettool=$(which qlxvet) ./...
//	qlxvet ./...
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/noobmaisfier/qlx/analyzer/qlxvet"
)

func main() { singlechecker.Main(qlxvet.Analyzer) }

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/noobmaisfier/qlx/internal/pgtool"
	"github.com/noobmaisfier/qlx/qtest"
)

// cmdPrepare asks a PostgreSQL server to PREPARE every query of a dump made
// by qtest.Dump. The server is, in order: DATABASE_URL, database_url from
// qlx.json, or a throwaway server built from the migrations. A query the
// server rejects is reported by name with the server's own message.
func cmdPrepare(f File, base, queries string, out io.Writer) error {
	if queries == "" {
		return errors.New("prepare needs -queries <file> (write it with qtest.Dump)")
	}
	data, err := os.ReadFile(queries)
	if err != nil {
		return err
	}
	var qs []qtest.Query
	if err := json.Unmarshal(data, &qs); err != nil {
		return fmt.Errorf("%s: %w", queries, err)
	}
	var conn pgtool.Conn
	switch url := firstNonEmpty(os.Getenv("DATABASE_URL"), f.DatabaseURL); {
	case url != "":
		conn = pgtool.FromURL(url)
	case f.Migrations != "":
		srv, err := pgtool.StartEphemeral(f.PGBin)
		if err != nil {
			return fmt.Errorf("starting a throwaway server: %w", err)
		}
		defer srv.Close()
		if err := srv.Conn().ApplySQLDir(rel(base, f.Migrations)); err != nil {
			return fmt.Errorf("applying migrations: %w", err)
		}
		conn = srv.Conn()
	default:
		return errors.New("prepare needs DATABASE_URL, database_url or migrations in qlx.json")
	}

	var mu sync.Mutex
	var bad []string
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, q := range qs {
		wg.Add(1)
		sem <- struct{}{}
		go func(q qtest.Query) {
			defer wg.Done()
			defer func() { <-sem }()
			// A fresh session per query: PREPARE names cannot clash and one
			// failure cannot poison the next.
			if err := conn.Exec("PREPARE qlx_check AS " + q.SQL); err != nil {
				msg := strings.TrimPrefix(err.Error(), "psql: ")
				mu.Lock()
				bad = append(bad, fmt.Sprintf("%s: %s", q.Name, msg))
				mu.Unlock()
			}
		}(q)
	}
	wg.Wait()
	sort.Strings(bad)
	if len(bad) > 0 {
		return fmt.Errorf("%d of %d queries rejected by the server:\n  %s", len(bad), len(qs), strings.Join(bad, "\n  "))
	}
	fmt.Fprintf(out, "qlx: %d queries prepared\n", len(qs))
	return nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

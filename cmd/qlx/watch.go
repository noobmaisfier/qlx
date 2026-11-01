package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// snapshot fingerprints the inputs of `qlx gen`: the config and every .sql
// file of the migrations directory (name, size, mtime). Polling keeps the
// command dependency-free and behaves the same on every OS and file system.
func snapshot(cfgPath, migDir string) string {
	var parts []string
	add := func(p string) {
		if st, err := os.Stat(p); err == nil {
			parts = append(parts, fmt.Sprintf("%s|%d|%d", p, st.Size(), st.ModTime().UnixNano()))
		}
	}
	add(cfgPath)
	if migDir != "" {
		_ = filepath.WalkDir(migDir, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(p, ".sql") {
				add(p)
			}
			return nil
		})
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

// watch regenerates whenever the migrations or the config change, until ctx
// ends. A failed generation is reported and does not stop the loop: the next
// save usually fixes it. Writing is atomic (see cmdGen), so a half-written
// package is never left behind.
func watch(ctx context.Context, f File, base, cfgPath string, every time.Duration, out, errw io.Writer) error {
	if f.Migrations == "" {
		return errors.New("watch needs migrations in qlx.json (a live database_url has nothing to watch)")
	}
	dir := rel(base, f.Migrations)
	last := ""
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		if cur := snapshot(cfgPath, dir); cur != last {
			if last != "" {
				// Let an editor finish its save-and-rename before reading.
				time.Sleep(every / 2)
				cur = snapshot(cfgPath, dir)
			}
			last = cur
			start := time.Now()
			if nf, err := loadConfig(cfgPath); err == nil {
				f = nf // pick up config edits
			} else {
				fmt.Fprintln(errw, "qlx watch:", err)
				goto wait
			}
			if err := cmdGen(f, base, false, io.Discard); err != nil {
				fmt.Fprintf(errw, "qlx watch: gen failed: %v\n", err)
			} else {
				fmt.Fprintf(out, "qlx watch: regenerated in %s\n", time.Since(start).Round(time.Millisecond))
			}
		}
	wait:
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

func cmdWatch(f File, base, cfgPath string, out, errw io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Fprintln(out, "qlx watch: watching", rel(base, f.Migrations), "(Ctrl-C to stop)")
	return watch(ctx, f, base, cfgPath, 500*time.Millisecond, out, errw)
}

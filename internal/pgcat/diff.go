package pgcat

import (
	"fmt"
	"sort"
	"strings"
)

// Diff lists what changed between two ledgers read from single server
// versions (old, then new): entries added, removed, and functions whose
// result type, volatility or strictness changed. It is the first step of
// reviewing a release: every line is something the Go API may need to follow.
func Diff(old, cur *Ledger) string {
	var b strings.Builder
	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		sort.Strings(items)
		fmt.Fprintf(&b, "## %s (%d)\n\n", title, len(items))
		for _, s := range items {
			fmt.Fprintf(&b, "- `%s`\n", s)
		}
		b.WriteString("\n")
	}

	of := map[string]Fn{}
	for _, f := range old.Funcs {
		of[f.shapeKey()] = f
	}
	var added, removed, changed []string
	cf := map[string]bool{}
	for _, f := range cur.Funcs {
		cf[f.shapeKey()] = true
		o, ok := of[f.shapeKey()]
		switch {
		case !ok:
			added = append(added, f.Sig()+" → "+f.Ret+shapeNote(f))
		case o.Ret != f.Ret || o.Volatility != f.Volatility || o.Strict != f.Strict || o.RetSet != f.RetSet:
			changed = append(changed, fmt.Sprintf("%s: %s/%s/strict=%v → %s/%s/strict=%v", f.Sig(), o.Ret, o.Volatility, o.Strict, f.Ret, f.Volatility, f.Strict))
		}
	}
	for s := range of {
		if !cf[s] {
			removed = append(removed, of[s].Sig()+" → "+of[s].Ret+shapeNote(of[s]))
		}
	}
	section("Functions added", added)
	section("Functions removed", removed)
	section("Functions changed", changed)

	oo := map[string]bool{}
	for _, o := range old.Ops {
		oo[o.Sig()] = true
	}
	co := map[string]bool{}
	var oadd, orem []string
	for _, o := range cur.Ops {
		co[o.Sig()] = true
		if !oo[o.Sig()] {
			oadd = append(oadd, strings.TrimSpace(o.Sig())+" → "+o.Ret)
		}
	}
	for _, o := range old.Ops {
		if !co[o.Sig()] {
			orem = append(orem, strings.TrimSpace(o.Sig()))
		}
	}
	section("Operators added", oadd)
	section("Operators removed", orem)

	oc := map[string]string{}
	for _, c := range old.Casts {
		oc[c.Sig()] = c.Context
	}
	cc := map[string]bool{}
	var cadd, crem, cctx []string
	for _, c := range cur.Casts {
		cc[c.Sig()] = true
		ctx, ok := oc[c.Sig()]
		switch {
		case !ok:
			cadd = append(cadd, c.Sig()+" ("+c.Context+")")
		case ctx != c.Context:
			cctx = append(cctx, fmt.Sprintf("%s: %s → %s", c.Sig(), ctx, c.Context))
		}
	}
	for s := range oc {
		if !cc[s] {
			crem = append(crem, s)
		}
	}
	section("Casts added", cadd)
	section("Casts removed", crem)
	section("Cast context changed", cctx)

	if b.Len() == 0 {
		return NoDifferences
	}
	return b.String()
}

// shapeNote adds the column count of a record result, so two shapes of one
// signature read differently in a diff.
func shapeNote(f Fn) string {
	if len(f.Outs) == 0 {
		return ""
	}
	return fmt.Sprintf(" (%d columns)", len(f.Outs))
}

// NoDifferences is what Diff returns for two identical ledgers.
const NoDifferences = "No differences.\n"

// At returns the ledger as one server of the given major would report it:
// the entries whose version range covers major, with the range cleared and
// Versions set to that major. A ledger merged over several majors can then be
// compared with an extraction from a single server.
func (l *Ledger) At(major int) *Ledger {
	in := func(since, until int) bool {
		return (since == 0 || since <= major) && (until == 0 || major < until)
	}
	out := &Ledger{Extension: l.Extension, Schema: l.Schema, Versions: []int{major}}
	for _, f := range l.Funcs {
		if in(f.Since, f.Until) {
			f.Since, f.Until = 0, 0
			out.Funcs = append(out.Funcs, f)
		}
	}
	for _, o := range l.Ops {
		if in(o.Since, o.Until) {
			o.Since, o.Until = 0, 0
			out.Ops = append(out.Ops, o)
		}
	}
	out.Casts = append(out.Casts, l.Casts...)
	return out
}

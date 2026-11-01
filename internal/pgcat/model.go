// Package pgcat reads PostgreSQL's built-in functions, operators and casts
// into a ledger, merges ledgers from several server versions so each entry
// knows the version it first appeared in, and measures what the Go API covers.
package pgcat

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Fn is a pg_catalog function (or aggregate or window function).
type Fn struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"` // f function, a aggregate, w window
	Args       []string `json:"args"` // type names (pg_type.typname)
	ArgNames   []string `json:"arg_names,omitempty"`
	Ret        string   `json:"ret"`
	RetSet     bool     `json:"ret_set,omitempty"`
	Strict     bool     `json:"strict,omitempty"`
	Volatility string   `json:"volatility"` // i, s, v
	Variadic   bool     `json:"variadic,omitempty"`
	Defaults   int      `json:"defaults,omitempty"`
	OutModes   bool     `json:"out_modes,omitempty"` // has OUT / INOUT / TABLE arguments
	Outs       []Out    `json:"outs,omitempty"`      // OUT / INOUT / TABLE columns, in order
	Since      int      `json:"since,omitempty"`     // first major version seen; 0 = all versions read
	Until      int      `json:"until,omitempty"`     // first merged major that no longer has it; 0 = still there
	Schema     string   `json:"schema,omitempty"`    // owning schema; empty means pg_catalog
}

// shapeKey is Sig plus the output columns: a function that keeps its name and
// arguments but returns a different record in another major is a different
// entry, with its own version range (pg_stat_get_wal has three shapes in 16 to
// 19). Entries without Outs and entries with the same columns share a key.
func (f Fn) shapeKey() string {
	if len(f.Outs) == 0 {
		return f.Sig()
	}
	var b strings.Builder
	b.WriteString(f.Sig())
	b.WriteString("→")
	for _, o := range f.Outs {
		b.WriteString(o.Name + ":" + o.Type + ",")
	}
	return b.String()
}

// Out is one output column of a function with OUT, INOUT or TABLE arguments.
type Out struct {
	Name string `json:"name"`
	Type string `json:"type"` // pg_type.typname
}

// Sig is the identity of a function: name and argument types.
func (f Fn) Sig() string { return f.Name + "(" + strings.Join(f.Args, ",") + ")" }

// Op is a pg_catalog operator.
type Op struct {
	Name   string `json:"name"`
	Left   string `json:"left,omitempty"` // "" for a prefix operator
	Right  string `json:"right"`
	Ret    string `json:"ret"`
	Proc   string `json:"proc"`
	Since  int    `json:"since,omitempty"`
	Until  int    `json:"until,omitempty"`
	Schema string `json:"schema,omitempty"` // owning schema; empty means pg_catalog
}

// Sig is the identity of an operator.
func (o Op) Sig() string { return o.Left + " " + o.Name + " " + o.Right }

// Cast is a pg_cast entry.
type Cast struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Context string `json:"context"` // i implicit, a assignment, e explicit
	Since   int    `json:"since,omitempty"`
	Until   int    `json:"until,omitempty"`
}

// Sig is the identity of a cast.
func (c Cast) Sig() string { return c.From + "->" + c.To }

// Ledger is the content of a pgcat file.
type Ledger struct {
	// Extension names the extension this ledger describes ("" for the
	// built-in catalog); Schema is where the extension installed itself.
	Extension string `json:"extension,omitempty"`
	Schema    string `json:"schema,omitempty"`
	Versions  []int  `json:"versions"` // server majors merged into this ledger
	Funcs     []Fn   `json:"funcs"`
	Ops       []Op   `json:"ops"`
	Casts     []Cast `json:"casts"`
}

// Normalize sorts the ledger into a canonical order.
func (l *Ledger) Normalize() {
	sort.Ints(l.Versions)
	sort.Slice(l.Funcs, func(i, j int) bool {
		a, b := l.Funcs[i], l.Funcs[j]
		if a.Sig() != b.Sig() {
			return a.Sig() < b.Sig()
		}
		return a.shapeKey() < b.shapeKey()
	})
	sort.Slice(l.Ops, func(i, j int) bool { return l.Ops[i].Sig() < l.Ops[j].Sig() })
	sort.Slice(l.Casts, func(i, j int) bool { return l.Casts[i].Sig() < l.Casts[j].Sig() })
}

// Marshal renders the canonical JSON.
func (l *Ledger) Marshal() ([]byte, error) {
	l.Normalize()
	b, err := json.MarshalIndent(l, "", " ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Parse reads a ledger.
func Parse(data []byte) (*Ledger, error) {
	var l Ledger
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("pgcat ledger: %w", err)
	}
	return &l, nil
}

// Merge combines ledgers read from different server versions. For every entry
// it records where the entry lives:
//
//	Since  the lowest merged version that has it, or 0 when the lowest merged
//	       version does too (so adding an older server later rewrites nothing);
//	Until  the first merged version above its last appearance that lacks it,
//	       or 0 when the newest merged version still has it.
//
// An entry is "present in version v" when some input ledger lists v among its
// Versions and the entry's own Since/Until (a merged ledger can be merged
// again) include v. Until can only be as precise as the versions you merge:
// with ledgers for 16, 18 and 19, a function that left in 17 is reported as
// removed in 18, so extract every major you target.
func Merge(ledgers ...*Ledger) *Ledger {
	out := &Ledger{}
	have := map[int]bool{}
	for _, l := range ledgers {
		if out.Extension == "" {
			out.Extension, out.Schema = l.Extension, l.Schema
		}
		for _, v := range l.Versions {
			have[v] = true
		}
	}
	for v := range have {
		out.Versions = append(out.Versions, v)
	}
	sort.Ints(out.Versions)

	// presence lists the versions of l in which an entry with this own
	// since/until exists.
	presence := func(l *Ledger, since, until int) []int {
		var vs []int
		for _, v := range l.Versions {
			if v >= since && (until == 0 || v < until) {
				vs = append(vs, v)
			}
		}
		return vs
	}
	span := func(present map[int]bool) (since, until int) {
		lo, hi := 0, 0
		for v := range present {
			if lo == 0 || v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
		if lo > out.Versions[0] {
			since = lo
		}
		for _, v := range out.Versions {
			if v > hi {
				until = v
				break
			}
		}
		return
	}

	fn, fnP := map[string]*Fn{}, map[string]map[int]bool{}
	op, opP := map[string]*Op{}, map[string]map[int]bool{}
	cs, csP := map[string]*Cast{}, map[string]map[int]bool{}
	add := func(m map[string]map[int]bool, sig string, vs []int) {
		if m[sig] == nil {
			m[sig] = map[int]bool{}
		}
		for _, v := range vs {
			m[sig][v] = true
		}
	}
	// An entry read before OUT columns were recorded (no Outs) adopts the
	// richest shape known for its signature instead of becoming a shape of
	// its own.
	best := map[string][]Out{}
	for _, l := range ledgers {
		for _, f := range l.Funcs {
			if len(f.Outs) > len(best[f.Sig()]) {
				best[f.Sig()] = f.Outs
			}
		}
	}
	for _, l := range ledgers {
		for i := range l.Funcs {
			f := l.Funcs[i]
			if len(f.Outs) == 0 && (f.OutModes || f.Ret == "record") {
				f.Outs = best[f.Sig()]
			}
			k := f.shapeKey()
			add(fnP, k, presence(l, f.Since, f.Until))
			if _, ok := fn[k]; !ok {
				fn[k] = &f
			}
		}
		for i := range l.Ops {
			o := l.Ops[i]
			add(opP, o.Sig(), presence(l, o.Since, o.Until))
			if _, ok := op[o.Sig()]; !ok {
				op[o.Sig()] = &o
			}
		}
		for i := range l.Casts {
			c := l.Casts[i]
			add(csP, c.Sig(), presence(l, c.Since, c.Until))
			if _, ok := cs[c.Sig()]; !ok {
				cs[c.Sig()] = &c
			}
		}
	}
	if len(out.Versions) == 0 {
		return out
	}
	for s, f := range fn {
		f.Since, f.Until = span(fnP[s])
		out.Funcs = append(out.Funcs, *f)
	}
	for s, o := range op {
		o.Since, o.Until = span(opP[s])
		out.Ops = append(out.Ops, *o)
	}
	for s, c := range cs {
		c.Since, c.Until = span(csP[s])
		out.Casts = append(out.Casts, *c)
	}
	out.Normalize()
	return out
}

#!/usr/bin/env python3
"""Builds docs/GUIDE.md (French) from docs/guide/GUIDE.tmpl.md and
docs/GUIDE.en.md (English) from docs/guide/GUIDE.en.tmpl.md.

A line `{{ex:name}}` becomes the Go source of exName in guide_examples_test.go
followed by the SQL (and arguments) that test renders for it on PostgreSQL 18.

  QLX_GUIDE_OUT=/tmp/guide_sql.txt go test -run TestGuideExamples .
  python3 docs/guide/gen.py /tmp/guide_sql.txt
"""
import re, sys, textwrap, pathlib

root = pathlib.Path(__file__).resolve().parents[2]
src = (root / "guide_examples_test.go").read_text()
sql_file = pathlib.Path(sys.argv[1]).read_text()

sqls = {}
for block in sql_file.split("=== ")[1:]:
    name, rest = block.split("\n", 1)
    sql, args = rest.rsplit("\n--- ", 1)
    sqls[name.strip()] = (sql.strip(), args.strip())

funcs = {}
for m in re.finditer(r"// ex: ([a-z0-9-]+)\nfunc \w+\(\) q\.Statement \{(.*?)\n\}\n", src, re.S):
    body = textwrap.dedent(m.group(2).strip("\n"))
    lines = body.split("\n")
    if len(lines) and lines[0].startswith("return ") and not any(l.startswith("return ") for l in lines[1:]):
        lines[0] = lines[0][len("return "):]
        # continuation lines were indented one tab deeper than `return`
        lines = [lines[0]] + [l[1:] if l.startswith("\t") else l for l in lines[1:]]
    funcs[m.group(1)] = "\n".join(lines).replace("\t", "\t")
funcs_single = {k: v for k, v in funcs.items()}

def make_expand(args_label):
    def expand(m):
        name = m.group(1)
        if name not in funcs or name not in sqls:
            sys.exit(f"unknown example {name}")
        code = funcs[name]
        sql, args = sqls[name]
        out = "```go\n" + code + "\n```\n\n```sql\n" + sql + "\n```\n"
        if args and args != "[]":
            out += f"\n{args_label} `{args}`\n"
        return out
    return expand

# (template, output, label of the arguments line)
for tmpl_name, out_name, label in [
    ("GUIDE.tmpl.md", "GUIDE.md", "Arguments :"),
    ("GUIDE.en.tmpl.md", "GUIDE.en.md", "Arguments:"),
]:
    tmpl = (root / "docs/guide" / tmpl_name).read_text()
    text = re.sub(r"^\{\{ex:([a-z0-9-]+)\}\}$", make_expand(label), tmpl, flags=re.M)
    (root / "docs" / out_name).write_text(text)
    print("wrote docs/" + out_name, len(text.splitlines()), "lines")

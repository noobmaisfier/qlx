package qlx_test

import (
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

func xmlCases() []goldenCase {
	doc := q.NewParam[qt.XML]()
	id := q.NewField[int64]("id")
	name := q.NewField[string]("name")
	n := q.NewField[int64]("n")
	tbl := q.XMLTable(q.Val("/rows/row"), doc, "x",
		q.XMLColOrdinality(n),
		q.XMLColValue(id, q.PgInt8).Path(q.Val("@id")).NotNull(),
		q.XMLColValue(name, q.PgText).Path(q.Val("name/text()")).Default(q.Val("none"))).
		Namespaces(q.XMLNamespace("urn:example", "e"), q.XMLNamespace("urn:other", "o"))
	return []goldenCase{
		{name: "xmlelement with attributes and content", stmt: q.Select(
			q.XMLElement("user").Attrs(q.XMLAttribute("id", Users.ID)).Content(Users.Email).Expr().As("x")).From(Users)},
		{name: "xmlforest", stmt: q.Select(
			q.XMLForest(q.XMLAttribute("id", Users.ID), q.XMLAttribute("email", Users.Email)).As("x")).From(Users)},
		{name: "xmlconcat", stmt: q.Select(q.XMLConcat(doc, q.XMLParse(q.XMLContent, Users.Email).Expr()).As("x")).From(Users)},
		{name: "xmlparse document strip", stmt: q.Select(q.XMLParse(q.XMLDocument, Users.Email).StripWhitespace().Expr().As("x")).From(Users)},
		{name: "xmlpi", stmt: q.Select(q.XMLPI("php").As("a"), q.XMLPIContent("php", Users.Email).As("b")).From(Users)},
		{name: "xmlroot", stmt: q.Select(q.XMLRoot(doc).Version(q.Val("1.0")).StandaloneYes().Expr().As("x"))},
		{name: "xmlroot no value", stmt: q.Select(q.XMLRoot(doc).StandaloneNoValue().Expr().As("x"))},
		{name: "xmlserialize indent", stmt: q.Select(q.XMLSerialize(q.XMLContent, doc).Indent().Text().As("t"))},
		{name: "xmlexists", stmt: q.Select(Users.ID).From(Users).Where(q.XMLExists(q.Val("/a"), doc))},
		{name: "is document", stmt: q.Select(Users.ID).From(Users).Where(q.IsDocument(doc), q.IsNotDocument(doc))},
		{name: "xmltable", stmt: q.Select(q.RefNum(tbl, id), q.RefText(tbl, name)).From(tbl)},
	}
}

func TestXMLGolden(t *testing.T) { checkGolden(t, "xml", xmlCases()) }

func TestXMLShapeErrors(t *testing.T) {
	for name, st := range map[string]q.Statement{
		"empty xmlforest":  q.Select(q.XMLForest().As("x")),
		"empty xmlconcat":  q.Select(q.XMLConcat().As("x")),
		"xmltable no cols": q.Select(Users.ID).From(q.XMLTable(q.Val("/a"), q.NewParam[qt.XML](), "t")),
	} {
		if _, err := q.Render(q.Dialect{Version: q.PG19}, st); err == nil {
			t.Errorf("%s rendered", name)
		}
	}
}

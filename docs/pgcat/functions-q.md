# Functions Q

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `xml.QueryToXml` | `query_to_xml(text,bool,bool,text)` | `xml` | all | volatile |
| `xml.QueryToXmlAndXmlschema` | `query_to_xml_and_xmlschema(text,bool,bool,text)` | `xml` | all | volatile |
| `xml.QueryToXmlschema` | `query_to_xmlschema(text,bool,bool,text)` | `xml` | all | volatile |
| `fts.Querytree` | `querytree(tsquery)` | `text` | all | immutable |
| `text.QuoteIdent` | `quote_ident(text)` | `text` | all | immutable |
| `array.QuoteLiteral` | `quote_literal(anyelement)` | `text` | all | stable |
| `text.QuoteLiteral` | `quote_literal(text)` | `text` | all | immutable |
| `array.QuoteNullable` | `quote_nullable(anyelement)` | `text` | all | stable, may return NULL |
| `text.QuoteNullable` | `quote_nullable(text)` | `text` | all | immutable, may return NULL |

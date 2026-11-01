# Functions X

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `sys.Xid8Larger` | `xid8_larger(xid8,xid8)` | `xid8` | all | immutable |
| `sys.Xid8Smaller` | `xid8_smaller(xid8,xid8)` | `xid8` | all | immutable |
| `sys.Xid8cmp` | `xid8cmp(xid8,xid8)` | `int4` | all | immutable |
| `text.XmlIsWellFormed` | `xml_is_well_formed(text)` | `bool` | all | stable |
| `text.XmlIsWellFormedContent` | `xml_is_well_formed_content(text)` | `bool` | all | immutable |
| `text.XmlIsWellFormedDocument` | `xml_is_well_formed_document(text)` | `bool` | all | immutable |
| `xml.XmlSend` | `xml_send(xml)` | `bytea` | all | stable |
| `xml.Xmlagg` | `xmlagg(xml)` | `xml` | all | immutable, may return NULL |
| `xml.Xmlcomment` | `xmlcomment(text)` | `xml` | all | immutable |
| `xml.Xmlconcat2` | `xmlconcat2(xml,xml)` | `xml` | all | immutable, may return NULL |
| `xml.Xmlexists` | `xmlexists(text,xml)` | `bool` | all | immutable |
| `xml.Xmltext` | `xmltext(text)` | `xml` | 17+ | immutable |
| `xml.Xmlvalidate` | `xmlvalidate(xml,text)` | `bool` | all | immutable |
| `xml.XpathTextXml` | `xpath(text,xml)` | `_xml` | all | immutable |
| `xml.XpathTextXmlTextArray` | `xpath(text,xml,_text)` | `_xml` | all | immutable |
| `xml.XpathExistsTextXml` | `xpath_exists(text,xml)` | `bool` | all | immutable |
| `xml.XpathExistsTextXmlTextArray` | `xpath_exists(text,xml,_text)` | `bool` | all | immutable |

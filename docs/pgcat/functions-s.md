# Functions S

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `sys.SatisfiesHashPartition` | `satisfies_hash_partition(oid,int4,int4,any)` | `bool` | all | immutable, may return NULL |
| `num.Scale` | `scale(numeric)` | `int4` | all | immutable |
| `xml.SchemaToXml` | `schema_to_xml(name,bool,bool,text)` | `xml` | all | stable |
| `xml.SchemaToXmlAndXmlschema` | `schema_to_xml_and_xmlschema(name,bool,bool,text)` | `xml` | all | stable |
| `xml.SchemaToXmlschema` | `schema_to_xmlschema(name,bool,bool,text)` | `xml` | all | stable |
| `sys.SessionUser` | `session_user()` | `name` | all | stable |
| `binary.SetBitBitInt4Int4` | `set_bit(bit,int4,int4)` | `bit` | all | immutable |
| `binary.SetBitByteaInt8Int4` | `set_bit(bytea,int8,int4)` | `bytea` | all | immutable |
| `binary.SetByte` | `set_byte(bytea,int4,int4)` | `bytea` | all | immutable |
| `text.SetConfig` | `set_config(text,text,bool)` | `text` | all | volatile, may return NULL |
| `network.SetMasklenCidrInt4` | `set_masklen(cidr,int4)` | `cidr` | all | immutable |
| `network.SetMasklenInetInt4` | `set_masklen(inet,int4)` | `inet` | all | immutable |
| `num.Setseed` | `setseed(float8)` | `void` | all | volatile |
| `sys.SetvalRegclassInt8` | `setval(regclass,int8)` | `int8` | all | volatile |
| `sys.SetvalRegclassInt8Bool` | `setval(regclass,int8,bool)` | `int8` | all | volatile |
| `fts.SetweightTsvectorChar` | `setweight(tsvector,char)` | `tsvector` | all | immutable |
| `fts.SetweightTsvectorCharTextArray` | `setweight(tsvector,char,_text)` | `tsvector` | all | immutable |
| `binary.Sha224` | `sha224(bytea)` | `bytea` | all | immutable |
| `binary.Sha256` | `sha256(bytea)` | `bytea` | all | immutable |
| `binary.Sha384` | `sha384(bytea)` | `bytea` | all | immutable |
| `binary.Sha512` | `sha512(bytea)` | `bytea` | all | immutable |
| `sys.ShobjDescription` | `shobj_description(oid,name)` | `text` | all | stable, may return NULL |
| `num.SignFloat8` | `sign(float8)` | `float8` | all | immutable |
| `num.SignNumeric` | `sign(numeric)` | `numeric` | all | immutable |
| `text.SimilarEscape` | `similar_escape(text,text)` | `text` | all | immutable, may return NULL |
| `text.SimilarToEscapeText` | `similar_to_escape(text)` | `text` | all | immutable |
| `text.SimilarToEscapeTextText` | `similar_to_escape(text,text)` | `text` | all | immutable |
| `num.Sin` | `sin(float8)` | `float8` | all | immutable |
| `num.Sind` | `sind(float8)` | `float8` | all | immutable |
| `num.Sinh` | `sinh(float8)` | `float8` | all | immutable |
| `geo.Slope` | `slope(point,point)` | `float8` | all | immutable |
| `geo.SpgPolyQuadCompress` | `spg_poly_quad_compress(polygon)` | `box` | all | immutable |
| `text.SplitPart` | `split_part(text,text,int4)` | `text` | all | immutable |
| `num.SqrtFloat8` | `sqrt(float8)` | `float8` | all | immutable |
| `num.SqrtNumeric` | `sqrt(numeric)` | `numeric` | all | immutable |
| `text.StartsWith` | `starts_with(text,text)` | `bool` | all | immutable |
| `datetime.StatementTimestamp` | `statement_timestamp()` | `timestamptz` | all | stable |
| `num.StddevFloat4` | `stddev(float4)` | `float8` | all | immutable, may return NULL |
| `num.StddevFloat8` | `stddev(float8)` | `float8` | all | immutable, may return NULL |
| `num.StddevInt2` | `stddev(int2)` | `numeric` | all | immutable, may return NULL |
| `num.StddevInt4` | `stddev(int4)` | `numeric` | all | immutable, may return NULL |
| `num.StddevInt8` | `stddev(int8)` | `numeric` | all | immutable, may return NULL |
| `num.StddevNumeric` | `stddev(numeric)` | `numeric` | all | immutable, may return NULL |
| `num.StddevPopFloat4` | `stddev_pop(float4)` | `float8` | all | immutable, may return NULL |
| `num.StddevPopFloat8` | `stddev_pop(float8)` | `float8` | all | immutable, may return NULL |
| `num.StddevPopInt2` | `stddev_pop(int2)` | `numeric` | all | immutable, may return NULL |
| `num.StddevPopInt4` | `stddev_pop(int4)` | `numeric` | all | immutable, may return NULL |
| `num.StddevPopInt8` | `stddev_pop(int8)` | `numeric` | all | immutable, may return NULL |
| `num.StddevPopNumeric` | `stddev_pop(numeric)` | `numeric` | all | immutable, may return NULL |
| `num.StddevSampFloat4` | `stddev_samp(float4)` | `float8` | all | immutable, may return NULL |
| `num.StddevSampFloat8` | `stddev_samp(float8)` | `float8` | all | immutable, may return NULL |
| `num.StddevSampInt2` | `stddev_samp(int2)` | `numeric` | all | immutable, may return NULL |
| `num.StddevSampInt4` | `stddev_samp(int4)` | `numeric` | all | immutable, may return NULL |
| `num.StddevSampInt8` | `stddev_samp(int8)` | `numeric` | all | immutable, may return NULL |
| `num.StddevSampNumeric` | `stddev_samp(numeric)` | `numeric` | all | immutable, may return NULL |
| `binary.StringAgg` | `string_agg(bytea,bytea)` | `bytea` | all | immutable |
| `text.StringAgg` | `string_agg(text,text)` | `text` | all | immutable, may return NULL |
| `array.StringToArrayTextText` | `string_to_array(text,text)` | `_text` | all | immutable |
| `array.StringToArrayTextTextText` | `string_to_array(text,text,text)` | `_text` | all | immutable |
| `text.StringToTableTextText` | `string_to_table(text,text)` | `text` | all | immutable |
| `text.StringToTableTextTextText` | `string_to_table(text,text,text)` | `text` | all | immutable |
| `fts.Strip` | `strip(tsvector)` | `tsvector` | all | immutable |
| `text.Strpos` | `strpos(text,text)` | `int4` | all | immutable |
| `binary.SubstrByteaInt4` | `substr(bytea,int4)` | `bytea` | all | immutable |
| `binary.SubstrByteaInt4Int4` | `substr(bytea,int4,int4)` | `bytea` | all | immutable |
| `text.SubstrTextInt4` | `substr(text,int4)` | `text` | all | immutable |
| `text.SubstrTextInt4Int4` | `substr(text,int4,int4)` | `text` | all | immutable |
| `binary.SubstringBitInt4` | `substring(bit,int4)` | `bit` | all | immutable |
| `binary.SubstringBitInt4Int4` | `substring(bit,int4,int4)` | `bit` | all | immutable |
| `binary.SubstringByteaInt4` | `substring(bytea,int4)` | `bytea` | all | immutable |
| `binary.SubstringByteaInt4Int4` | `substring(bytea,int4,int4)` | `bytea` | all | immutable |
| `text.SubstringTextInt4` | `substring(text,int4)` | `text` | all | immutable |
| `text.SubstringTextInt4Int4` | `substring(text,int4,int4)` | `text` | all | immutable |
| `text.SubstringTextText` | `substring(text,text)` | `text` | all | immutable, may return NULL |
| `text.SubstringTextTextText` | `substring(text,text,text)` | `text` | all | immutable, may return NULL |
| `num.SumFloat4` | `sum(float4)` | `float4` | all | immutable, may return NULL |
| `num.SumFloat8` | `sum(float8)` | `float8` | all | immutable, may return NULL |
| `num.SumInt2` | `sum(int2)` | `int8` | all | immutable, may return NULL |
| `num.SumInt4` | `sum(int4)` | `int8` | all | immutable, may return NULL |
| `num.SumInt8` | `sum(int8)` | `numeric` | all | immutable, may return NULL |
| `datetime.Sum` | `sum(interval)` | `interval` | all | immutable, may return NULL |
| `num.SumMoney` | `sum(money)` | `money` | all | immutable, may return NULL |
| `num.SumNumeric` | `sum(numeric)` | `numeric` | all | immutable, may return NULL |
| `text.SystemUser` | `system_user()` | `text` | all | stable |

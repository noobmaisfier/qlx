# Functions J

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `jsonb.JsonAgg` | `json_agg(anyelement)` | `json` | all | stable, may return NULL |
| `jsonb.JsonAggStrict` | `json_agg_strict(anyelement)` | `json` | all | stable, may return NULL |
| `jsonb.JsonArrayElements` | `json_array_elements(json)` | `json` | all | immutable |
| `jsonb.JsonArrayElementsText` | `json_array_elements_text(json)` | `text` | all | immutable |
| `jsonb.JsonArrayLength` | `json_array_length(json)` | `int4` | all | immutable |
| `jsonb.JsonBuildArray` | `json_build_array()` | `json` | all | stable, may return NULL |
| `jsonb.JsonBuildArrayAny` | `json_build_array(any)` | `json` | all | stable, may return NULL |
| `jsonb.JsonBuildObject` | `json_build_object()` | `json` | all | stable, may return NULL |
| `jsonb.JsonBuildObjectAny` | `json_build_object(any)` | `json` | all | stable, may return NULL |
| `jsonb.JsonEach` | `json_each(json)` | `record` | all | immutable |
| `jsonb.JsonEachText` | `json_each_text(json)` | `record` | all | immutable |
| `jsonb.JsonObjectTextArray` | `json_object(_text)` | `json` | all | immutable |
| `jsonb.JsonObjectTextArrayTextArray` | `json_object(_text,_text)` | `json` | all | immutable |
| `jsonb.JsonObjectAgg` | `json_object_agg(any,any)` | `json` | all | stable, may return NULL |
| `jsonb.JsonObjectAggStrict` | `json_object_agg_strict(any,any)` | `json` | all | stable, may return NULL |
| `jsonb.JsonObjectAggUnique` | `json_object_agg_unique(any,any)` | `json` | all | stable, may return NULL |
| `jsonb.JsonObjectAggUniqueStrict` | `json_object_agg_unique_strict(any,any)` | `json` | all | stable, may return NULL |
| `jsonb.JsonObjectKeys` | `json_object_keys(json)` | `text` | all | immutable |
| `jsonb.JsonPopulateRecord` | `json_populate_record(anyelement,json,bool)` | `anyelement` | all | stable, may return NULL |
| `jsonb.JsonPopulateRecordset` | `json_populate_recordset(anyelement,json,bool)` | `anyelement` | all | stable |
| `jsonb.JsonSend` | `json_send(json)` | `bytea` | all | immutable |
| `jsonb.JsonStripNullsJson` | `json_strip_nulls(json)` | `json` | up to 17 | immutable |
| `jsonb.JsonStripNullsJsonBool` | `json_strip_nulls(json,bool)` | `json` | 18+ | immutable |
| `jsonb.JsonToTsvectorJsonJsonb` | `json_to_tsvector(json,jsonb)` | `tsvector` | all | stable |
| `jsonb.JsonToTsvectorRegconfigJsonJsonb` | `json_to_tsvector(regconfig,json,jsonb)` | `tsvector` | all | immutable |
| `jsonb.JsonTypeof` | `json_typeof(json)` | `text` | all | immutable |
| `jsonb.JsonbAgg` | `jsonb_agg(anyelement)` | `jsonb` | all | stable, may return NULL |
| `jsonb.JsonbAggStrict` | `jsonb_agg_strict(anyelement)` | `jsonb` | all | stable, may return NULL |
| `jsonb.JsonbArrayElements` | `jsonb_array_elements(jsonb)` | `jsonb` | all | immutable |
| `jsonb.JsonbArrayElementsText` | `jsonb_array_elements_text(jsonb)` | `text` | all | immutable |
| `jsonb.JsonbArrayLength` | `jsonb_array_length(jsonb)` | `int4` | all | immutable |
| `jsonb.JsonbBuildArray` | `jsonb_build_array()` | `jsonb` | all | stable, may return NULL |
| `jsonb.JsonbBuildArrayAny` | `jsonb_build_array(any)` | `jsonb` | all | stable, may return NULL |
| `jsonb.JsonbBuildObject` | `jsonb_build_object()` | `jsonb` | all | stable, may return NULL |
| `jsonb.JsonbBuildObjectAny` | `jsonb_build_object(any)` | `jsonb` | all | stable, may return NULL |
| `jsonb.JsonbCmp` | `jsonb_cmp(jsonb,jsonb)` | `int4` | all | immutable |
| `jsonb.JsonbDeleteJsonbTextArray` | `jsonb_delete(jsonb,_text)` | `jsonb` | all | immutable |
| `jsonb.JsonbDeleteJsonbInt4` | `jsonb_delete(jsonb,int4)` | `jsonb` | all | immutable |
| `jsonb.JsonbDeleteJsonbText` | `jsonb_delete(jsonb,text)` | `jsonb` | all | immutable |
| `jsonb.JsonbEach` | `jsonb_each(jsonb)` | `record` | all | immutable |
| `jsonb.JsonbEachText` | `jsonb_each_text(jsonb)` | `record` | all | immutable |
| `jsonb.JsonbHash` | `jsonb_hash(jsonb)` | `int4` | all | immutable |
| `jsonb.JsonbHashExtended` | `jsonb_hash_extended(jsonb,int8)` | `int8` | all | immutable |
| `jsonb.JsonbInsert` | `jsonb_insert(jsonb,_text,jsonb,bool)` | `jsonb` | all | immutable |
| `jsonb.JsonbObjectTextArray` | `jsonb_object(_text)` | `jsonb` | all | immutable |
| `jsonb.JsonbObjectTextArrayTextArray` | `jsonb_object(_text,_text)` | `jsonb` | all | immutable |
| `jsonb.JsonbObjectAgg` | `jsonb_object_agg(any,any)` | `jsonb` | all | immutable, may return NULL |
| `jsonb.JsonbObjectAggStrict` | `jsonb_object_agg_strict(any,any)` | `jsonb` | all | immutable, may return NULL |
| `jsonb.JsonbObjectAggUnique` | `jsonb_object_agg_unique(any,any)` | `jsonb` | all | immutable, may return NULL |
| `jsonb.JsonbObjectAggUniqueStrict` | `jsonb_object_agg_unique_strict(any,any)` | `jsonb` | all | immutable, may return NULL |
| `jsonb.JsonbObjectKeys` | `jsonb_object_keys(jsonb)` | `text` | all | immutable |
| `jsonb.JsonbPathExists` | `jsonb_path_exists(jsonb,jsonpath,jsonb,bool)` | `bool` | all | immutable, may return NULL |
| `jsonb.JsonbPathExistsTz` | `jsonb_path_exists_tz(jsonb,jsonpath,jsonb,bool)` | `bool` | all | stable, may return NULL |
| `jsonb.JsonbPathMatch` | `jsonb_path_match(jsonb,jsonpath,jsonb,bool)` | `bool` | all | immutable, may return NULL |
| `jsonb.JsonbPathMatchTz` | `jsonb_path_match_tz(jsonb,jsonpath,jsonb,bool)` | `bool` | all | stable, may return NULL |
| `jsonb.JsonbPathQuery` | `jsonb_path_query(jsonb,jsonpath,jsonb,bool)` | `jsonb` | all | immutable |
| `jsonb.JsonbPathQueryArray` | `jsonb_path_query_array(jsonb,jsonpath,jsonb,bool)` | `jsonb` | all | immutable |
| `jsonb.JsonbPathQueryArrayTz` | `jsonb_path_query_array_tz(jsonb,jsonpath,jsonb,bool)` | `jsonb` | all | stable |
| `jsonb.JsonbPathQueryFirst` | `jsonb_path_query_first(jsonb,jsonpath,jsonb,bool)` | `jsonb` | all | immutable, may return NULL |
| `jsonb.JsonbPathQueryFirstTz` | `jsonb_path_query_first_tz(jsonb,jsonpath,jsonb,bool)` | `jsonb` | all | stable, may return NULL |
| `jsonb.JsonbPathQueryTz` | `jsonb_path_query_tz(jsonb,jsonpath,jsonb,bool)` | `jsonb` | all | stable |
| `jsonb.JsonbPopulateRecord` | `jsonb_populate_record(anyelement,jsonb)` | `anyelement` | all | stable, may return NULL |
| `jsonb.JsonbPopulateRecordValid` | `jsonb_populate_record_valid(anyelement,jsonb)` | `bool` | 17+ | stable, may return NULL |
| `jsonb.JsonbPopulateRecordset` | `jsonb_populate_recordset(anyelement,jsonb)` | `anyelement` | all | stable |
| `jsonb.JsonbPretty` | `jsonb_pretty(jsonb)` | `text` | all | immutable |
| `jsonb.JsonbSend` | `jsonb_send(jsonb)` | `bytea` | all | immutable |
| `jsonb.JsonbSet` | `jsonb_set(jsonb,_text,jsonb,bool)` | `jsonb` | all | immutable |
| `jsonb.JsonbSetLax` | `jsonb_set_lax(jsonb,_text,jsonb,bool,text)` | `jsonb` | all | immutable, may return NULL |
| `jsonb.JsonbStripNullsJsonb` | `jsonb_strip_nulls(jsonb)` | `jsonb` | up to 17 | immutable |
| `jsonb.JsonbStripNullsJsonbBool` | `jsonb_strip_nulls(jsonb,bool)` | `jsonb` | 18+ | immutable |
| `jsonb.JsonbToTsvectorJsonbJsonb` | `jsonb_to_tsvector(jsonb,jsonb)` | `tsvector` | all | stable |
| `jsonb.JsonbToTsvectorRegconfigJsonbJsonb` | `jsonb_to_tsvector(regconfig,jsonb,jsonb)` | `tsvector` | all | immutable |
| `jsonb.JsonbTypeof` | `jsonb_typeof(jsonb)` | `text` | all | immutable |
| `jsonb.JsonpathSend` | `jsonpath_send(jsonpath)` | `bytea` | all | immutable |
| `datetime.JustifyDays` | `justify_days(interval)` | `interval` | all | immutable |
| `datetime.JustifyHours` | `justify_hours(interval)` | `interval` | all | immutable |
| `datetime.JustifyInterval` | `justify_interval(interval)` | `interval` | all | immutable |

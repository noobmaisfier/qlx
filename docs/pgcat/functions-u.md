# Functions U

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `text.UnicodeAssigned` | `unicode_assigned(text)` | `bool` | 17+ | immutable |
| `text.UnicodeVersion` | `unicode_version()` | `text` | 17+ | immutable |
| `text.Unistr` | `unistr(text)` | `text` | all | immutable |
| `array.Unnest` | `unnest(anyarray)` | `anyelement` | all | immutable |
| `ranges.Unnest` | `unnest(anymultirange)` | `anyrange` | all | immutable |
| `fts.Unnest` | `unnest(tsvector)` | `record` | all | immutable |
| `ranges.UpperAnymultirange` | `upper(anymultirange)` | `anyelement` | all | immutable, may return NULL |
| `ranges.UpperAnyrange` | `upper(anyrange)` | `anyelement` | all | immutable, may return NULL |
| `text.Upper` | `upper(text)` | `text` | all | immutable |
| `ranges.UpperIncAnymultirange` | `upper_inc(anymultirange)` | `bool` | all | immutable |
| `ranges.UpperIncAnyrange` | `upper_inc(anyrange)` | `bool` | all | immutable |
| `ranges.UpperInfAnymultirange` | `upper_inf(anymultirange)` | `bool` | all | immutable |
| `ranges.UpperInfAnyrange` | `upper_inf(anyrange)` | `bool` | all | immutable |
| `num.UuidCmp` | `uuid_cmp(uuid,uuid)` | `int4` | all | immutable |
| `datetime.UuidExtractTimestamp` | `uuid_extract_timestamp(uuid)` | `timestamptz` | 17+ | immutable |
| `num.UuidExtractVersion` | `uuid_extract_version(uuid)` | `int2` | 17+ | immutable |
| `num.UuidHash` | `uuid_hash(uuid)` | `int4` | all | immutable |
| `num.UuidHashExtended` | `uuid_hash_extended(uuid,int8)` | `int8` | all | immutable |
| `binary.UuidSend` | `uuid_send(uuid)` | `bytea` | all | immutable |
| `misc.Uuidv4` | `uuidv4()` | `uuid` | 18+ | volatile |
| `misc.Uuidv7` | `uuidv7()` | `uuid` | 18+ | volatile |
| `datetime.Uuidv7` | `uuidv7(interval)` | `uuid` | 18+ | volatile |

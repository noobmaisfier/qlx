# Functions G

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `num.Gamma` | `gamma(float8)` | `float8` | 18+ | immutable |
| `num.GcdInt4Int4` | `gcd(int4,int4)` | `int4` | all | immutable |
| `num.GcdInt8Int8` | `gcd(int8,int8)` | `int8` | all | immutable |
| `num.GcdNumericNumeric` | `gcd(numeric,numeric)` | `numeric` | all | immutable |
| `misc.GenRandomUuid` | `gen_random_uuid()` | `uuid` | all | volatile |
| `num.GenerateSeriesInt4Int4` | `generate_series(int4,int4)` | `int4` | all | immutable |
| `num.GenerateSeriesInt4Int4Int4` | `generate_series(int4,int4,int4)` | `int4` | all | immutable |
| `num.GenerateSeriesInt8Int8` | `generate_series(int8,int8)` | `int8` | all | immutable |
| `num.GenerateSeriesInt8Int8Int8` | `generate_series(int8,int8,int8)` | `int8` | all | immutable |
| `num.GenerateSeriesNumericNumeric` | `generate_series(numeric,numeric)` | `numeric` | all | immutable |
| `num.GenerateSeriesNumericNumericNumeric` | `generate_series(numeric,numeric,numeric)` | `numeric` | all | immutable |
| `datetime.GenerateSeriesTimestampTimestampInterval` | `generate_series(timestamp,timestamp,interval)` | `timestamp` | all | immutable |
| `datetime.GenerateSeriesTimestamptzTimestamptzInterval` | `generate_series(timestamptz,timestamptz,interval)` | `timestamptz` | all | stable |
| `datetime.GenerateSeriesTimestamptzTimestamptzIntervalText` | `generate_series(timestamptz,timestamptz,interval,text)` | `timestamptz` | all | immutable |
| `array.GenerateSubscriptsAnyarrayInt4` | `generate_subscripts(anyarray,int4)` | `int4` | all | immutable |
| `array.GenerateSubscriptsAnyarrayInt4Bool` | `generate_subscripts(anyarray,int4,bool)` | `int4` | all | immutable |
| `binary.GetBitBitInt4` | `get_bit(bit,int4)` | `int4` | all | immutable |
| `binary.GetBitByteaInt8` | `get_bit(bytea,int8)` | `int4` | all | immutable |
| `binary.GetByte` | `get_byte(bytea,int4)` | `int4` | all | immutable |
| `fts.GetCurrentTsConfig` | `get_current_ts_config()` | `regconfig` | all | stable |
| `sys.Getdatabaseencoding` | `getdatabaseencoding()` | `name` | all | stable |
| `sys.Getpgusername` | `getpgusername()` | `name` | all | stable |
| `sys.GinCleanPendingList` | `gin_clean_pending_list(regclass)` | `int8` | all | volatile |
| `text.GinCmpTslexeme` | `gin_cmp_tslexeme(text,text)` | `int4` | all | immutable |
| `text.GinCompareJsonb` | `gin_compare_jsonb(text,text)` | `int4` | all | immutable |
| `num.GistTranslateCmptypeCommon` | `gist_translate_cmptype_common(int4)` | `int2` | 18+ | immutable |

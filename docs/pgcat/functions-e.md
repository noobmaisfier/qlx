# Functions E

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `ranges.ElemContainedByMultirange` | `elem_contained_by_multirange(anyelement,anymultirange)` | `bool` | all | immutable |
| `ranges.ElemContainedByRange` | `elem_contained_by_range(anyelement,anyrange)` | `bool` | all | immutable |
| `text.Encode` | `encode(bytea,text)` | `text` | all | immutable |
| `num.EnumCmp` | `enum_cmp(anyenum,anyenum)` | `int4` | all | immutable |
| `misc.EnumEq` | `enum_eq(anyenum,anyenum)` | `bool` | all | immutable |
| `misc.EnumFirst` | `enum_first(anyenum)` | `anyenum` | all | stable, may return NULL |
| `misc.EnumGe` | `enum_ge(anyenum,anyenum)` | `bool` | all | immutable |
| `misc.EnumGt` | `enum_gt(anyenum,anyenum)` | `bool` | all | immutable |
| `misc.EnumLarger` | `enum_larger(anyenum,anyenum)` | `anyenum` | all | immutable |
| `misc.EnumLast` | `enum_last(anyenum)` | `anyenum` | all | stable, may return NULL |
| `misc.EnumLe` | `enum_le(anyenum,anyenum)` | `bool` | all | immutable |
| `misc.EnumLt` | `enum_lt(anyenum,anyenum)` | `bool` | all | immutable |
| `misc.EnumNe` | `enum_ne(anyenum,anyenum)` | `bool` | all | immutable |
| `array.EnumRangeAnyenum` | `enum_range(anyenum)` | `anyarray` | all | stable |
| `array.EnumRangeAnyenumAnyenum` | `enum_range(anyenum,anyenum)` | `anyarray` | all | stable |
| `binary.EnumSend` | `enum_send(anyenum)` | `bytea` | all | stable |
| `misc.EnumSmaller` | `enum_smaller(anyenum,anyenum)` | `anyenum` | all | immutable |
| `num.Erf` | `erf(float8)` | `float8` | all | immutable |
| `num.Erfc` | `erfc(float8)` | `float8` | all | immutable |
| `array.ErrorOnNull` | `error_on_null(anyelement)` | `anyelement` | 19+ | immutable, may return NULL |
| `misc.Every` | `every(bool)` | `bool` | all | immutable, may return NULL |
| `num.ExpFloat8` | `exp(float8)` | `float8` | all | immutable |
| `num.ExpNumeric` | `exp(numeric)` | `numeric` | all | immutable |
| `datetime.ExtractTextDate` | `extract(text,date)` | `numeric` | all | immutable |
| `datetime.ExtractTextInterval` | `extract(text,interval)` | `numeric` | all | immutable |
| `datetime.ExtractTextTime` | `extract(text,time)` | `numeric` | all | immutable |
| `datetime.ExtractTextTimestamp` | `extract(text,timestamp)` | `numeric` | all | immutable |
| `datetime.ExtractTextTimestamptz` | `extract(text,timestamptz)` | `numeric` | all | stable |
| `datetime.ExtractTextTimetz` | `extract(text,timetz)` | `numeric` | all | immutable |

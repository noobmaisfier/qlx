# Functions A

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `network.AbbrevCidr` | `abbrev(cidr)` | `text` | all | immutable |
| `network.AbbrevInet` | `abbrev(inet)` | `text` | all | immutable |
| `num.AbsFloat4` | `abs(float4)` | `float4` | all | immutable |
| `num.AbsFloat8` | `abs(float8)` | `float8` | all | immutable |
| `num.AbsInt2` | `abs(int2)` | `int2` | all | immutable |
| `num.AbsInt4` | `abs(int4)` | `int4` | all | immutable |
| `num.AbsInt8` | `abs(int8)` | `int8` | all | immutable |
| `num.AbsNumeric` | `abs(numeric)` | `numeric` | all | immutable |
| `sys.Aclcontains` | `aclcontains(_aclitem,aclitem)` | `bool` | all | immutable |
| `sys.Acldefault` | `acldefault(char,oid)` | `_aclitem` | all | immutable |
| `sys.Aclexplode` | `aclexplode(_aclitem)` | `record` | all | stable |
| `sys.Aclinsert` | `aclinsert(_aclitem,aclitem)` | `_aclitem` | all | immutable |
| `sys.Aclremove` | `aclremove(_aclitem,aclitem)` | `_aclitem` | all | immutable |
| `num.Acos` | `acos(float8)` | `float8` | all | immutable |
| `num.Acosd` | `acosd(float8)` | `float8` | all | immutable |
| `num.Acosh` | `acosh(float8)` | `float8` | all | immutable |
| `datetime.AgeTimestamp` | `age(timestamp)` | `interval` | all | stable |
| `datetime.AgeTimestampTimestamp` | `age(timestamp,timestamp)` | `interval` | all | immutable |
| `datetime.AgeTimestamptz` | `age(timestamptz)` | `interval` | all | stable |
| `datetime.AgeTimestamptzTimestamptz` | `age(timestamptz,timestamptz)` | `interval` | all | immutable |
| `sys.Age` | `age(xid)` | `int4` | all | stable |
| `sys.Amvalidate` | `amvalidate(oid)` | `bool` | all | volatile |
| `array.AnyValue` | `any_value(anyelement)` | `anyelement` | all | immutable, may return NULL |
| `array.AnyValueTransfn` | `any_value_transfn(anyelement,anyelement)` | `anyelement` | all | immutable |
| `array.AnyarraySend` | `anyarray_send(anyarray)` | `bytea` | all | stable |
| `array.AnycompatiblearraySend` | `anycompatiblearray_send(anycompatiblearray)` | `bytea` | all | stable |
| `geo.AreaBox` | `area(box)` | `float8` | all | immutable |
| `geo.AreaCircle` | `area(circle)` | `float8` | all | immutable |
| `geo.AreaPath` | `area(path)` | `float8` | all | immutable |
| `array.ArrayAggAnyarray` | `array_agg(anyarray)` | `anyarray` | all | immutable |
| `array.ArrayAggAnynonarray` | `array_agg(anynonarray)` | `anyarray` | all | immutable |
| `array.ArrayAppend` | `array_append(anycompatiblearray,anycompatible)` | `anycompatiblearray` | all | immutable |
| `array.ArrayCat` | `array_cat(anycompatiblearray,anycompatiblearray)` | `anycompatiblearray` | all | immutable |
| `array.ArrayDims` | `array_dims(anyarray)` | `text` | all | immutable, may return NULL |
| `array.ArrayEq` | `array_eq(anyarray,anyarray)` | `bool` | all | immutable |
| `array.ArrayFillAnyelementInt4Array` | `array_fill(anyelement,_int4)` | `anyarray` | all | immutable |
| `array.ArrayFillAnyelementInt4ArrayInt4Array` | `array_fill(anyelement,_int4,_int4)` | `anyarray` | all | immutable |
| `array.ArrayGe` | `array_ge(anyarray,anyarray)` | `bool` | all | immutable |
| `array.ArrayGt` | `array_gt(anyarray,anyarray)` | `bool` | all | immutable |
| `array.ArrayLarger` | `array_larger(anyarray,anyarray)` | `anyarray` | all | immutable |
| `array.ArrayLe` | `array_le(anyarray,anyarray)` | `bool` | all | immutable |
| `array.ArrayLength` | `array_length(anyarray,int4)` | `int4` | all | immutable, may return NULL |
| `array.ArrayLower` | `array_lower(anyarray,int4)` | `int4` | all | immutable, may return NULL |
| `array.ArrayLt` | `array_lt(anyarray,anyarray)` | `bool` | all | immutable |
| `array.ArrayNdims` | `array_ndims(anyarray)` | `int4` | all | immutable, may return NULL |
| `array.ArrayNe` | `array_ne(anyarray,anyarray)` | `bool` | all | immutable |
| `array.ArrayPositionAnycompatiblearrayAnycompatible` | `array_position(anycompatiblearray,anycompatible)` | `int4` | all | immutable, may return NULL |
| `array.ArrayPositionAnycompatiblearrayAnycompatibleInt4` | `array_position(anycompatiblearray,anycompatible,int4)` | `int4` | all | immutable, may return NULL |
| `array.ArrayPositions` | `array_positions(anycompatiblearray,anycompatible)` | `_int4` | all | immutable |
| `array.ArrayPrepend` | `array_prepend(anycompatible,anycompatiblearray)` | `anycompatiblearray` | all | immutable |
| `array.ArrayRemove` | `array_remove(anycompatiblearray,anycompatible)` | `anycompatiblearray` | all | immutable |
| `array.ArrayReplace` | `array_replace(anycompatiblearray,anycompatible,anycompatible)` | `anycompatiblearray` | all | immutable |
| `array.ArrayReverse` | `array_reverse(anyarray)` | `anyarray` | 18+ | immutable |
| `array.ArraySample` | `array_sample(anyarray,int4)` | `anyarray` | all | volatile |
| `array.ArraySend` | `array_send(anyarray)` | `bytea` | all | stable |
| `array.ArrayShuffle` | `array_shuffle(anyarray)` | `anyarray` | all | volatile |
| `array.ArraySmaller` | `array_smaller(anyarray,anyarray)` | `anyarray` | all | immutable |
| `array.ArraySortAnyarray` | `array_sort(anyarray)` | `anyarray` | 18+ | immutable |
| `array.ArraySortAnyarrayBool` | `array_sort(anyarray,bool)` | `anyarray` | 18+ | immutable |
| `array.ArraySortAnyarrayBoolBool` | `array_sort(anyarray,bool,bool)` | `anyarray` | 18+ | immutable |
| `jsonb.ArrayToJsonAnyarray` | `array_to_json(anyarray)` | `json` | all | stable |
| `jsonb.ArrayToJsonAnyarrayBool` | `array_to_json(anyarray,bool)` | `json` | all | stable |
| `array.ArrayToStringAnyarrayText` | `array_to_string(anyarray,text)` | `text` | all | stable |
| `array.ArrayToStringAnyarrayTextText` | `array_to_string(anyarray,text,text)` | `text` | all | stable, may return NULL |
| `fts.ArrayToTsvector` | `array_to_tsvector(_text)` | `tsvector` | all | immutable |
| `array.ArrayUpper` | `array_upper(anyarray,int4)` | `int4` | all | immutable, may return NULL |
| `array.Arraycontained` | `arraycontained(anyarray,anyarray)` | `bool` | all | immutable |
| `array.Arraycontains` | `arraycontains(anyarray,anyarray)` | `bool` | all | immutable |
| `array.Arrayoverlap` | `arrayoverlap(anyarray,anyarray)` | `bool` | all | immutable |
| `text.Ascii` | `ascii(text)` | `int4` | all | immutable |
| `num.Asin` | `asin(float8)` | `float8` | all | immutable |
| `num.Asind` | `asind(float8)` | `float8` | all | immutable |
| `num.Asinh` | `asinh(float8)` | `float8` | all | immutable |
| `num.Atan` | `atan(float8)` | `float8` | all | immutable |
| `num.Atan2` | `atan2(float8,float8)` | `float8` | all | immutable |
| `num.Atan2d` | `atan2d(float8,float8)` | `float8` | all | immutable |
| `num.Atand` | `atand(float8)` | `float8` | all | immutable |
| `num.Atanh` | `atanh(float8)` | `float8` | all | immutable |
| `num.AvgFloat4` | `avg(float4)` | `float8` | all | immutable, may return NULL |
| `num.AvgFloat8` | `avg(float8)` | `float8` | all | immutable, may return NULL |
| `num.AvgInt2` | `avg(int2)` | `numeric` | all | immutable, may return NULL |
| `num.AvgInt4` | `avg(int4)` | `numeric` | all | immutable, may return NULL |
| `num.AvgInt8` | `avg(int8)` | `numeric` | all | immutable, may return NULL |
| `datetime.Avg` | `avg(interval)` | `interval` | all | immutable, may return NULL |
| `num.AvgNumeric` | `avg(numeric)` | `numeric` | all | immutable, may return NULL |

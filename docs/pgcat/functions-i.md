# Functions I

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `text.IcuUnicodeVersion` | `icu_unicode_version()` | `text` | 17+ | immutable |
| `datetime.InRangeDateDateIntervalBoolBool` | `in_range(date,date,interval,bool,bool)` | `bool` | all | immutable |
| `num.InRangeFloat4Float4Float8BoolBool` | `in_range(float4,float4,float8,bool,bool)` | `bool` | all | immutable |
| `num.InRangeFloat8Float8Float8BoolBool` | `in_range(float8,float8,float8,bool,bool)` | `bool` | all | immutable |
| `num.InRangeInt2Int2Int2BoolBool` | `in_range(int2,int2,int2,bool,bool)` | `bool` | all | immutable |
| `num.InRangeInt2Int2Int4BoolBool` | `in_range(int2,int2,int4,bool,bool)` | `bool` | all | immutable |
| `num.InRangeInt2Int2Int8BoolBool` | `in_range(int2,int2,int8,bool,bool)` | `bool` | all | immutable |
| `num.InRangeInt4Int4Int2BoolBool` | `in_range(int4,int4,int2,bool,bool)` | `bool` | all | immutable |
| `num.InRangeInt4Int4Int4BoolBool` | `in_range(int4,int4,int4,bool,bool)` | `bool` | all | immutable |
| `num.InRangeInt4Int4Int8BoolBool` | `in_range(int4,int4,int8,bool,bool)` | `bool` | all | immutable |
| `num.InRangeInt8Int8Int8BoolBool` | `in_range(int8,int8,int8,bool,bool)` | `bool` | all | immutable |
| `datetime.InRangeIntervalIntervalIntervalBoolBool` | `in_range(interval,interval,interval,bool,bool)` | `bool` | all | immutable |
| `num.InRangeNumericNumericNumericBoolBool` | `in_range(numeric,numeric,numeric,bool,bool)` | `bool` | all | immutable |
| `datetime.InRangeTimeTimeIntervalBoolBool` | `in_range(time,time,interval,bool,bool)` | `bool` | all | immutable |
| `datetime.InRangeTimestampTimestampIntervalBoolBool` | `in_range(timestamp,timestamp,interval,bool,bool)` | `bool` | all | immutable |
| `datetime.InRangeTimestamptzTimestamptzIntervalBoolBool` | `in_range(timestamptz,timestamptz,interval,bool,bool)` | `bool` | all | stable |
| `datetime.InRangeTimetzTimetzIntervalBoolBool` | `in_range(timetz,timetz,interval,bool,bool)` | `bool` | all | immutable |
| `network.InetClientAddr` | `inet_client_addr()` | `inet` | all | stable, may return NULL |
| `num.InetClientPort` | `inet_client_port()` | `int4` | all | stable, may return NULL |
| `network.InetMerge` | `inet_merge(inet,inet)` | `cidr` | all | immutable |
| `network.InetSameFamily` | `inet_same_family(inet,inet)` | `bool` | all | immutable |
| `network.InetSend` | `inet_send(inet)` | `bytea` | all | immutable |
| `network.InetServerAddr` | `inet_server_addr()` | `inet` | all | stable, may return NULL |
| `num.InetServerPort` | `inet_server_port()` | `int4` | all | stable, may return NULL |
| `text.Initcap` | `initcap(text)` | `text` | all | immutable |
| `array.Int2AvgAccum` | `int2_avg_accum(_int8,int2)` | `_int8` | all | immutable |
| `array.Int2AvgAccumInv` | `int2_avg_accum_inv(_int8,int2)` | `_int8` | all | immutable |
| `num.Int2Sum` | `int2_sum(int8,int2)` | `int8` | all | immutable, may return NULL |
| `array.Int2int4Sum` | `int2int4_sum(_int8)` | `int8` | all | immutable |
| `num.Int2larger` | `int2larger(int2,int2)` | `int2` | all | immutable |
| `num.Int2smaller` | `int2smaller(int2,int2)` | `int2` | all | immutable |
| `array.Int4AvgAccum` | `int4_avg_accum(_int8,int4)` | `_int8` | all | immutable |
| `array.Int4AvgAccumInv` | `int4_avg_accum_inv(_int8,int4)` | `_int8` | all | immutable |
| `array.Int4AvgCombine` | `int4_avg_combine(_int8,_int8)` | `_int8` | all | immutable |
| `num.Int4Sum` | `int4_sum(int8,int4)` | `int8` | all | immutable, may return NULL |
| `num.Int4inc` | `int4inc(int4)` | `int4` | all | immutable |
| `num.Int4larger` | `int4larger(int4,int4)` | `int4` | all | immutable |
| `ranges.Int4rangeInt4Int4` | `int4range(int4,int4)` | `int4range` | all | immutable, may return NULL |
| `ranges.Int4rangeInt4Int4Text` | `int4range(int4,int4,text)` | `int4range` | all | immutable, may return NULL |
| `ranges.Int4rangeCanonical` | `int4range_canonical(int4range)` | `int4range` | all | immutable |
| `num.Int4rangeSubdiff` | `int4range_subdiff(int4,int4)` | `float8` | all | immutable |
| `num.Int4smaller` | `int4smaller(int4,int4)` | `int4` | all | immutable |
| `array.Int8Avg` | `int8_avg(_int8)` | `numeric` | all | immutable |
| `num.Int8Sum` | `int8_sum(numeric,int8)` | `numeric` | all | immutable, may return NULL |
| `num.Int8dec` | `int8dec(int8)` | `int8` | all | immutable |
| `num.Int8decAny` | `int8dec_any(int8,any)` | `int8` | all | immutable |
| `num.Int8inc` | `int8inc(int8)` | `int8` | all | immutable |
| `num.Int8incAny` | `int8inc_any(int8,any)` | `int8` | all | immutable |
| `num.Int8incFloat8Float8` | `int8inc_float8_float8(int8,float8,float8)` | `int8` | all | immutable |
| `num.Int8larger` | `int8larger(int8,int8)` | `int8` | all | immutable |
| `ranges.Int8rangeInt8Int8` | `int8range(int8,int8)` | `int8range` | all | immutable, may return NULL |
| `ranges.Int8rangeInt8Int8Text` | `int8range(int8,int8,text)` | `int8range` | all | immutable, may return NULL |
| `ranges.Int8rangeCanonical` | `int8range_canonical(int8range)` | `int8range` | all | immutable |
| `num.Int8rangeSubdiff` | `int8range_subdiff(int8,int8)` | `float8` | all | immutable |
| `num.Int8smaller` | `int8smaller(int8,int8)` | `int8` | all | immutable |
| `datetime.IntegerPlDate` | `integer_pl_date(int4,date)` | `date` | all | immutable |
| `geo.InterLb` | `inter_lb(line,box)` | `bool` | all | immutable |
| `geo.InterSb` | `inter_sb(lseg,box)` | `bool` | all | immutable |
| `geo.InterSl` | `inter_sl(lseg,line)` | `bool` | all | immutable |
| `array.IntervalAccum` | `interval_accum(_interval,interval)` | `_interval` | up to 16 | immutable |
| `array.IntervalAccumInv` | `interval_accum_inv(_interval,interval)` | `_interval` | up to 16 | immutable |
| `array.IntervalAvg` | `interval_avg(_interval)` | `interval` | up to 16 | immutable |
| `datetime.IntervalCmp` | `interval_cmp(interval,interval)` | `int4` | all | immutable |
| `array.IntervalCombine` | `interval_combine(_interval,_interval)` | `_interval` | up to 16 | immutable |
| `datetime.IntervalHash` | `interval_hash(interval)` | `int4` | all | immutable |
| `datetime.IntervalHashExtended` | `interval_hash_extended(interval,int8)` | `int8` | all | immutable |
| `datetime.IntervalLarger` | `interval_larger(interval,interval)` | `interval` | all | immutable |
| `datetime.IntervalSend` | `interval_send(interval)` | `bytea` | all | immutable |
| `datetime.IntervalSmaller` | `interval_smaller(interval,interval)` | `interval` | all | immutable |
| `text.IsNormalized` | `is_normalized(text,text)` | `bool` | all | immutable |
| `geo.Isclosed` | `isclosed(path)` | `bool` | all | immutable |
| `ranges.IsemptyAnymultirange` | `isempty(anymultirange)` | `bool` | all | immutable |
| `ranges.IsemptyAnyrange` | `isempty(anyrange)` | `bool` | all | immutable |
| `datetime.IsfiniteDate` | `isfinite(date)` | `bool` | all | immutable |
| `datetime.IsfiniteInterval` | `isfinite(interval)` | `bool` | all | immutable |
| `datetime.IsfiniteTimestamp` | `isfinite(timestamp)` | `bool` | all | immutable |
| `datetime.IsfiniteTimestamptz` | `isfinite(timestamptz)` | `bool` | all | immutable |
| `geo.IshorizontalLine` | `ishorizontal(line)` | `bool` | all | immutable |
| `geo.IshorizontalLseg` | `ishorizontal(lseg)` | `bool` | all | immutable |
| `geo.IshorizontalPointPoint` | `ishorizontal(point,point)` | `bool` | all | immutable |
| `geo.Isopen` | `isopen(path)` | `bool` | all | immutable |
| `geo.IsparallelLineLine` | `isparallel(line,line)` | `bool` | all | immutable |
| `geo.IsparallelLsegLseg` | `isparallel(lseg,lseg)` | `bool` | all | immutable |
| `geo.IsperpLineLine` | `isperp(line,line)` | `bool` | all | immutable |
| `geo.IsperpLsegLseg` | `isperp(lseg,lseg)` | `bool` | all | immutable |
| `geo.IsverticalLine` | `isvertical(line)` | `bool` | all | immutable |
| `geo.IsverticalLseg` | `isvertical(lseg)` | `bool` | all | immutable |
| `geo.IsverticalPointPoint` | `isvertical(point,point)` | `bool` | all | immutable |

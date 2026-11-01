# Functions M

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `network.Macaddr8Cmp` | `macaddr8_cmp(macaddr8,macaddr8)` | `int4` | all | immutable |
| `network.Macaddr8Send` | `macaddr8_send(macaddr8)` | `bytea` | all | immutable |
| `network.Macaddr8Set7bit` | `macaddr8_set7bit(macaddr8)` | `macaddr8` | all | immutable |
| `network.MacaddrCmp` | `macaddr_cmp(macaddr,macaddr)` | `int4` | all | immutable |
| `network.MacaddrSend` | `macaddr_send(macaddr)` | `bytea` | all | immutable |
| `datetime.MakeDate` | `make_date(int4,int4,int4)` | `date` | all | immutable |
| `datetime.MakeInterval` | `make_interval(int4,int4,int4,int4,int4,int4,float8)` | `interval` | all | immutable |
| `datetime.MakeTime` | `make_time(int4,int4,float8)` | `time` | all | immutable |
| `datetime.MakeTimestamp` | `make_timestamp(int4,int4,int4,int4,int4,float8)` | `timestamp` | all | immutable |
| `datetime.MakeTimestamptzInt4Int4Int4Int4Int4Float8` | `make_timestamptz(int4,int4,int4,int4,int4,float8)` | `timestamptz` | all | stable |
| `datetime.MakeTimestamptzInt4Int4Int4Int4Int4Float8Text` | `make_timestamptz(int4,int4,int4,int4,int4,float8,text)` | `timestamptz` | all | stable |
| `sys.Makeaclitem` | `makeaclitem(oid,oid,text,bool)` | `aclitem` | all | immutable |
| `network.Masklen` | `masklen(inet)` | `int4` | all | immutable |
| `array.Max` | `max(anyarray)` | `anyarray` | all | immutable |
| `misc.Max` | `max(anyenum)` | `anyenum` | all | immutable, may return NULL |
| `text.MaxBpchar` | `max(bpchar)` | `bpchar` | all | immutable, may return NULL |
| `binary.Max` | `max(bytea)` | `bytea` | 18+ | immutable |
| `datetime.MaxDate` | `max(date)` | `date` | all | immutable, may return NULL |
| `num.MaxFloat4` | `max(float4)` | `float4` | all | immutable, may return NULL |
| `num.MaxFloat8` | `max(float8)` | `float8` | all | immutable, may return NULL |
| `network.Max` | `max(inet)` | `inet` | all | immutable, may return NULL |
| `num.MaxInt2` | `max(int2)` | `int2` | all | immutable, may return NULL |
| `num.MaxInt4` | `max(int4)` | `int4` | all | immutable, may return NULL |
| `num.MaxInt8` | `max(int8)` | `int8` | all | immutable, may return NULL |
| `datetime.MaxInterval` | `max(interval)` | `interval` | all | immutable, may return NULL |
| `num.MaxMoney` | `max(money)` | `money` | all | immutable, may return NULL |
| `num.MaxNumeric` | `max(numeric)` | `numeric` | all | immutable, may return NULL |
| `sys.MaxOid` | `max(oid)` | `oid` | all | immutable, may return NULL |
| `sys.MaxOid8` | `max(oid8)` | `oid8` | 19+ | immutable, may return NULL |
| `sys.MaxPgLsn` | `max(pg_lsn)` | `pg_lsn` | all | immutable, may return NULL |
| `text.MaxText` | `max(text)` | `text` | all | immutable, may return NULL |
| `sys.MaxTid` | `max(tid)` | `tid` | all | immutable, may return NULL |
| `datetime.MaxTime` | `max(time)` | `time` | all | immutable, may return NULL |
| `datetime.MaxTimestamp` | `max(timestamp)` | `timestamp` | all | immutable, may return NULL |
| `datetime.MaxTimestamptz` | `max(timestamptz)` | `timestamptz` | all | immutable, may return NULL |
| `datetime.MaxTimetz` | `max(timetz)` | `timetz` | all | immutable, may return NULL |
| `sys.MaxXid8` | `max(xid8)` | `xid8` | all | immutable, may return NULL |
| `text.Md5Bytea` | `md5(bytea)` | `text` | all | immutable |
| `text.Md5Text` | `md5(text)` | `text` | all | immutable |
| `array.Min` | `min(anyarray)` | `anyarray` | all | immutable |
| `misc.Min` | `min(anyenum)` | `anyenum` | all | immutable, may return NULL |
| `text.MinBpchar` | `min(bpchar)` | `bpchar` | all | immutable, may return NULL |
| `binary.Min` | `min(bytea)` | `bytea` | 18+ | immutable |
| `datetime.MinDate` | `min(date)` | `date` | all | immutable, may return NULL |
| `num.MinFloat4` | `min(float4)` | `float4` | all | immutable, may return NULL |
| `num.MinFloat8` | `min(float8)` | `float8` | all | immutable, may return NULL |
| `network.Min` | `min(inet)` | `inet` | all | immutable, may return NULL |
| `num.MinInt2` | `min(int2)` | `int2` | all | immutable, may return NULL |
| `num.MinInt4` | `min(int4)` | `int4` | all | immutable, may return NULL |
| `num.MinInt8` | `min(int8)` | `int8` | all | immutable, may return NULL |
| `datetime.MinInterval` | `min(interval)` | `interval` | all | immutable, may return NULL |
| `num.MinMoney` | `min(money)` | `money` | all | immutable, may return NULL |
| `num.MinNumeric` | `min(numeric)` | `numeric` | all | immutable, may return NULL |
| `sys.MinOid` | `min(oid)` | `oid` | all | immutable, may return NULL |
| `sys.MinOid8` | `min(oid8)` | `oid8` | 19+ | immutable, may return NULL |
| `sys.MinPgLsn` | `min(pg_lsn)` | `pg_lsn` | all | immutable, may return NULL |
| `text.MinText` | `min(text)` | `text` | all | immutable, may return NULL |
| `sys.MinTid` | `min(tid)` | `tid` | all | immutable, may return NULL |
| `datetime.MinTime` | `min(time)` | `time` | all | immutable, may return NULL |
| `datetime.MinTimestamp` | `min(timestamp)` | `timestamp` | all | immutable, may return NULL |
| `datetime.MinTimestamptz` | `min(timestamptz)` | `timestamptz` | all | immutable, may return NULL |
| `datetime.MinTimetz` | `min(timetz)` | `timetz` | all | immutable, may return NULL |
| `sys.MinXid8` | `min(xid8)` | `xid8` | all | immutable, may return NULL |
| `num.MinScale` | `min_scale(numeric)` | `int4` | all | immutable |
| `num.ModInt2Int2` | `mod(int2,int2)` | `int2` | all | immutable |
| `num.ModInt4Int4` | `mod(int4,int4)` | `int4` | all | immutable |
| `num.ModInt8Int8` | `mod(int8,int8)` | `int8` | all | immutable |
| `num.ModNumericNumeric` | `mod(numeric,numeric)` | `numeric` | all | immutable |
| `datetime.MulDInterval` | `mul_d_interval(float8,interval)` | `interval` | all | immutable |
| `ranges.Multirange` | `multirange(anyrange)` | `anymultirange` | all | immutable |
| `ranges.MultirangeAdjacentMultirange` | `multirange_adjacent_multirange(anymultirange,anymultirange)` | `bool` | all | immutable |
| `ranges.MultirangeAdjacentRange` | `multirange_adjacent_range(anymultirange,anyrange)` | `bool` | all | immutable |
| `ranges.MultirangeAfterMultirange` | `multirange_after_multirange(anymultirange,anymultirange)` | `bool` | all | immutable |
| `ranges.MultirangeAfterRange` | `multirange_after_range(anymultirange,anyrange)` | `bool` | all | immutable |
| `ranges.MultirangeBeforeMultirange` | `multirange_before_multirange(anymultirange,anymultirange)` | `bool` | all | immutable |
| `ranges.MultirangeBeforeRange` | `multirange_before_range(anymultirange,anyrange)` | `bool` | all | immutable |
| `ranges.MultirangeCmp` | `multirange_cmp(anymultirange,anymultirange)` | `int4` | all | immutable |
| `ranges.MultirangeContainedByMultirange` | `multirange_contained_by_multirange(anymultirange,anymultirange)` | `bool` | all | immutable |
| `ranges.MultirangeContainedByRange` | `multirange_contained_by_range(anymultirange,anyrange)` | `bool` | all | immutable |
| `ranges.MultirangeContainsElem` | `multirange_contains_elem(anymultirange,anyelement)` | `bool` | all | immutable |
| `ranges.MultirangeContainsMultirange` | `multirange_contains_multirange(anymultirange,anymultirange)` | `bool` | all | immutable |
| `ranges.MultirangeContainsRange` | `multirange_contains_range(anymultirange,anyrange)` | `bool` | all | immutable |
| `ranges.MultirangeEq` | `multirange_eq(anymultirange,anymultirange)` | `bool` | all | immutable |
| `ranges.MultirangeGe` | `multirange_ge(anymultirange,anymultirange)` | `bool` | all | immutable |
| `ranges.MultirangeGt` | `multirange_gt(anymultirange,anymultirange)` | `bool` | all | immutable |
| `ranges.MultirangeIntersect` | `multirange_intersect(anymultirange,anymultirange)` | `anymultirange` | all | immutable |
| `ranges.MultirangeIntersectAggTransfn` | `multirange_intersect_agg_transfn(anymultirange,anymultirange)` | `anymultirange` | all | immutable |
| `ranges.MultirangeLe` | `multirange_le(anymultirange,anymultirange)` | `bool` | all | immutable |
| `ranges.MultirangeLt` | `multirange_lt(anymultirange,anymultirange)` | `bool` | all | immutable |
| `ranges.MultirangeMinus` | `multirange_minus(anymultirange,anymultirange)` | `anymultirange` | all | immutable |
| `ranges.MultirangeMinusMulti` | `multirange_minus_multi(anymultirange,anymultirange)` | `anymultirange` | 19+ | immutable |
| `ranges.MultirangeNe` | `multirange_ne(anymultirange,anymultirange)` | `bool` | all | immutable |
| `ranges.MultirangeOverlapsMultirange` | `multirange_overlaps_multirange(anymultirange,anymultirange)` | `bool` | all | immutable |
| `ranges.MultirangeOverlapsRange` | `multirange_overlaps_range(anymultirange,anyrange)` | `bool` | all | immutable |
| `ranges.MultirangeOverleftMultirange` | `multirange_overleft_multirange(anymultirange,anymultirange)` | `bool` | all | immutable |
| `ranges.MultirangeOverleftRange` | `multirange_overleft_range(anymultirange,anyrange)` | `bool` | all | immutable |
| `ranges.MultirangeOverrightMultirange` | `multirange_overright_multirange(anymultirange,anymultirange)` | `bool` | all | immutable |
| `ranges.MultirangeOverrightRange` | `multirange_overright_range(anymultirange,anyrange)` | `bool` | all | immutable |
| `ranges.MultirangeSend` | `multirange_send(anymultirange)` | `bytea` | all | stable |
| `ranges.MultirangeUnion` | `multirange_union(anymultirange,anymultirange)` | `anymultirange` | all | immutable |
| `sys.MxidAge` | `mxid_age(xid)` | `int4` | all | stable |

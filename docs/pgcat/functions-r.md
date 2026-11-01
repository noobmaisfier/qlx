# Functions R

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `num.Radians` | `radians(float8)` | `float8` | all | immutable |
| `geo.Radius` | `radius(circle)` | `float8` | all | immutable |
| `num.Random` | `random()` | `float8` | all | volatile |
| `datetime.RandomDateDate` | `random(date,date)` | `date` | 19+ | volatile |
| `num.RandomInt4Int4` | `random(int4,int4)` | `int4` | 17+ | volatile |
| `num.RandomInt8Int8` | `random(int8,int8)` | `int8` | 17+ | volatile |
| `num.RandomNumericNumeric` | `random(numeric,numeric)` | `numeric` | 17+ | volatile |
| `datetime.RandomTimestampTimestamp` | `random(timestamp,timestamp)` | `timestamp` | 19+ | volatile |
| `datetime.RandomTimestamptzTimestamptz` | `random(timestamptz,timestamptz)` | `timestamptz` | 19+ | volatile |
| `num.RandomNormal` | `random_normal(float8,float8)` | `float8` | all | volatile |
| `ranges.RangeAdjacent` | `range_adjacent(anyrange,anyrange)` | `bool` | all | immutable |
| `ranges.RangeAdjacentMultirange` | `range_adjacent_multirange(anyrange,anymultirange)` | `bool` | all | immutable |
| `ranges.RangeAfter` | `range_after(anyrange,anyrange)` | `bool` | all | immutable |
| `ranges.RangeAfterMultirange` | `range_after_multirange(anyrange,anymultirange)` | `bool` | all | immutable |
| `ranges.RangeAggAnymultirange` | `range_agg(anymultirange)` | `anymultirange` | all | immutable |
| `ranges.RangeAggAnyrange` | `range_agg(anyrange)` | `anymultirange` | all | immutable |
| `ranges.RangeBefore` | `range_before(anyrange,anyrange)` | `bool` | all | immutable |
| `ranges.RangeBeforeMultirange` | `range_before_multirange(anyrange,anymultirange)` | `bool` | all | immutable |
| `ranges.RangeCmp` | `range_cmp(anyrange,anyrange)` | `int4` | all | immutable |
| `ranges.RangeContainedBy` | `range_contained_by(anyrange,anyrange)` | `bool` | all | immutable |
| `ranges.RangeContainedByMultirange` | `range_contained_by_multirange(anyrange,anymultirange)` | `bool` | all | immutable |
| `ranges.RangeContains` | `range_contains(anyrange,anyrange)` | `bool` | all | immutable |
| `ranges.RangeContainsElem` | `range_contains_elem(anyrange,anyelement)` | `bool` | all | immutable |
| `ranges.RangeContainsMultirange` | `range_contains_multirange(anyrange,anymultirange)` | `bool` | all | immutable |
| `ranges.RangeEq` | `range_eq(anyrange,anyrange)` | `bool` | all | immutable |
| `ranges.RangeGe` | `range_ge(anyrange,anyrange)` | `bool` | all | immutable |
| `ranges.RangeGt` | `range_gt(anyrange,anyrange)` | `bool` | all | immutable |
| `ranges.RangeIntersect` | `range_intersect(anyrange,anyrange)` | `anyrange` | all | immutable |
| `ranges.RangeIntersectAggAnymultirange` | `range_intersect_agg(anymultirange)` | `anymultirange` | all | immutable |
| `ranges.RangeIntersectAggAnyrange` | `range_intersect_agg(anyrange)` | `anyrange` | all | immutable |
| `ranges.RangeIntersectAggTransfn` | `range_intersect_agg_transfn(anyrange,anyrange)` | `anyrange` | all | immutable |
| `ranges.RangeLe` | `range_le(anyrange,anyrange)` | `bool` | all | immutable |
| `ranges.RangeLt` | `range_lt(anyrange,anyrange)` | `bool` | all | immutable |
| `ranges.RangeMergeAnymultirange` | `range_merge(anymultirange)` | `anyrange` | all | immutable |
| `ranges.RangeMergeAnyrangeAnyrange` | `range_merge(anyrange,anyrange)` | `anyrange` | all | immutable |
| `ranges.RangeMinus` | `range_minus(anyrange,anyrange)` | `anyrange` | all | immutable |
| `ranges.RangeMinusMulti` | `range_minus_multi(anyrange,anyrange)` | `anyrange` | 19+ | immutable |
| `ranges.RangeNe` | `range_ne(anyrange,anyrange)` | `bool` | all | immutable |
| `ranges.RangeOverlaps` | `range_overlaps(anyrange,anyrange)` | `bool` | all | immutable |
| `ranges.RangeOverlapsMultirange` | `range_overlaps_multirange(anyrange,anymultirange)` | `bool` | all | immutable |
| `ranges.RangeOverleft` | `range_overleft(anyrange,anyrange)` | `bool` | all | immutable |
| `ranges.RangeOverleftMultirange` | `range_overleft_multirange(anyrange,anymultirange)` | `bool` | all | immutable |
| `ranges.RangeOverright` | `range_overright(anyrange,anyrange)` | `bool` | all | immutable |
| `ranges.RangeOverrightMultirange` | `range_overright_multirange(anyrange,anymultirange)` | `bool` | all | immutable |
| `ranges.RangeSend` | `range_send(anyrange)` | `bytea` | all | stable |
| `ranges.RangeUnion` | `range_union(anyrange,anyrange)` | `anyrange` | all | immutable |
| `text.RegexpCountTextText` | `regexp_count(text,text)` | `int4` | all | immutable |
| `text.RegexpCountTextTextInt4` | `regexp_count(text,text,int4)` | `int4` | all | immutable |
| `text.RegexpCountTextTextInt4Text` | `regexp_count(text,text,int4,text)` | `int4` | all | immutable |
| `text.RegexpInstrTextText` | `regexp_instr(text,text)` | `int4` | all | immutable |
| `text.RegexpInstrTextTextInt4` | `regexp_instr(text,text,int4)` | `int4` | all | immutable |
| `text.RegexpInstrTextTextInt4Int4` | `regexp_instr(text,text,int4,int4)` | `int4` | all | immutable |
| `text.RegexpInstrTextTextInt4Int4Int4` | `regexp_instr(text,text,int4,int4,int4)` | `int4` | all | immutable |
| `text.RegexpInstrTextTextInt4Int4Int4Text` | `regexp_instr(text,text,int4,int4,int4,text)` | `int4` | all | immutable |
| `text.RegexpInstrTextTextInt4Int4Int4TextInt4` | `regexp_instr(text,text,int4,int4,int4,text,int4)` | `int4` | all | immutable |
| `text.RegexpLikeTextText` | `regexp_like(text,text)` | `bool` | all | immutable |
| `text.RegexpLikeTextTextText` | `regexp_like(text,text,text)` | `bool` | all | immutable |
| `array.RegexpMatchTextText` | `regexp_match(text,text)` | `_text` | all | immutable |
| `array.RegexpMatchTextTextText` | `regexp_match(text,text,text)` | `_text` | all | immutable |
| `array.RegexpMatchesTextText` | `regexp_matches(text,text)` | `_text` | all | immutable |
| `array.RegexpMatchesTextTextText` | `regexp_matches(text,text,text)` | `_text` | all | immutable |
| `text.RegexpReplaceTextTextText` | `regexp_replace(text,text,text)` | `text` | all | immutable |
| `text.RegexpReplaceTextTextTextInt4` | `regexp_replace(text,text,text,int4)` | `text` | all | immutable |
| `text.RegexpReplaceTextTextTextInt4Int4` | `regexp_replace(text,text,text,int4,int4)` | `text` | all | immutable |
| `text.RegexpReplaceTextTextTextInt4Int4Text` | `regexp_replace(text,text,text,int4,int4,text)` | `text` | all | immutable |
| `text.RegexpReplaceTextTextTextText` | `regexp_replace(text,text,text,text)` | `text` | all | immutable |
| `array.RegexpSplitToArrayTextText` | `regexp_split_to_array(text,text)` | `_text` | all | immutable |
| `array.RegexpSplitToArrayTextTextText` | `regexp_split_to_array(text,text,text)` | `_text` | all | immutable |
| `text.RegexpSplitToTableTextText` | `regexp_split_to_table(text,text)` | `text` | all | immutable |
| `text.RegexpSplitToTableTextTextText` | `regexp_split_to_table(text,text,text)` | `text` | all | immutable |
| `text.RegexpSubstrTextText` | `regexp_substr(text,text)` | `text` | all | immutable, may return NULL |
| `text.RegexpSubstrTextTextInt4` | `regexp_substr(text,text,int4)` | `text` | all | immutable, may return NULL |
| `text.RegexpSubstrTextTextInt4Int4` | `regexp_substr(text,text,int4,int4)` | `text` | all | immutable, may return NULL |
| `text.RegexpSubstrTextTextInt4Int4Text` | `regexp_substr(text,text,int4,int4,text)` | `text` | all | immutable, may return NULL |
| `text.RegexpSubstrTextTextInt4Int4TextInt4` | `regexp_substr(text,text,int4,int4,text,int4)` | `text` | all | immutable, may return NULL |
| `num.RegrAvgx` | `regr_avgx(float8,float8)` | `float8` | all | immutable, may return NULL |
| `num.RegrAvgy` | `regr_avgy(float8,float8)` | `float8` | all | immutable, may return NULL |
| `num.RegrCount` | `regr_count(float8,float8)` | `int8` | all | immutable |
| `num.RegrIntercept` | `regr_intercept(float8,float8)` | `float8` | all | immutable, may return NULL |
| `num.RegrR2` | `regr_r2(float8,float8)` | `float8` | all | immutable, may return NULL |
| `num.RegrSlope` | `regr_slope(float8,float8)` | `float8` | all | immutable, may return NULL |
| `num.RegrSxx` | `regr_sxx(float8,float8)` | `float8` | all | immutable, may return NULL |
| `num.RegrSxy` | `regr_sxy(float8,float8)` | `float8` | all | immutable, may return NULL |
| `num.RegrSyy` | `regr_syy(float8,float8)` | `float8` | all | immutable, may return NULL |
| `text.Repeat` | `repeat(text,int4)` | `text` | all | immutable |
| `text.Replace` | `replace(text,text,text)` | `text` | all | immutable |
| `binary.Reverse` | `reverse(bytea)` | `bytea` | 18+ | immutable |
| `text.Reverse` | `reverse(text)` | `text` | all | immutable |
| `text.Right` | `right(text,int4)` | `text` | all | immutable |
| `num.RoundFloat8` | `round(float8)` | `float8` | all | immutable |
| `num.RoundNumeric` | `round(numeric)` | `numeric` | all | immutable |
| `num.RoundNumericInt4` | `round(numeric,int4)` | `numeric` | all | immutable |
| `sys.RowSecurityActive` | `row_security_active(oid)` | `bool` | all | stable |
| `text.RowSecurityActive` | `row_security_active(text)` | `bool` | all | stable |
| `text.RpadTextInt4` | `rpad(text,int4)` | `text` | all | immutable |
| `text.RpadTextInt4Text` | `rpad(text,int4,text)` | `text` | all | immutable |
| `binary.Rtrim` | `rtrim(bytea,bytea)` | `bytea` | all | immutable |
| `text.RtrimText` | `rtrim(text)` | `text` | all | immutable |
| `text.RtrimTextText` | `rtrim(text,text)` | `text` | all | immutable |

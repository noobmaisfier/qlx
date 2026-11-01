# Functions T

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `xml.TableToXml` | `table_to_xml(regclass,bool,bool,text)` | `xml` | all | stable |
| `xml.TableToXmlAndXmlschema` | `table_to_xml_and_xmlschema(regclass,bool,bool,text)` | `xml` | all | stable |
| `xml.TableToXmlschema` | `table_to_xmlschema(regclass,bool,bool,text)` | `xml` | all | stable |
| `num.Tan` | `tan(float8)` | `float8` | all | immutable |
| `num.Tand` | `tand(float8)` | `float8` | all | immutable |
| `num.Tanh` | `tanh(float8)` | `float8` | all | immutable |
| `text.TextLarger` | `text_larger(text,text)` | `text` | all | immutable |
| `text.TextSmaller` | `text_smaller(text,text)` | `text` | all | immutable |
| `text.Textlen` | `textlen(text)` | `int4` | all | immutable |
| `sys.TidBlock` | `tid_block(tid)` | `int8` | 19+ | immutable |
| `sys.TidOffset` | `tid_offset(tid)` | `int4` | 19+ | immutable |
| `sys.Tidlarger` | `tidlarger(tid,tid)` | `tid` | all | immutable |
| `sys.Tidsmaller` | `tidsmaller(tid,tid)` | `tid` | all | immutable |
| `datetime.TimeCmp` | `time_cmp(time,time)` | `int4` | all | immutable |
| `datetime.TimeHash` | `time_hash(time)` | `int4` | all | immutable |
| `datetime.TimeHashExtended` | `time_hash_extended(time,int8)` | `int8` | all | immutable |
| `datetime.TimeLarger` | `time_larger(time,time)` | `time` | all | immutable |
| `datetime.TimeSend` | `time_send(time)` | `bytea` | all | immutable |
| `datetime.TimeSmaller` | `time_smaller(time,time)` | `time` | all | immutable |
| `text.Timeofday` | `timeofday()` | `text` | all | volatile |
| `datetime.TimestampCmp` | `timestamp_cmp(timestamp,timestamp)` | `int4` | all | immutable |
| `datetime.TimestampCmpDate` | `timestamp_cmp_date(timestamp,date)` | `int4` | all | immutable |
| `datetime.TimestampCmpTimestamptz` | `timestamp_cmp_timestamptz(timestamp,timestamptz)` | `int4` | all | stable |
| `datetime.TimestampHash` | `timestamp_hash(timestamp)` | `int4` | all | immutable |
| `datetime.TimestampHashExtended` | `timestamp_hash_extended(timestamp,int8)` | `int8` | all | immutable |
| `datetime.TimestampLarger` | `timestamp_larger(timestamp,timestamp)` | `timestamp` | all | immutable |
| `datetime.TimestampSend` | `timestamp_send(timestamp)` | `bytea` | all | immutable |
| `datetime.TimestampSmaller` | `timestamp_smaller(timestamp,timestamp)` | `timestamp` | all | immutable |
| `datetime.TimestamptzCmp` | `timestamptz_cmp(timestamptz,timestamptz)` | `int4` | all | immutable |
| `datetime.TimestamptzCmpDate` | `timestamptz_cmp_date(timestamptz,date)` | `int4` | all | stable |
| `datetime.TimestamptzCmpTimestamp` | `timestamptz_cmp_timestamp(timestamptz,timestamp)` | `int4` | all | stable |
| `datetime.TimestamptzHash` | `timestamptz_hash(timestamptz)` | `int4` | 18+ | immutable |
| `datetime.TimestamptzHashExtended` | `timestamptz_hash_extended(timestamptz,int8)` | `int8` | 18+ | immutable |
| `datetime.TimestamptzLarger` | `timestamptz_larger(timestamptz,timestamptz)` | `timestamptz` | all | immutable |
| `datetime.TimestamptzSend` | `timestamptz_send(timestamptz)` | `bytea` | all | immutable |
| `datetime.TimestamptzSmaller` | `timestamptz_smaller(timestamptz,timestamptz)` | `timestamptz` | all | immutable |
| `datetime.TimetzCmp` | `timetz_cmp(timetz,timetz)` | `int4` | all | immutable |
| `datetime.TimetzHash` | `timetz_hash(timetz)` | `int4` | all | immutable |
| `datetime.TimetzHashExtended` | `timetz_hash_extended(timetz,int8)` | `int8` | all | immutable |
| `datetime.TimetzLarger` | `timetz_larger(timetz,timetz)` | `timetz` | all | immutable |
| `datetime.TimetzSend` | `timetz_send(timetz)` | `bytea` | all | immutable |
| `datetime.TimetzSmaller` | `timetz_smaller(timetz,timetz)` | `timetz` | all | immutable |
| `datetime.TimezoneIntervalTimestamp` | `timezone(interval,timestamp)` | `timestamptz` | all | immutable |
| `datetime.TimezoneIntervalTimestamptz` | `timezone(interval,timestamptz)` | `timestamp` | all | immutable |
| `datetime.TimezoneIntervalTimetz` | `timezone(interval,timetz)` | `timetz` | all | immutable |
| `datetime.TimezoneTextTimestamp` | `timezone(text,timestamp)` | `timestamptz` | all | immutable |
| `datetime.TimezoneTextTimestamptz` | `timezone(text,timestamptz)` | `timestamp` | all | immutable |
| `datetime.TimezoneTextTimetz` | `timezone(text,timetz)` | `timetz` | all | stable |
| `datetime.TimezoneTimestamp` | `timezone(timestamp)` | `timestamptz` | 17+ | stable |
| `datetime.TimezoneTimestamptz` | `timezone(timestamptz)` | `timestamp` | 17+ | stable |
| `datetime.TimezoneTimetz` | `timezone(timetz)` | `timetz` | 17+ | stable |
| `text.ToAsciiText` | `to_ascii(text)` | `text` | all | immutable |
| `text.ToAsciiTextInt4` | `to_ascii(text,int4)` | `text` | all | immutable |
| `sys.ToAscii` | `to_ascii(text,name)` | `text` | all | immutable |
| `text.ToBinInt4` | `to_bin(int4)` | `text` | 17+ | immutable |
| `text.ToBinInt8` | `to_bin(int8)` | `text` | 17+ | immutable |
| `text.ToCharFloat4Text` | `to_char(float4,text)` | `text` | all | stable |
| `text.ToCharFloat8Text` | `to_char(float8,text)` | `text` | all | stable |
| `text.ToCharInt4Text` | `to_char(int4,text)` | `text` | all | stable |
| `text.ToCharInt8Text` | `to_char(int8,text)` | `text` | all | stable |
| `datetime.ToCharIntervalText` | `to_char(interval,text)` | `text` | all | stable |
| `text.ToCharNumericText` | `to_char(numeric,text)` | `text` | all | stable |
| `datetime.ToCharTimestampText` | `to_char(timestamp,text)` | `text` | all | stable |
| `datetime.ToCharTimestamptzText` | `to_char(timestamptz,text)` | `text` | all | stable |
| `datetime.ToDate` | `to_date(text,text)` | `date` | all | stable |
| `text.ToHexInt4` | `to_hex(int4)` | `text` | all | immutable |
| `text.ToHexInt8` | `to_hex(int8)` | `text` | all | immutable |
| `jsonb.ToJson` | `to_json(anyelement)` | `json` | all | stable |
| `jsonb.ToJsonb` | `to_jsonb(anyelement)` | `jsonb` | all | stable |
| `text.ToNumber` | `to_number(text,text)` | `numeric` | all | stable |
| `text.ToOctInt4` | `to_oct(int4)` | `text` | 17+ | immutable |
| `text.ToOctInt8` | `to_oct(int8)` | `text` | 17+ | immutable |
| `sys.ToRegclass` | `to_regclass(text)` | `regclass` | all | stable, may return NULL |
| `sys.ToRegcollation` | `to_regcollation(text)` | `regcollation` | all | stable, may return NULL |
| `sys.ToRegdatabase` | `to_regdatabase(text)` | `regdatabase` | 19+ | stable, may return NULL |
| `sys.ToRegnamespace` | `to_regnamespace(text)` | `regnamespace` | all | stable, may return NULL |
| `sys.ToRegoper` | `to_regoper(text)` | `regoper` | all | stable, may return NULL |
| `sys.ToRegoperator` | `to_regoperator(text)` | `regoperator` | all | stable, may return NULL |
| `sys.ToRegproc` | `to_regproc(text)` | `regproc` | all | stable, may return NULL |
| `sys.ToRegprocedure` | `to_regprocedure(text)` | `regprocedure` | all | stable, may return NULL |
| `sys.ToRegrole` | `to_regrole(text)` | `regrole` | all | stable, may return NULL |
| `sys.ToRegtype` | `to_regtype(text)` | `regtype` | all | stable, may return NULL |
| `text.ToRegtypemod` | `to_regtypemod(text)` | `int4` | 17+ | stable, may return NULL |
| `datetime.ToTimestampFloat8` | `to_timestamp(float8)` | `timestamptz` | all | immutable |
| `datetime.ToTimestampTextText` | `to_timestamp(text,text)` | `timestamptz` | all | stable |
| `fts.ToTsqueryRegconfigText` | `to_tsquery(regconfig,text)` | `tsquery` | all | immutable |
| `fts.ToTsqueryText` | `to_tsquery(text)` | `tsquery` | all | stable |
| `jsonb.ToTsvectorJson` | `to_tsvector(json)` | `tsvector` | all | stable |
| `jsonb.ToTsvectorJsonb` | `to_tsvector(jsonb)` | `tsvector` | all | stable |
| `jsonb.ToTsvectorRegconfigJson` | `to_tsvector(regconfig,json)` | `tsvector` | all | immutable |
| `jsonb.ToTsvectorRegconfigJsonb` | `to_tsvector(regconfig,jsonb)` | `tsvector` | all | immutable |
| `fts.ToTsvectorRegconfigText` | `to_tsvector(regconfig,text)` | `tsvector` | all | immutable |
| `fts.ToTsvectorText` | `to_tsvector(text)` | `tsvector` | all | stable |
| `datetime.TransactionTimestamp` | `transaction_timestamp()` | `timestamptz` | all | stable |
| `text.Translate` | `translate(text,text,text)` | `text` | all | immutable |
| `array.TrimArray` | `trim_array(anyarray,int4)` | `anyarray` | all | immutable |
| `num.TrimScale` | `trim_scale(numeric)` | `numeric` | all | immutable |
| `num.TruncFloat8` | `trunc(float8)` | `float8` | all | immutable |
| `network.TruncMacaddr` | `trunc(macaddr)` | `macaddr` | all | immutable |
| `network.TruncMacaddr8` | `trunc(macaddr8)` | `macaddr8` | all | immutable |
| `num.TruncNumeric` | `trunc(numeric)` | `numeric` | all | immutable |
| `num.TruncNumericInt4` | `trunc(numeric,int4)` | `numeric` | all | immutable |
| `fts.TsDebug` | `ts_debug(regconfig,text)` | `record` | all | stable |
| `text.TsDebug` | `ts_debug(text)` | `record` | all | stable |
| `fts.TsDeleteTsvectorTextArray` | `ts_delete(tsvector,_text)` | `tsvector` | all | immutable |
| `fts.TsDeleteTsvectorText` | `ts_delete(tsvector,text)` | `tsvector` | all | immutable |
| `fts.TsFilter` | `ts_filter(tsvector,_char)` | `tsvector` | all | immutable |
| `jsonb.TsHeadlineJsonTsquery` | `ts_headline(json,tsquery)` | `json` | all | stable |
| `jsonb.TsHeadlineJsonTsqueryText` | `ts_headline(json,tsquery,text)` | `json` | all | stable |
| `jsonb.TsHeadlineJsonbTsquery` | `ts_headline(jsonb,tsquery)` | `jsonb` | all | stable |
| `jsonb.TsHeadlineJsonbTsqueryText` | `ts_headline(jsonb,tsquery,text)` | `jsonb` | all | stable |
| `jsonb.TsHeadlineRegconfigJsonTsquery` | `ts_headline(regconfig,json,tsquery)` | `json` | all | immutable |
| `jsonb.TsHeadlineRegconfigJsonTsqueryText` | `ts_headline(regconfig,json,tsquery,text)` | `json` | all | immutable |
| `jsonb.TsHeadlineRegconfigJsonbTsquery` | `ts_headline(regconfig,jsonb,tsquery)` | `jsonb` | all | immutable |
| `jsonb.TsHeadlineRegconfigJsonbTsqueryText` | `ts_headline(regconfig,jsonb,tsquery,text)` | `jsonb` | all | immutable |
| `fts.TsHeadlineRegconfigTextTsquery` | `ts_headline(regconfig,text,tsquery)` | `text` | all | immutable |
| `fts.TsHeadlineRegconfigTextTsqueryText` | `ts_headline(regconfig,text,tsquery,text)` | `text` | all | immutable |
| `fts.TsHeadlineTextTsquery` | `ts_headline(text,tsquery)` | `text` | all | stable |
| `fts.TsHeadlineTextTsqueryText` | `ts_headline(text,tsquery,text)` | `text` | all | stable |
| `fts.TsLexize` | `ts_lexize(regdictionary,text)` | `_text` | all | immutable |
| `fts.TsMatchQv` | `ts_match_qv(tsquery,tsvector)` | `bool` | all | immutable |
| `fts.TsMatchTq` | `ts_match_tq(text,tsquery)` | `bool` | all | stable |
| `text.TsMatchTt` | `ts_match_tt(text,text)` | `bool` | all | stable |
| `fts.TsMatchVq` | `ts_match_vq(tsvector,tsquery)` | `bool` | all | immutable |
| `sys.TsParse` | `ts_parse(oid,text)` | `record` | all | immutable |
| `text.TsParse` | `ts_parse(text,text)` | `record` | all | stable |
| `fts.TsRankFloat4ArrayTsvectorTsquery` | `ts_rank(_float4,tsvector,tsquery)` | `float4` | all | immutable |
| `fts.TsRankFloat4ArrayTsvectorTsqueryInt4` | `ts_rank(_float4,tsvector,tsquery,int4)` | `float4` | all | immutable |
| `fts.TsRankTsvectorTsquery` | `ts_rank(tsvector,tsquery)` | `float4` | all | immutable |
| `fts.TsRankTsvectorTsqueryInt4` | `ts_rank(tsvector,tsquery,int4)` | `float4` | all | immutable |
| `fts.TsRankCdFloat4ArrayTsvectorTsquery` | `ts_rank_cd(_float4,tsvector,tsquery)` | `float4` | all | immutable |
| `fts.TsRankCdFloat4ArrayTsvectorTsqueryInt4` | `ts_rank_cd(_float4,tsvector,tsquery,int4)` | `float4` | all | immutable |
| `fts.TsRankCdTsvectorTsquery` | `ts_rank_cd(tsvector,tsquery)` | `float4` | all | immutable |
| `fts.TsRankCdTsvectorTsqueryInt4` | `ts_rank_cd(tsvector,tsquery,int4)` | `float4` | all | immutable |
| `fts.TsRewriteTsqueryText` | `ts_rewrite(tsquery,text)` | `tsquery` | all | volatile |
| `fts.TsRewriteTsqueryTsqueryTsquery` | `ts_rewrite(tsquery,tsquery,tsquery)` | `tsquery` | all | immutable |
| `text.TsStatText` | `ts_stat(text)` | `record` | all | volatile |
| `text.TsStatTextText` | `ts_stat(text,text)` | `record` | all | volatile |
| `sys.TsTokenType` | `ts_token_type(oid)` | `record` | all | immutable |
| `text.TsTokenType` | `ts_token_type(text)` | `record` | all | stable |
| `fts.TsqMcontained` | `tsq_mcontained(tsquery,tsquery)` | `bool` | all | immutable |
| `fts.TsqMcontains` | `tsq_mcontains(tsquery,tsquery)` | `bool` | all | immutable |
| `fts.TsqueryCmp` | `tsquery_cmp(tsquery,tsquery)` | `int4` | all | immutable |
| `fts.TsqueryPhraseTsqueryTsquery` | `tsquery_phrase(tsquery,tsquery)` | `tsquery` | all | immutable |
| `fts.TsqueryPhraseTsqueryTsqueryInt4` | `tsquery_phrase(tsquery,tsquery,int4)` | `tsquery` | all | immutable |
| `ranges.TsrangeTimestampTimestamp` | `tsrange(timestamp,timestamp)` | `tsrange` | all | immutable, may return NULL |
| `ranges.TsrangeTimestampTimestampText` | `tsrange(timestamp,timestamp,text)` | `tsrange` | all | immutable, may return NULL |
| `datetime.TsrangeSubdiff` | `tsrange_subdiff(timestamp,timestamp)` | `float8` | all | immutable |
| `ranges.TstzrangeTimestamptzTimestamptz` | `tstzrange(timestamptz,timestamptz)` | `tstzrange` | all | immutable, may return NULL |
| `ranges.TstzrangeTimestamptzTimestamptzText` | `tstzrange(timestamptz,timestamptz,text)` | `tstzrange` | all | immutable, may return NULL |
| `datetime.TstzrangeSubdiff` | `tstzrange_subdiff(timestamptz,timestamptz)` | `float8` | all | immutable |
| `fts.TsvectorCmp` | `tsvector_cmp(tsvector,tsvector)` | `int4` | all | immutable |
| `fts.TsvectorToArray` | `tsvector_to_array(tsvector)` | `_text` | all | immutable |
| `num.TxidCurrent` | `txid_current()` | `int8` | all | stable |
| `num.TxidCurrentIfAssigned` | `txid_current_if_assigned()` | `int8` | all | stable |
| `sys.TxidCurrentSnapshot` | `txid_current_snapshot()` | `txid_snapshot` | all | stable |
| `sys.TxidSnapshotSend` | `txid_snapshot_send(txid_snapshot)` | `bytea` | all | immutable |
| `sys.TxidSnapshotXip` | `txid_snapshot_xip(txid_snapshot)` | `int8` | all | immutable |
| `sys.TxidSnapshotXmax` | `txid_snapshot_xmax(txid_snapshot)` | `int8` | all | immutable |
| `sys.TxidSnapshotXmin` | `txid_snapshot_xmin(txid_snapshot)` | `int8` | all | immutable |
| `text.TxidStatus` | `txid_status(int8)` | `text` | all | volatile |
| `sys.TxidVisibleInSnapshot` | `txid_visible_in_snapshot(int8,txid_snapshot)` | `bool` | all | immutable |

# Functions C

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `array.Cardinality` | `cardinality(anyarray)` | `int4` | all | immutable |
| `text.Casefold` | `casefold(text)` | `text` | 18+ | immutable |
| `num.CashCmp` | `cash_cmp(money,money)` | `int4` | all | immutable |
| `num.CashDivCash` | `cash_div_cash(money,money)` | `float8` | all | immutable |
| `num.CashDivFlt4` | `cash_div_flt4(money,float4)` | `money` | all | immutable |
| `num.CashDivFlt8` | `cash_div_flt8(money,float8)` | `money` | all | immutable |
| `num.CashDivInt2` | `cash_div_int2(money,int2)` | `money` | all | immutable |
| `num.CashDivInt4` | `cash_div_int4(money,int4)` | `money` | all | immutable |
| `num.CashDivInt8` | `cash_div_int8(money,int8)` | `money` | all | immutable |
| `num.CashEq` | `cash_eq(money,money)` | `bool` | all | immutable |
| `num.CashGe` | `cash_ge(money,money)` | `bool` | all | immutable |
| `num.CashGt` | `cash_gt(money,money)` | `bool` | all | immutable |
| `num.CashLe` | `cash_le(money,money)` | `bool` | all | immutable |
| `num.CashLt` | `cash_lt(money,money)` | `bool` | all | immutable |
| `num.CashMi` | `cash_mi(money,money)` | `money` | all | immutable |
| `num.CashMulFlt4` | `cash_mul_flt4(money,float4)` | `money` | all | immutable |
| `num.CashMulFlt8` | `cash_mul_flt8(money,float8)` | `money` | all | immutable |
| `num.CashMulInt2` | `cash_mul_int2(money,int2)` | `money` | all | immutable |
| `num.CashMulInt4` | `cash_mul_int4(money,int4)` | `money` | all | immutable |
| `num.CashMulInt8` | `cash_mul_int8(money,int8)` | `money` | all | immutable |
| `num.CashNe` | `cash_ne(money,money)` | `bool` | all | immutable |
| `num.CashPl` | `cash_pl(money,money)` | `money` | all | immutable |
| `binary.CashSend` | `cash_send(money)` | `bytea` | all | immutable |
| `text.CashWords` | `cash_words(money)` | `text` | all | immutable |
| `num.Cashlarger` | `cashlarger(money,money)` | `money` | all | immutable |
| `num.Cashsmaller` | `cashsmaller(money,money)` | `money` | all | immutable |
| `num.Cbrt` | `cbrt(float8)` | `float8` | all | immutable |
| `num.CeilFloat8` | `ceil(float8)` | `float8` | all | immutable |
| `num.CeilNumeric` | `ceil(numeric)` | `numeric` | all | immutable |
| `num.CeilingFloat8` | `ceiling(float8)` | `float8` | all | immutable |
| `num.CeilingNumeric` | `ceiling(numeric)` | `numeric` | all | immutable |
| `geo.CenterBox` | `center(box)` | `point` | all | immutable |
| `geo.CenterCircle` | `center(circle)` | `point` | all | immutable |
| `text.CharLengthBpchar` | `char_length(bpchar)` | `int4` | all | immutable |
| `text.CharLengthText` | `char_length(text)` | `int4` | all | immutable |
| `text.CharacterLengthBpchar` | `character_length(bpchar)` | `int4` | all | immutable |
| `text.CharacterLengthText` | `character_length(text)` | `int4` | all | immutable |
| `text.Chr` | `chr(int4)` | `text` | all | immutable |
| `network.CidrSend` | `cidr_send(cidr)` | `bytea` | all | immutable |
| `geo.CircleSend` | `circle_send(circle)` | `bytea` | all | immutable |
| `datetime.ClockTimestamp` | `clock_timestamp()` | `timestamptz` | all | volatile |
| `geo.CloseLs` | `close_ls(line,lseg)` | `point` | all | immutable |
| `geo.CloseLseg` | `close_lseg(lseg,lseg)` | `point` | all | immutable |
| `geo.ClosePb` | `close_pb(point,box)` | `point` | all | immutable |
| `geo.ClosePl` | `close_pl(point,line)` | `point` | all | immutable |
| `geo.ClosePs` | `close_ps(point,lseg)` | `point` | all | immutable |
| `geo.CloseSb` | `close_sb(lseg,box)` | `point` | all | immutable |
| `sys.ColDescription` | `col_description(oid,int4)` | `text` | all | stable, may return NULL |
| `text.Concat` | `concat(any)` | `text` | all | stable, may return NULL |
| `text.ConcatWs` | `concat_ws(text,any)` | `text` | all | stable, may return NULL |
| `sys.Convert` | `convert(bytea,name,name)` | `bytea` | all | stable |
| `sys.ConvertFrom` | `convert_from(bytea,name)` | `text` | all | stable |
| `sys.ConvertTo` | `convert_to(text,name)` | `bytea` | all | stable |
| `num.Corr` | `corr(float8,float8)` | `float8` | all | immutable, may return NULL |
| `num.Cos` | `cos(float8)` | `float8` | all | immutable |
| `num.Cosd` | `cosd(float8)` | `float8` | all | immutable |
| `num.Cosh` | `cosh(float8)` | `float8` | all | immutable |
| `num.Cot` | `cot(float8)` | `float8` | all | immutable |
| `num.Cotd` | `cotd(float8)` | `float8` | all | immutable |
| `num.Count` | `count(any)` | `int8` | all | immutable |
| `num.CovarPop` | `covar_pop(float8,float8)` | `float8` | all | immutable, may return NULL |
| `num.CovarSamp` | `covar_samp(float8,float8)` | `float8` | all | immutable, may return NULL |
| `binary.Crc32` | `crc32(bytea)` | `int8` | 18+ | immutable |
| `binary.Crc32c` | `crc32c(bytea)` | `int8` | 18+ | immutable |
| `sys.CurrentDatabase` | `current_database()` | `name` | all | stable |
| `text.CurrentQuery` | `current_query()` | `text` | all | volatile, may return NULL |
| `sys.CurrentSchema` | `current_schema()` | `name` | all | stable |
| `sys.CurrentSchemas` | `current_schemas(bool)` | `_name` | all | stable |
| `text.CurrentSettingText` | `current_setting(text)` | `text` | all | stable |
| `text.CurrentSettingTextBool` | `current_setting(text,bool)` | `text` | all | stable |
| `sys.CurrentUser` | `current_user()` | `name` | all | stable |
| `sys.Currtid2` | `currtid2(text,tid)` | `tid` | all | volatile |
| `sys.Currval` | `currval(regclass)` | `int8` | all | volatile |
| `xml.CursorToXml` | `cursor_to_xml(refcursor,int4,bool,bool,text)` | `xml` | all | volatile |
| `xml.CursorToXmlschema` | `cursor_to_xmlschema(refcursor,bool,bool,text)` | `xml` | all | volatile |

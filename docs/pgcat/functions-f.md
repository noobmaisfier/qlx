# Functions F

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `num.Factorial` | `factorial(int8)` | `numeric` | all | immutable |
| `network.Family` | `family(inet)` | `int4` | all | immutable |
| `array.Float4Accum` | `float4_accum(_float8,float4)` | `_float8` | all | immutable |
| `num.Float4larger` | `float4larger(float4,float4)` | `float4` | all | immutable |
| `num.Float4smaller` | `float4smaller(float4,float4)` | `float4` | all | immutable |
| `array.Float8Accum` | `float8_accum(_float8,float8)` | `_float8` | all | immutable |
| `array.Float8Avg` | `float8_avg(_float8)` | `float8` | all | immutable |
| `array.Float8Combine` | `float8_combine(_float8,_float8)` | `_float8` | all | immutable |
| `array.Float8Corr` | `float8_corr(_float8)` | `float8` | all | immutable |
| `array.Float8CovarPop` | `float8_covar_pop(_float8)` | `float8` | all | immutable |
| `array.Float8CovarSamp` | `float8_covar_samp(_float8)` | `float8` | all | immutable |
| `array.Float8RegrAccum` | `float8_regr_accum(_float8,float8,float8)` | `_float8` | all | immutable |
| `array.Float8RegrAvgx` | `float8_regr_avgx(_float8)` | `float8` | all | immutable |
| `array.Float8RegrAvgy` | `float8_regr_avgy(_float8)` | `float8` | all | immutable |
| `array.Float8RegrCombine` | `float8_regr_combine(_float8,_float8)` | `_float8` | all | immutable |
| `array.Float8RegrIntercept` | `float8_regr_intercept(_float8)` | `float8` | all | immutable |
| `array.Float8RegrR2` | `float8_regr_r2(_float8)` | `float8` | all | immutable |
| `array.Float8RegrSlope` | `float8_regr_slope(_float8)` | `float8` | all | immutable |
| `array.Float8RegrSxx` | `float8_regr_sxx(_float8)` | `float8` | all | immutable |
| `array.Float8RegrSxy` | `float8_regr_sxy(_float8)` | `float8` | all | immutable |
| `array.Float8RegrSyy` | `float8_regr_syy(_float8)` | `float8` | all | immutable |
| `array.Float8StddevPop` | `float8_stddev_pop(_float8)` | `float8` | all | immutable |
| `array.Float8StddevSamp` | `float8_stddev_samp(_float8)` | `float8` | all | immutable |
| `array.Float8VarPop` | `float8_var_pop(_float8)` | `float8` | all | immutable |
| `array.Float8VarSamp` | `float8_var_samp(_float8)` | `float8` | all | immutable |
| `num.Float8larger` | `float8larger(float8,float8)` | `float8` | all | immutable |
| `num.Float8smaller` | `float8smaller(float8,float8)` | `float8` | all | immutable |
| `num.FloorFloat8` | `floor(float8)` | `float8` | all | immutable |
| `num.FloorNumeric` | `floor(numeric)` | `numeric` | all | immutable |
| `num.Flt4MulCash` | `flt4_mul_cash(float4,money)` | `money` | all | immutable |
| `num.Flt8MulCash` | `flt8_mul_cash(float8,money)` | `money` | all | immutable |
| `sys.FmgrCValidator` | `fmgr_c_validator(oid)` | `void` | all | stable |
| `sys.FmgrInternalValidator` | `fmgr_internal_validator(oid)` | `void` | all | stable |
| `sys.FmgrSqlValidator` | `fmgr_sql_validator(oid)` | `void` | all | stable |
| `text.FormatText` | `format(text)` | `text` | all | stable, may return NULL |
| `text.FormatTextAny` | `format(text,any)` | `text` | all | stable, may return NULL |
| `sys.FormatType` | `format_type(oid,int4)` | `text` | all | stable, may return NULL |

# Functions V

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `num.VarPopFloat4` | `var_pop(float4)` | `float8` | all | immutable, may return NULL |
| `num.VarPopFloat8` | `var_pop(float8)` | `float8` | all | immutable, may return NULL |
| `num.VarPopInt2` | `var_pop(int2)` | `numeric` | all | immutable, may return NULL |
| `num.VarPopInt4` | `var_pop(int4)` | `numeric` | all | immutable, may return NULL |
| `num.VarPopInt8` | `var_pop(int8)` | `numeric` | all | immutable, may return NULL |
| `num.VarPopNumeric` | `var_pop(numeric)` | `numeric` | all | immutable, may return NULL |
| `num.VarSampFloat4` | `var_samp(float4)` | `float8` | all | immutable, may return NULL |
| `num.VarSampFloat8` | `var_samp(float8)` | `float8` | all | immutable, may return NULL |
| `num.VarSampInt2` | `var_samp(int2)` | `numeric` | all | immutable, may return NULL |
| `num.VarSampInt4` | `var_samp(int4)` | `numeric` | all | immutable, may return NULL |
| `num.VarSampInt8` | `var_samp(int8)` | `numeric` | all | immutable, may return NULL |
| `num.VarSampNumeric` | `var_samp(numeric)` | `numeric` | all | immutable, may return NULL |
| `binary.VarbitSend` | `varbit_send(varbit)` | `bytea` | all | immutable |
| `binary.Varbitcmp` | `varbitcmp(varbit,varbit)` | `int4` | all | immutable |
| `num.VarianceFloat4` | `variance(float4)` | `float8` | all | immutable, may return NULL |
| `num.VarianceFloat8` | `variance(float8)` | `float8` | all | immutable, may return NULL |
| `num.VarianceInt2` | `variance(int2)` | `numeric` | all | immutable, may return NULL |
| `num.VarianceInt4` | `variance(int4)` | `numeric` | all | immutable, may return NULL |
| `num.VarianceInt8` | `variance(int8)` | `numeric` | all | immutable, may return NULL |
| `num.VarianceNumeric` | `variance(numeric)` | `numeric` | all | immutable, may return NULL |
| `text.Version` | `version()` | `text` | all | stable |

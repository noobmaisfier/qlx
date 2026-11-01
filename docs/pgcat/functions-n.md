# Functions N

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `sys.Nameconcatoid` | `nameconcatoid(name,oid)` | `name` | all | immutable |
| `network.Netmask` | `netmask(inet)` | `inet` | all | immutable |
| `network.Network` | `network(inet)` | `cidr` | all | immutable |
| `network.NetworkCmp` | `network_cmp(inet,inet)` | `int4` | all | immutable |
| `network.NetworkEq` | `network_eq(inet,inet)` | `bool` | all | immutable |
| `network.NetworkGe` | `network_ge(inet,inet)` | `bool` | all | immutable |
| `network.NetworkGt` | `network_gt(inet,inet)` | `bool` | all | immutable |
| `network.NetworkLarger` | `network_larger(inet,inet)` | `inet` | all | immutable |
| `network.NetworkLe` | `network_le(inet,inet)` | `bool` | all | immutable |
| `network.NetworkLt` | `network_lt(inet,inet)` | `bool` | all | immutable |
| `network.NetworkNe` | `network_ne(inet,inet)` | `bool` | all | immutable |
| `network.NetworkOverlap` | `network_overlap(inet,inet)` | `bool` | all | immutable |
| `network.NetworkSmaller` | `network_smaller(inet,inet)` | `inet` | all | immutable |
| `network.NetworkSub` | `network_sub(inet,inet)` | `bool` | all | immutable |
| `network.NetworkSubeq` | `network_subeq(inet,inet)` | `bool` | all | immutable |
| `network.NetworkSup` | `network_sup(inet,inet)` | `bool` | all | immutable |
| `network.NetworkSupeq` | `network_supeq(inet,inet)` | `bool` | all | immutable |
| `sys.Nextval` | `nextval(regclass)` | `int8` | all | volatile |
| `text.Normalize` | `normalize(text,text)` | `text` | all | immutable |
| `binary.Notlike` | `notlike(bytea,bytea)` | `bool` | all | immutable |
| `sys.Notlike` | `notlike(name,text)` | `bool` | all | immutable |
| `text.Notlike` | `notlike(text,text)` | `bool` | all | immutable |
| `datetime.Now` | `now()` | `timestamptz` | all | stable |
| `geo.NpointsPath` | `npoints(path)` | `int4` | all | immutable |
| `geo.NpointsPolygon` | `npoints(polygon)` | `int4` | all | immutable |
| `num.NumNonnulls` | `num_nonnulls(any)` | `int4` | all | immutable, may return NULL |
| `num.NumNulls` | `num_nulls(any)` | `int4` | all | immutable, may return NULL |
| `num.NumericCmp` | `numeric_cmp(numeric,numeric)` | `int4` | all | immutable |
| `num.NumericDivTrunc` | `numeric_div_trunc(numeric,numeric)` | `numeric` | all | immutable |
| `num.NumericExp` | `numeric_exp(numeric)` | `numeric` | all | immutable |
| `num.NumericInc` | `numeric_inc(numeric)` | `numeric` | all | immutable |
| `num.NumericLarger` | `numeric_larger(numeric,numeric)` | `numeric` | all | immutable |
| `num.NumericLn` | `numeric_ln(numeric)` | `numeric` | all | immutable |
| `num.NumericLog` | `numeric_log(numeric,numeric)` | `numeric` | all | immutable |
| `binary.NumericSend` | `numeric_send(numeric)` | `bytea` | all | immutable |
| `num.NumericSmaller` | `numeric_smaller(numeric,numeric)` | `numeric` | all | immutable |
| `num.NumericSqrt` | `numeric_sqrt(numeric)` | `numeric` | all | immutable |
| `fts.Numnode` | `numnode(tsquery)` | `int4` | all | immutable |
| `ranges.NumrangeNumericNumeric` | `numrange(numeric,numeric)` | `numrange` | all | immutable, may return NULL |
| `ranges.NumrangeNumericNumericText` | `numrange(numeric,numeric,text)` | `numrange` | all | immutable, may return NULL |
| `num.NumrangeSubdiff` | `numrange_subdiff(numeric,numeric)` | `float8` | all | immutable |

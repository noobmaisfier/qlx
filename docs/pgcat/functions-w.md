# Functions W

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `fts.WebsearchToTsqueryRegconfigText` | `websearch_to_tsquery(regconfig,text)` | `tsquery` | all | immutable |
| `fts.WebsearchToTsqueryText` | `websearch_to_tsquery(text)` | `tsquery` | all | stable |
| `geo.Width` | `width(box)` | `float8` | all | immutable |
| `array.WidthBucket` | `width_bucket(anycompatible,anycompatiblearray)` | `int4` | all | immutable |
| `num.WidthBucketFloat8Float8Float8Int4` | `width_bucket(float8,float8,float8,int4)` | `int4` | all | immutable |
| `num.WidthBucketNumericNumericNumericInt4` | `width_bucket(numeric,numeric,numeric,int4)` | `int4` | all | immutable |

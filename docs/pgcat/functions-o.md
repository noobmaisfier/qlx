# Functions O

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `sys.ObjDescriptionOid` | `obj_description(oid)` | `text` | all | stable, may return NULL |
| `sys.ObjDescriptionOidName` | `obj_description(oid,name)` | `text` | all | stable, may return NULL |
| `binary.OctetLengthBit` | `octet_length(bit)` | `int4` | all | immutable |
| `text.OctetLengthBpchar` | `octet_length(bpchar)` | `int4` | all | immutable |
| `binary.OctetLengthBytea` | `octet_length(bytea)` | `int4` | all | immutable |
| `text.OctetLengthText` | `octet_length(text)` | `int4` | all | immutable |
| `sys.Oid8larger` | `oid8larger(oid8,oid8)` | `oid8` | 19+ | immutable |
| `sys.Oid8smaller` | `oid8smaller(oid8,oid8)` | `oid8` | 19+ | immutable |
| `sys.Oidlarger` | `oidlarger(oid,oid)` | `oid` | all | immutable |
| `sys.Oidsmaller` | `oidsmaller(oid,oid)` | `oid` | all | immutable |
| `sys.Oidvectortypes` | `oidvectortypes(oidvector)` | `text` | all | stable |
| `geo.OnPb` | `on_pb(point,box)` | `bool` | all | immutable |
| `geo.OnPl` | `on_pl(point,line)` | `bool` | all | immutable |
| `geo.OnPpath` | `on_ppath(point,path)` | `bool` | all | immutable |
| `geo.OnPs` | `on_ps(point,lseg)` | `bool` | all | immutable |
| `geo.OnSb` | `on_sb(lseg,box)` | `bool` | all | immutable |
| `geo.OnSl` | `on_sl(lseg,line)` | `bool` | all | immutable |
| `datetime.OverlapsTimeIntervalTimeInterval` | `overlaps(time,interval,time,interval)` | `bool` | all | immutable, may return NULL |
| `datetime.OverlapsTimeIntervalTimeTime` | `overlaps(time,interval,time,time)` | `bool` | all | immutable, may return NULL |
| `datetime.OverlapsTimeTimeTimeInterval` | `overlaps(time,time,time,interval)` | `bool` | all | immutable, may return NULL |
| `datetime.OverlapsTimeTimeTimeTime` | `overlaps(time,time,time,time)` | `bool` | all | immutable, may return NULL |
| `datetime.OverlapsTimestampIntervalTimestampInterval` | `overlaps(timestamp,interval,timestamp,interval)` | `bool` | all | immutable, may return NULL |
| `datetime.OverlapsTimestampIntervalTimestampTimestamp` | `overlaps(timestamp,interval,timestamp,timestamp)` | `bool` | all | immutable, may return NULL |
| `datetime.OverlapsTimestampTimestampTimestampInterval` | `overlaps(timestamp,timestamp,timestamp,interval)` | `bool` | all | immutable, may return NULL |
| `datetime.OverlapsTimestampTimestampTimestampTimestamp` | `overlaps(timestamp,timestamp,timestamp,timestamp)` | `bool` | all | immutable, may return NULL |
| `datetime.OverlapsTimestamptzIntervalTimestamptzInterval` | `overlaps(timestamptz,interval,timestamptz,interval)` | `bool` | all | stable, may return NULL |
| `datetime.OverlapsTimestamptzIntervalTimestamptzTimestamptz` | `overlaps(timestamptz,interval,timestamptz,timestamptz)` | `bool` | all | stable, may return NULL |
| `datetime.OverlapsTimestamptzTimestamptzTimestamptzInterval` | `overlaps(timestamptz,timestamptz,timestamptz,interval)` | `bool` | all | stable, may return NULL |
| `datetime.OverlapsTimestamptzTimestamptzTimestamptzTimestamptz` | `overlaps(timestamptz,timestamptz,timestamptz,timestamptz)` | `bool` | all | immutable, may return NULL |
| `datetime.OverlapsTimetzTimetzTimetzTimetz` | `overlaps(timetz,timetz,timetz,timetz)` | `bool` | all | immutable, may return NULL |
| `binary.OverlayBitBitInt4` | `overlay(bit,bit,int4)` | `bit` | all | immutable |
| `binary.OverlayBitBitInt4Int4` | `overlay(bit,bit,int4,int4)` | `bit` | all | immutable |
| `binary.OverlayByteaByteaInt4` | `overlay(bytea,bytea,int4)` | `bytea` | all | immutable |
| `binary.OverlayByteaByteaInt4Int4` | `overlay(bytea,bytea,int4,int4)` | `bytea` | all | immutable |
| `text.OverlayTextTextInt4` | `overlay(text,text,int4)` | `text` | all | immutable |
| `text.OverlayTextTextInt4Int4` | `overlay(text,text,int4,int4)` | `text` | all | immutable |

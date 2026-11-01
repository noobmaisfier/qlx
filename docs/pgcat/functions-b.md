# Functions B

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `sys.BinaryUpgradeAddSubRelState` | `binary_upgrade_add_sub_rel_state(text,oid,char,pg_lsn)` | `void` | 17+ | volatile |
| `sys.BinaryUpgradeCheckLogicalSlotPendingWal` | `binary_upgrade_check_logical_slot_pending_wal(name,pg_lsn)` | `pg_lsn` | 19+ | volatile |
| `misc.BinaryUpgradeCreateConflictDetectionSlot` | `binary_upgrade_create_conflict_detection_slot()` | `void` | 19+ | volatile |
| `sys.BinaryUpgradeCreateEmptyExtension` | `binary_upgrade_create_empty_extension(text,text,bool,text,_oid,_text,_text)` | `void` | all | volatile |
| `sys.BinaryUpgradeLogicalSlotHasCaughtUp` | `binary_upgrade_logical_slot_has_caught_up(name)` | `bool` | 17–18 | volatile |
| `sys.BinaryUpgradeReploriginAdvance` | `binary_upgrade_replorigin_advance(text,pg_lsn)` | `void` | 17+ | volatile |
| `sys.BinaryUpgradeSetMissingValue` | `binary_upgrade_set_missing_value(oid,text,text)` | `void` | all | volatile |
| `sys.BinaryUpgradeSetNextArrayPgTypeOid` | `binary_upgrade_set_next_array_pg_type_oid(oid)` | `void` | all | volatile |
| `sys.BinaryUpgradeSetNextHeapPgClassOid` | `binary_upgrade_set_next_heap_pg_class_oid(oid)` | `void` | all | volatile |
| `sys.BinaryUpgradeSetNextHeapRelfilenode` | `binary_upgrade_set_next_heap_relfilenode(oid)` | `void` | all | volatile |
| `sys.BinaryUpgradeSetNextIndexPgClassOid` | `binary_upgrade_set_next_index_pg_class_oid(oid)` | `void` | all | volatile |
| `sys.BinaryUpgradeSetNextIndexRelfilenode` | `binary_upgrade_set_next_index_relfilenode(oid)` | `void` | all | volatile |
| `sys.BinaryUpgradeSetNextMultirangeArrayPgTypeOid` | `binary_upgrade_set_next_multirange_array_pg_type_oid(oid)` | `void` | all | volatile |
| `sys.BinaryUpgradeSetNextMultirangePgTypeOid` | `binary_upgrade_set_next_multirange_pg_type_oid(oid)` | `void` | all | volatile |
| `sys.BinaryUpgradeSetNextPgAuthidOid` | `binary_upgrade_set_next_pg_authid_oid(oid)` | `void` | all | volatile |
| `sys.BinaryUpgradeSetNextPgEnumOid` | `binary_upgrade_set_next_pg_enum_oid(oid)` | `void` | all | volatile |
| `sys.BinaryUpgradeSetNextPgTablespaceOid` | `binary_upgrade_set_next_pg_tablespace_oid(oid)` | `void` | all | volatile |
| `sys.BinaryUpgradeSetNextPgTypeOid` | `binary_upgrade_set_next_pg_type_oid(oid)` | `void` | all | volatile |
| `sys.BinaryUpgradeSetNextToastPgClassOid` | `binary_upgrade_set_next_toast_pg_class_oid(oid)` | `void` | all | volatile |
| `sys.BinaryUpgradeSetNextToastRelfilenode` | `binary_upgrade_set_next_toast_relfilenode(oid)` | `void` | all | volatile |
| `misc.BinaryUpgradeSetRecordInitPrivs` | `binary_upgrade_set_record_init_privs(bool)` | `void` | all | volatile |
| `binary.BitAnd` | `bit_and(bit)` | `bit` | all | immutable, may return NULL |
| `num.BitAndInt2` | `bit_and(int2)` | `int2` | all | immutable, may return NULL |
| `num.BitAndInt4` | `bit_and(int4)` | `int4` | all | immutable, may return NULL |
| `num.BitAndInt8` | `bit_and(int8)` | `int8` | all | immutable, may return NULL |
| `binary.BitCountBit` | `bit_count(bit)` | `int8` | all | immutable |
| `binary.BitCountBytea` | `bit_count(bytea)` | `int8` | all | immutable |
| `binary.BitLengthBit` | `bit_length(bit)` | `int4` | all | immutable |
| `binary.BitLengthBytea` | `bit_length(bytea)` | `int4` | all | immutable |
| `text.BitLength` | `bit_length(text)` | `int4` | all | immutable |
| `binary.BitOr` | `bit_or(bit)` | `bit` | all | immutable, may return NULL |
| `num.BitOrInt2` | `bit_or(int2)` | `int2` | all | immutable, may return NULL |
| `num.BitOrInt4` | `bit_or(int4)` | `int4` | all | immutable, may return NULL |
| `num.BitOrInt8` | `bit_or(int8)` | `int8` | all | immutable, may return NULL |
| `binary.BitSend` | `bit_send(bit)` | `bytea` | all | immutable |
| `binary.BitXor` | `bit_xor(bit)` | `bit` | all | immutable, may return NULL |
| `num.BitXorInt2` | `bit_xor(int2)` | `int2` | all | immutable, may return NULL |
| `num.BitXorInt4` | `bit_xor(int4)` | `int4` | all | immutable, may return NULL |
| `num.BitXorInt8` | `bit_xor(int8)` | `int8` | all | immutable, may return NULL |
| `binary.Bitcmp` | `bitcmp(bit,bit)` | `int4` | all | immutable |
| `misc.BoolAnd` | `bool_and(bool)` | `bool` | all | immutable, may return NULL |
| `misc.BoolOr` | `bool_or(bool)` | `bool` | all | immutable, may return NULL |
| `misc.BoolandStatefunc` | `booland_statefunc(bool,bool)` | `bool` | all | immutable |
| `misc.BoolorStatefunc` | `boolor_statefunc(bool,bool)` | `bool` | all | immutable |
| `geo.BoundBox` | `bound_box(box,box)` | `box` | all | immutable |
| `geo.BoxSend` | `box_send(box)` | `bytea` | all | immutable |
| `text.BpcharLarger` | `bpchar_larger(bpchar,bpchar)` | `bpchar` | all | immutable |
| `text.BpcharSmaller` | `bpchar_smaller(bpchar,bpchar)` | `bpchar` | all | immutable |
| `text.Bpcharcmp` | `bpcharcmp(bpchar,bpchar)` | `int4` | all | immutable |
| `sys.BrinDesummarizeRange` | `brin_desummarize_range(regclass,int8)` | `void` | all | volatile |
| `sys.BrinSummarizeNewValues` | `brin_summarize_new_values(regclass)` | `int4` | all | volatile |
| `sys.BrinSummarizeRange` | `brin_summarize_range(regclass,int8)` | `int4` | all | volatile |
| `network.Broadcast` | `broadcast(inet)` | `inet` | all | immutable |
| `array.Btarraycmp` | `btarraycmp(anyarray,anyarray)` | `int4` | all | immutable |
| `num.Btboolcmp` | `btboolcmp(bool,bool)` | `int4` | all | immutable |
| `text.BtbpcharPatternCmp` | `btbpchar_pattern_cmp(bpchar,bpchar)` | `int4` | all | immutable |
| `text.Btcharcmp` | `btcharcmp(char,char)` | `int4` | all | immutable |
| `sys.Btequalimage` | `btequalimage(oid)` | `bool` | all | immutable |
| `num.Btfloat48cmp` | `btfloat48cmp(float4,float8)` | `int4` | all | immutable |
| `num.Btfloat4cmp` | `btfloat4cmp(float4,float4)` | `int4` | all | immutable |
| `num.Btfloat84cmp` | `btfloat84cmp(float8,float4)` | `int4` | all | immutable |
| `num.Btfloat8cmp` | `btfloat8cmp(float8,float8)` | `int4` | all | immutable |
| `num.Btint24cmp` | `btint24cmp(int2,int4)` | `int4` | all | immutable |
| `num.Btint28cmp` | `btint28cmp(int2,int8)` | `int4` | all | immutable |
| `num.Btint2cmp` | `btint2cmp(int2,int2)` | `int4` | all | immutable |
| `num.Btint42cmp` | `btint42cmp(int4,int2)` | `int4` | all | immutable |
| `num.Btint48cmp` | `btint48cmp(int4,int8)` | `int4` | all | immutable |
| `num.Btint4cmp` | `btint4cmp(int4,int4)` | `int4` | all | immutable |
| `num.Btint82cmp` | `btint82cmp(int8,int2)` | `int4` | all | immutable |
| `num.Btint84cmp` | `btint84cmp(int8,int4)` | `int4` | all | immutable |
| `num.Btint8cmp` | `btint8cmp(int8,int8)` | `int4` | all | immutable |
| `sys.Btnamecmp` | `btnamecmp(name,name)` | `int4` | all | immutable |
| `sys.Btnametextcmp` | `btnametextcmp(name,text)` | `int4` | all | immutable |
| `sys.Btoid8cmp` | `btoid8cmp(oid8,oid8)` | `int4` | 19+ | immutable |
| `sys.Btoidcmp` | `btoidcmp(oid,oid)` | `int4` | all | immutable |
| `sys.Btoidvectorcmp` | `btoidvectorcmp(oidvector,oidvector)` | `int4` | all | immutable |
| `binary.Btrim` | `btrim(bytea,bytea)` | `bytea` | all | immutable |
| `text.BtrimText` | `btrim(text)` | `text` | all | immutable |
| `text.BtrimTextText` | `btrim(text,text)` | `text` | all | immutable |
| `text.BttextPatternCmp` | `bttext_pattern_cmp(text,text)` | `int4` | all | immutable |
| `text.Bttextcmp` | `bttextcmp(text,text)` | `int4` | all | immutable |
| `sys.Bttextnamecmp` | `bttextnamecmp(text,name)` | `int4` | all | immutable |
| `sys.Bttidcmp` | `bttidcmp(tid,tid)` | `int4` | all | immutable |
| `sys.Btvarstrequalimage` | `btvarstrequalimage(oid)` | `bool` | all | immutable |
| `binary.ByteaLarger` | `bytea_larger(bytea,bytea)` | `bytea` | 18+ | immutable |
| `binary.ByteaSmaller` | `bytea_smaller(bytea,bytea)` | `bytea` | 18+ | immutable |
| `binary.Byteacmp` | `byteacmp(bytea,bytea)` | `int4` | all | immutable |

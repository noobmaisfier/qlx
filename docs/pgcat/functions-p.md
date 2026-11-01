# Functions P

| Go | PostgreSQL | Returns | Versions | Notes |
|---|---|---|---|---|
| `array.ParseIdent` | `parse_ident(text,bool)` | `_text` | all | immutable |
| `geo.PathSend` | `path_send(path)` | `bytea` | all | immutable |
| `geo.Pclose` | `pclose(path)` | `path` | all | immutable |
| `sys.PgAdvisoryLockInt4Int4` | `pg_advisory_lock(int4,int4)` | `void` | all | volatile |
| `sys.PgAdvisoryLockInt8` | `pg_advisory_lock(int8)` | `void` | all | volatile |
| `sys.PgAdvisoryLockSharedInt4Int4` | `pg_advisory_lock_shared(int4,int4)` | `void` | all | volatile |
| `sys.PgAdvisoryLockSharedInt8` | `pg_advisory_lock_shared(int8)` | `void` | all | volatile |
| `sys.PgAdvisoryUnlockInt4Int4` | `pg_advisory_unlock(int4,int4)` | `bool` | all | volatile |
| `sys.PgAdvisoryUnlockInt8` | `pg_advisory_unlock(int8)` | `bool` | all | volatile |
| `sys.PgAdvisoryUnlockAll` | `pg_advisory_unlock_all()` | `void` | all | volatile |
| `sys.PgAdvisoryUnlockSharedInt4Int4` | `pg_advisory_unlock_shared(int4,int4)` | `bool` | all | volatile |
| `sys.PgAdvisoryUnlockSharedInt8` | `pg_advisory_unlock_shared(int8)` | `bool` | all | volatile |
| `sys.PgAdvisoryXactLockInt4Int4` | `pg_advisory_xact_lock(int4,int4)` | `void` | all | volatile |
| `sys.PgAdvisoryXactLockInt8` | `pg_advisory_xact_lock(int8)` | `void` | all | volatile |
| `sys.PgAdvisoryXactLockSharedInt4Int4` | `pg_advisory_xact_lock_shared(int4,int4)` | `void` | all | volatile |
| `sys.PgAdvisoryXactLockSharedInt8` | `pg_advisory_xact_lock_shared(int8)` | `void` | all | volatile |
| `sys.PgAvailableExtensionVersionsUpTo18` | `pg_available_extension_versions()` | `record` | up to 18 | stable |
| `sys.PgAvailableExtensionVersionsFrom19` | `pg_available_extension_versions()` | `record` | 19+ | stable |
| `sys.PgAvailableExtensionsUpTo18` | `pg_available_extensions()` | `record` | up to 18 | stable |
| `sys.PgAvailableExtensionsFrom19` | `pg_available_extensions()` | `record` | 19+ | stable |
| `sys.PgAvailableWalSummaries` | `pg_available_wal_summaries()` | `record` | 17+ | volatile |
| `sys.PgBackendPid` | `pg_backend_pid()` | `int4` | all | stable |
| `sys.PgBackupStart` | `pg_backup_start(text,bool)` | `pg_lsn` | all | volatile |
| `sys.PgBackupStop` | `pg_backup_stop(bool)` | `record` | all | volatile |
| `sys.PgBasetype` | `pg_basetype(regtype)` | `regtype` | 17+ | stable |
| `sys.PgBlockingPids` | `pg_blocking_pids(int4)` | `_int4` | all | volatile |
| `sys.PgCancelBackend` | `pg_cancel_backend(int4)` | `bool` | all | volatile |
| `sys.PgCharToEncoding` | `pg_char_to_encoding(name)` | `int4` | all | stable |
| `sys.PgClearAttributeStats` | `pg_clear_attribute_stats(text,text,text,bool)` | `void` | 18+ | volatile |
| `sys.PgClearExtendedStats` | `pg_clear_extended_stats(text,text,text,text,bool)` | `void` | 19+ | volatile |
| `sys.PgClearRelationStats` | `pg_clear_relation_stats(text,text)` | `void` | 18+ | volatile |
| `sys.PgClientEncoding` | `pg_client_encoding()` | `name` | all | stable |
| `sys.PgCollationActualVersion` | `pg_collation_actual_version(oid)` | `text` | all | volatile |
| `sys.PgCollationFor` | `pg_collation_for(any)` | `text` | all | stable, may return NULL |
| `sys.PgCollationIsVisible` | `pg_collation_is_visible(oid)` | `bool` | all | stable |
| `sys.PgColumnCompression` | `pg_column_compression(any)` | `text` | all | stable |
| `sys.PgColumnIsUpdatable` | `pg_column_is_updatable(regclass,int2,bool)` | `bool` | all | stable |
| `sys.PgColumnSize` | `pg_column_size(any)` | `int4` | all | stable |
| `sys.PgColumnToastChunkId` | `pg_column_toast_chunk_id(any)` | `oid` | 17+ | stable |
| `sys.PgConfLoadTime` | `pg_conf_load_time()` | `timestamptz` | all | stable |
| `sys.PgConfig` | `pg_config()` | `record` | all | stable |
| `sys.PgControlCheckpointFrom19` | `pg_control_checkpoint()` | `record` | 19+ | volatile |
| `sys.PgControlCheckpointUpTo18` | `pg_control_checkpoint()` | `record` | up to 18 | volatile |
| `sys.PgControlInitUpTo17` | `pg_control_init()` | `record` | up to 17 | volatile |
| `sys.PgControlInitFrom18` | `pg_control_init()` | `record` | 18+ | volatile |
| `sys.PgControlRecovery` | `pg_control_recovery()` | `record` | all | volatile |
| `sys.PgControlSystem` | `pg_control_system()` | `record` | all | volatile |
| `sys.PgConversionIsVisible` | `pg_conversion_is_visible(oid)` | `bool` | all | stable |
| `sys.PgCopyLogicalReplicationSlotNameName` | `pg_copy_logical_replication_slot(name,name)` | `record` | all | volatile |
| `sys.PgCopyLogicalReplicationSlotNameNameBool` | `pg_copy_logical_replication_slot(name,name,bool)` | `record` | all | volatile |
| `sys.PgCopyLogicalReplicationSlotNameNameBoolName` | `pg_copy_logical_replication_slot(name,name,bool,name)` | `record` | all | volatile |
| `sys.PgCopyPhysicalReplicationSlotNameName` | `pg_copy_physical_replication_slot(name,name)` | `record` | all | volatile |
| `sys.PgCopyPhysicalReplicationSlotNameNameBool` | `pg_copy_physical_replication_slot(name,name,bool)` | `record` | all | volatile |
| `sys.PgCreateLogicalReplicationSlotNameNameBoolBool` | `pg_create_logical_replication_slot(name,name,bool,bool)` | `record` | up to 16 | volatile |
| `sys.PgCreateLogicalReplicationSlotNameNameBoolBoolBool` | `pg_create_logical_replication_slot(name,name,bool,bool,bool)` | `record` | 17+ | volatile |
| `sys.PgCreatePhysicalReplicationSlot` | `pg_create_physical_replication_slot(name,bool,bool)` | `record` | all | volatile |
| `sys.PgCreateRestorePoint` | `pg_create_restore_point(text)` | `pg_lsn` | all | volatile |
| `sys.PgCurrentLogfile` | `pg_current_logfile()` | `text` | all | volatile, may return NULL |
| `sys.PgCurrentLogfileText` | `pg_current_logfile(text)` | `text` | all | volatile, may return NULL |
| `sys.PgCurrentSnapshot` | `pg_current_snapshot()` | `pg_snapshot` | all | stable |
| `sys.PgCurrentWalFlushLsn` | `pg_current_wal_flush_lsn()` | `pg_lsn` | all | volatile |
| `sys.PgCurrentWalInsertLsn` | `pg_current_wal_insert_lsn()` | `pg_lsn` | all | volatile |
| `sys.PgCurrentWalLsn` | `pg_current_wal_lsn()` | `pg_lsn` | all | volatile |
| `sys.PgCurrentXactId` | `pg_current_xact_id()` | `xid8` | all | stable |
| `sys.PgCurrentXactIdIfAssigned` | `pg_current_xact_id_if_assigned()` | `xid8` | all | stable |
| `sys.PgCursor` | `pg_cursor()` | `record` | all | stable |
| `sys.PgDatabaseCollationActualVersion` | `pg_database_collation_actual_version(oid)` | `text` | all | volatile |
| `sys.PgDatabaseSizeName` | `pg_database_size(name)` | `int8` | all | volatile |
| `sys.PgDatabaseSizeOid` | `pg_database_size(oid)` | `int8` | all | volatile |
| `sys.PgDependenciesSend` | `pg_dependencies_send(pg_dependencies)` | `bytea` | all | stable |
| `sys.PgDescribeObject` | `pg_describe_object(oid,oid,int4)` | `text` | all | stable |
| `sys.PgDropReplicationSlot` | `pg_drop_replication_slot(name)` | `void` | all | volatile |
| `sys.PgEncodingMaxLength` | `pg_encoding_max_length(int4)` | `int4` | all | immutable |
| `sys.PgEncodingToChar` | `pg_encoding_to_char(int4)` | `name` | all | stable |
| `sys.PgEventTriggerDroppedObjects` | `pg_event_trigger_dropped_objects()` | `record` | all | stable |
| `sys.PgEventTriggerTableRewriteOid` | `pg_event_trigger_table_rewrite_oid()` | `oid` | all | stable |
| `sys.PgEventTriggerTableRewriteReason` | `pg_event_trigger_table_rewrite_reason()` | `int4` | all | stable |
| `sys.PgExportSnapshot` | `pg_export_snapshot()` | `text` | all | volatile |
| `sys.PgExtensionConfigDump` | `pg_extension_config_dump(regclass,text)` | `void` | all | volatile |
| `sys.PgExtensionUpdatePaths` | `pg_extension_update_paths(name)` | `record` | all | stable |
| `sys.PgFilenodeRelation` | `pg_filenode_relation(oid,oid)` | `regclass` | all | stable |
| `sys.PgFunctionIsVisible` | `pg_function_is_visible(oid)` | `bool` | all | stable |
| `sys.PgGetAcl` | `pg_get_acl(oid,oid,int4)` | `_aclitem` | 18+ | stable |
| `sys.PgGetAios` | `pg_get_aios()` | `record` | 18+ | volatile |
| `sys.PgGetBackendMemoryContextsUpTo17` | `pg_get_backend_memory_contexts()` | `record` | up to 17 | volatile |
| `sys.PgGetBackendMemoryContextsFrom18` | `pg_get_backend_memory_contexts()` | `record` | 18+ | volatile |
| `sys.PgGetCatalogForeignKeys` | `pg_get_catalog_foreign_keys()` | `record` | all | stable |
| `sys.PgGetConstraintdefOid` | `pg_get_constraintdef(oid)` | `text` | all | stable, may return NULL |
| `sys.PgGetConstraintdefOidBool` | `pg_get_constraintdef(oid,bool)` | `text` | all | stable, may return NULL |
| `sys.PgGetDsmRegistryAllocations` | `pg_get_dsm_registry_allocations()` | `record` | 19+ | volatile |
| `sys.PgGetExprPgNodeTreeOid` | `pg_get_expr(pg_node_tree,oid)` | `text` | all | stable, may return NULL |
| `sys.PgGetExprPgNodeTreeOidBool` | `pg_get_expr(pg_node_tree,oid,bool)` | `text` | all | stable, may return NULL |
| `sys.PgGetFunctionArgDefault` | `pg_get_function_arg_default(oid,int4)` | `text` | all | stable |
| `sys.PgGetFunctionArguments` | `pg_get_function_arguments(oid)` | `text` | all | stable |
| `sys.PgGetFunctionIdentityArguments` | `pg_get_function_identity_arguments(oid)` | `text` | all | stable |
| `sys.PgGetFunctionResult` | `pg_get_function_result(oid)` | `text` | all | stable |
| `sys.PgGetFunctionSqlbody` | `pg_get_function_sqlbody(oid)` | `text` | all | stable |
| `sys.PgGetFunctiondef` | `pg_get_functiondef(oid)` | `text` | all | stable, may return NULL |
| `sys.PgGetIndexdefOid` | `pg_get_indexdef(oid)` | `text` | all | stable, may return NULL |
| `sys.PgGetIndexdefOidInt4Bool` | `pg_get_indexdef(oid,int4,bool)` | `text` | all | stable, may return NULL |
| `sys.PgGetKeywords` | `pg_get_keywords()` | `record` | all | stable |
| `sys.PgGetLoadedModules` | `pg_get_loaded_modules()` | `record` | 18+ | volatile |
| `sys.PgGetMultixactMembers` | `pg_get_multixact_members(xid)` | `record` | all | volatile |
| `sys.PgGetMultixactStats` | `pg_get_multixact_stats()` | `record` | 19+ | volatile |
| `sys.PgGetObjectAddress` | `pg_get_object_address(text,_text,_text)` | `record` | all | stable |
| `sys.PgGetPartitionConstraintdef` | `pg_get_partition_constraintdef(oid)` | `text` | all | stable |
| `sys.PgGetPartkeydef` | `pg_get_partkeydef(oid)` | `text` | all | stable |
| `sys.PgGetPublicationSequences` | `pg_get_publication_sequences(text)` | `oid` | 19+ | stable |
| `sys.PgGetPublicationTablesTextArray` | `pg_get_publication_tables(_text)` | `record` | all | stable |
| `sys.PgGetPublicationTablesTextArrayOid` | `pg_get_publication_tables(_text,oid)` | `record` | 19+ | stable |
| `sys.PgGetReplicaIdentityIndex` | `pg_get_replica_identity_index(regclass)` | `regclass` | all | stable |
| `sys.PgGetReplicationSlotsUpTo16` | `pg_get_replication_slots()` | `record` | up to 16 | stable |
| `sys.PgGetReplicationSlotsV17` | `pg_get_replication_slots()` | `record` | 17–17 | stable |
| `sys.PgGetReplicationSlotsV18` | `pg_get_replication_slots()` | `record` | 18–18 | stable |
| `sys.PgGetReplicationSlotsFrom19` | `pg_get_replication_slots()` | `record` | 19+ | stable |
| `sys.PgGetRuledefOid` | `pg_get_ruledef(oid)` | `text` | all | stable |
| `sys.PgGetRuledefOidBool` | `pg_get_ruledef(oid,bool)` | `text` | all | stable |
| `sys.PgGetSequenceDataRegclassV18` | `pg_get_sequence_data(regclass)` | `record` | 18–18 | volatile |
| `sys.PgGetSequenceDataRegclassFrom19` | `pg_get_sequence_data(regclass)` | `record` | 19+ | volatile |
| `sys.PgGetSerialSequence` | `pg_get_serial_sequence(text,text)` | `text` | all | stable, may return NULL |
| `sys.PgGetShmemAllocations` | `pg_get_shmem_allocations()` | `record` | all | volatile |
| `sys.PgGetShmemAllocationsNuma` | `pg_get_shmem_allocations_numa()` | `record` | 18+ | volatile |
| `sys.PgGetStatisticsobjdef` | `pg_get_statisticsobjdef(oid)` | `text` | all | stable |
| `sys.PgGetStatisticsobjdefColumns` | `pg_get_statisticsobjdef_columns(oid)` | `text` | all | stable |
| `sys.PgGetStatisticsobjdefExpressions` | `pg_get_statisticsobjdef_expressions(oid)` | `_text` | all | stable |
| `sys.PgGetTriggerdefOid` | `pg_get_triggerdef(oid)` | `text` | all | stable, may return NULL |
| `sys.PgGetTriggerdefOidBool` | `pg_get_triggerdef(oid,bool)` | `text` | all | stable, may return NULL |
| `sys.PgGetUserbyid` | `pg_get_userbyid(oid)` | `name` | all | stable |
| `sys.PgGetViewdefOid` | `pg_get_viewdef(oid)` | `text` | all | stable, may return NULL |
| `sys.PgGetViewdefOidBool` | `pg_get_viewdef(oid,bool)` | `text` | all | stable, may return NULL |
| `sys.PgGetViewdefOidInt4` | `pg_get_viewdef(oid,int4)` | `text` | all | stable, may return NULL |
| `sys.PgGetViewdefText` | `pg_get_viewdef(text)` | `text` | all | stable, may return NULL |
| `sys.PgGetViewdefTextBool` | `pg_get_viewdef(text,bool)` | `text` | all | stable, may return NULL |
| `sys.PgGetWaitEvents` | `pg_get_wait_events()` | `record` | 17+ | volatile |
| `sys.PgGetWalReplayPauseState` | `pg_get_wal_replay_pause_state()` | `text` | all | volatile |
| `sys.PgGetWalResourceManagers` | `pg_get_wal_resource_managers()` | `record` | all | volatile |
| `sys.PgGetWalSummarizerState` | `pg_get_wal_summarizer_state()` | `record` | 17+ | volatile |
| `sys.PgHasRoleNameNameText` | `pg_has_role(name,name,text)` | `bool` | all | stable |
| `sys.PgHasRoleNameOidText` | `pg_has_role(name,oid,text)` | `bool` | all | stable |
| `sys.PgHasRoleNameText` | `pg_has_role(name,text)` | `bool` | all | stable |
| `sys.PgHasRoleOidNameText` | `pg_has_role(oid,name,text)` | `bool` | all | stable |
| `sys.PgHasRoleOidOidText` | `pg_has_role(oid,oid,text)` | `bool` | all | stable |
| `sys.PgHasRoleOidText` | `pg_has_role(oid,text)` | `bool` | all | stable |
| `sys.PgHbaFileRules` | `pg_hba_file_rules()` | `record` | all | volatile |
| `sys.PgIdentFileMappings` | `pg_ident_file_mappings()` | `record` | all | volatile |
| `sys.PgIdentifyObject` | `pg_identify_object(oid,oid,int4)` | `record` | all | stable |
| `sys.PgIdentifyObjectAsAddress` | `pg_identify_object_as_address(oid,oid,int4)` | `record` | all | stable |
| `sys.PgImportSystemCollations` | `pg_import_system_collations(regnamespace)` | `int4` | all | volatile |
| `sys.PgIndexColumnHasProperty` | `pg_index_column_has_property(regclass,int4,text)` | `bool` | all | stable |
| `sys.PgIndexHasProperty` | `pg_index_has_property(regclass,text)` | `bool` | all | stable |
| `sys.PgIndexamHasProperty` | `pg_indexam_has_property(oid,text)` | `bool` | all | stable |
| `sys.PgIndexamProgressPhasename` | `pg_indexam_progress_phasename(oid,int8)` | `text` | all | immutable |
| `sys.PgIndexesSize` | `pg_indexes_size(regclass)` | `int8` | all | volatile |
| `sys.PgInputErrorInfo` | `pg_input_error_info(text,text)` | `record` | all | stable |
| `sys.PgInputIsValid` | `pg_input_is_valid(text,text)` | `bool` | all | stable |
| `sys.PgIsInRecovery` | `pg_is_in_recovery()` | `bool` | all | volatile |
| `sys.PgIsOtherTempSchema` | `pg_is_other_temp_schema(oid)` | `bool` | all | stable |
| `sys.PgIsWalReplayPaused` | `pg_is_wal_replay_paused()` | `bool` | all | volatile |
| `sys.PgIsolationTestSessionIsBlocked` | `pg_isolation_test_session_is_blocked(int4,_int4)` | `bool` | all | volatile |
| `sys.PgJitAvailable` | `pg_jit_available()` | `bool` | all | volatile |
| `sys.PgLastCommittedXact` | `pg_last_committed_xact()` | `record` | all | volatile |
| `sys.PgLastWalReceiveLsn` | `pg_last_wal_receive_lsn()` | `pg_lsn` | all | volatile, may return NULL |
| `sys.PgLastWalReplayLsn` | `pg_last_wal_replay_lsn()` | `pg_lsn` | all | volatile, may return NULL |
| `sys.PgLastXactReplayTimestamp` | `pg_last_xact_replay_timestamp()` | `timestamptz` | all | volatile, may return NULL |
| `sys.PgListeningChannels` | `pg_listening_channels()` | `text` | all | stable |
| `sys.PgLockStatus` | `pg_lock_status()` | `record` | all | volatile |
| `sys.PgLogBackendMemoryContexts` | `pg_log_backend_memory_contexts(int4)` | `bool` | all | volatile |
| `sys.PgLogStandbySnapshot` | `pg_log_standby_snapshot()` | `pg_lsn` | all | volatile |
| `sys.PgLogicalEmitMessageBoolTextBytea` | `pg_logical_emit_message(bool,text,bytea)` | `pg_lsn` | up to 16 | volatile |
| `sys.PgLogicalEmitMessageBoolTextByteaBool` | `pg_logical_emit_message(bool,text,bytea,bool)` | `pg_lsn` | 17+ | volatile |
| `sys.PgLogicalEmitMessageBoolTextText` | `pg_logical_emit_message(bool,text,text)` | `pg_lsn` | up to 16 | volatile |
| `sys.PgLogicalEmitMessageBoolTextTextBool` | `pg_logical_emit_message(bool,text,text,bool)` | `pg_lsn` | 17+ | volatile |
| `sys.PgLogicalSlotGetBinaryChanges` | `pg_logical_slot_get_binary_changes(name,pg_lsn,int4,_text)` | `record` | all | volatile |
| `sys.PgLogicalSlotGetChanges` | `pg_logical_slot_get_changes(name,pg_lsn,int4,_text)` | `record` | all | volatile |
| `sys.PgLogicalSlotPeekBinaryChanges` | `pg_logical_slot_peek_binary_changes(name,pg_lsn,int4,_text)` | `record` | all | volatile |
| `sys.PgLogicalSlotPeekChanges` | `pg_logical_slot_peek_changes(name,pg_lsn,int4,_text)` | `record` | all | volatile |
| `sys.PgLsArchiveStatusdir` | `pg_ls_archive_statusdir()` | `record` | all | volatile |
| `sys.PgLsDirText` | `pg_ls_dir(text)` | `text` | all | volatile |
| `sys.PgLsDirTextBoolBool` | `pg_ls_dir(text,bool,bool)` | `text` | all | volatile |
| `sys.PgLsLogdir` | `pg_ls_logdir()` | `record` | all | volatile |
| `sys.PgLsLogicalmapdir` | `pg_ls_logicalmapdir()` | `record` | all | volatile |
| `sys.PgLsLogicalsnapdir` | `pg_ls_logicalsnapdir()` | `record` | all | volatile |
| `sys.PgLsReplslotdir` | `pg_ls_replslotdir(text)` | `record` | all | volatile |
| `sys.PgLsSummariesdir` | `pg_ls_summariesdir()` | `record` | 18+ | volatile |
| `sys.PgLsTmpdir` | `pg_ls_tmpdir()` | `record` | all | volatile |
| `sys.PgLsTmpdirOid` | `pg_ls_tmpdir(oid)` | `record` | all | volatile |
| `sys.PgLsWaldir` | `pg_ls_waldir()` | `record` | all | volatile |
| `sys.PgLsn` | `pg_lsn(numeric)` | `pg_lsn` | all | immutable |
| `sys.PgLsnCmp` | `pg_lsn_cmp(pg_lsn,pg_lsn)` | `int4` | all | immutable |
| `sys.PgLsnHash` | `pg_lsn_hash(pg_lsn)` | `int4` | all | immutable |
| `sys.PgLsnHashExtended` | `pg_lsn_hash_extended(pg_lsn,int8)` | `int8` | all | immutable |
| `sys.PgLsnLarger` | `pg_lsn_larger(pg_lsn,pg_lsn)` | `pg_lsn` | all | immutable |
| `sys.PgLsnSend` | `pg_lsn_send(pg_lsn)` | `bytea` | all | immutable |
| `sys.PgLsnSmaller` | `pg_lsn_smaller(pg_lsn,pg_lsn)` | `pg_lsn` | all | immutable |
| `sys.PgMcvListItems` | `pg_mcv_list_items(pg_mcv_list)` | `record` | all | stable |
| `sys.PgMcvListSend` | `pg_mcv_list_send(pg_mcv_list)` | `bytea` | all | stable |
| `sys.PgMyTempSchema` | `pg_my_temp_schema()` | `oid` | all | stable |
| `sys.PgNdistinctSend` | `pg_ndistinct_send(pg_ndistinct)` | `bytea` | all | stable |
| `sys.PgNextoid` | `pg_nextoid(regclass,name,regclass)` | `oid` | all | volatile |
| `sys.PgNodeTreeSend` | `pg_node_tree_send(pg_node_tree)` | `bytea` | all | stable |
| `sys.PgNotificationQueueUsage` | `pg_notification_queue_usage()` | `float8` | all | volatile |
| `sys.PgNotify` | `pg_notify(text,text)` | `void` | all | volatile |
| `sys.PgNumaAvailable` | `pg_numa_available()` | `bool` | 18+ | stable |
| `sys.PgOpclassIsVisible` | `pg_opclass_is_visible(oid)` | `bool` | all | stable |
| `sys.PgOperatorIsVisible` | `pg_operator_is_visible(oid)` | `bool` | all | stable |
| `sys.PgOpfamilyIsVisible` | `pg_opfamily_is_visible(oid)` | `bool` | all | stable |
| `sys.PgOptionsToTable` | `pg_options_to_table(_text)` | `record` | all | stable |
| `sys.PgPartitionAncestors` | `pg_partition_ancestors(regclass)` | `regclass` | all | volatile |
| `sys.PgPartitionRoot` | `pg_partition_root(regclass)` | `regclass` | all | immutable |
| `sys.PgPartitionTree` | `pg_partition_tree(regclass)` | `record` | all | volatile |
| `sys.PgPostmasterStartTime` | `pg_postmaster_start_time()` | `timestamptz` | all | stable |
| `sys.PgPreparedStatement` | `pg_prepared_statement()` | `record` | all | stable |
| `sys.PgPreparedXact` | `pg_prepared_xact()` | `record` | all | volatile |
| `sys.PgPromote` | `pg_promote(bool,int4)` | `bool` | all | volatile |
| `sys.PgReadBinaryFileText` | `pg_read_binary_file(text)` | `bytea` | all | volatile |
| `sys.PgReadBinaryFileTextBool` | `pg_read_binary_file(text,bool)` | `bytea` | all | volatile |
| `sys.PgReadBinaryFileTextInt8Int8` | `pg_read_binary_file(text,int8,int8)` | `bytea` | all | volatile |
| `sys.PgReadBinaryFileTextInt8Int8Bool` | `pg_read_binary_file(text,int8,int8,bool)` | `bytea` | all | volatile |
| `sys.PgReadFileText` | `pg_read_file(text)` | `text` | all | volatile |
| `sys.PgReadFileTextBool` | `pg_read_file(text,bool)` | `text` | all | volatile |
| `sys.PgReadFileTextInt8Int8` | `pg_read_file(text,int8,int8)` | `text` | all | volatile |
| `sys.PgReadFileTextInt8Int8Bool` | `pg_read_file(text,int8,int8,bool)` | `text` | all | volatile |
| `sys.PgReadFileOld` | `pg_read_file_old(text,int8,int8)` | `text` | up to 16 | volatile |
| `sys.PgRelationFilenode` | `pg_relation_filenode(regclass)` | `oid` | all | stable |
| `sys.PgRelationFilepath` | `pg_relation_filepath(regclass)` | `text` | all | stable, may return NULL |
| `sys.PgRelationIsPublishable` | `pg_relation_is_publishable(regclass)` | `bool` | all | stable |
| `sys.PgRelationIsUpdatable` | `pg_relation_is_updatable(regclass,bool)` | `int4` | all | stable |
| `sys.PgRelationSizeRegclass` | `pg_relation_size(regclass)` | `int8` | all | volatile |
| `sys.PgRelationSizeRegclassText` | `pg_relation_size(regclass,text)` | `int8` | all | volatile |
| `sys.PgReloadConf` | `pg_reload_conf()` | `bool` | all | volatile |
| `sys.PgReplicationOriginAdvance` | `pg_replication_origin_advance(text,pg_lsn)` | `void` | all | volatile |
| `sys.PgReplicationOriginCreate` | `pg_replication_origin_create(text)` | `oid` | all | volatile |
| `sys.PgReplicationOriginDrop` | `pg_replication_origin_drop(text)` | `void` | all | volatile |
| `sys.PgReplicationOriginOid` | `pg_replication_origin_oid(text)` | `oid` | all | stable |
| `sys.PgReplicationOriginProgress` | `pg_replication_origin_progress(text,bool)` | `pg_lsn` | all | volatile |
| `sys.PgReplicationOriginSessionIsSetup` | `pg_replication_origin_session_is_setup()` | `bool` | all | volatile |
| `sys.PgReplicationOriginSessionProgress` | `pg_replication_origin_session_progress(bool)` | `pg_lsn` | all | volatile |
| `sys.PgReplicationOriginSessionReset` | `pg_replication_origin_session_reset()` | `void` | all | volatile |
| `sys.PgReplicationOriginSessionSetupText` | `pg_replication_origin_session_setup(text)` | `void` | up to 18 | volatile |
| `sys.PgReplicationOriginSessionSetupTextInt4` | `pg_replication_origin_session_setup(text,int4)` | `void` | 19+ | volatile |
| `sys.PgReplicationOriginXactReset` | `pg_replication_origin_xact_reset()` | `void` | all | volatile |
| `sys.PgReplicationOriginXactSetup` | `pg_replication_origin_xact_setup(pg_lsn,timestamptz)` | `void` | all | volatile |
| `sys.PgReplicationSlotAdvance` | `pg_replication_slot_advance(name,pg_lsn)` | `record` | all | volatile |
| `sys.PgRestoreAttributeStats` | `pg_restore_attribute_stats(any)` | `bool` | 18+ | volatile, may return NULL |
| `sys.PgRestoreExtendedStats` | `pg_restore_extended_stats(any)` | `bool` | 19+ | volatile, may return NULL |
| `sys.PgRestoreRelationStats` | `pg_restore_relation_stats(any)` | `bool` | 18+ | volatile, may return NULL |
| `sys.PgRotateLogfile` | `pg_rotate_logfile()` | `bool` | all | volatile |
| `sys.PgRotateLogfileOld` | `pg_rotate_logfile_old()` | `bool` | up to 16 | volatile |
| `sys.PgSafeSnapshotBlockingPids` | `pg_safe_snapshot_blocking_pids(int4)` | `_int4` | all | volatile |
| `sys.PgSequenceLastValue` | `pg_sequence_last_value(regclass)` | `int8` | all | volatile |
| `sys.PgSequenceParameters` | `pg_sequence_parameters(oid)` | `record` | all | stable |
| `sys.PgSettingsGetFlags` | `pg_settings_get_flags(text)` | `_text` | all | stable |
| `sys.PgShowAllFileSettings` | `pg_show_all_file_settings()` | `record` | all | volatile |
| `sys.PgShowAllSettings` | `pg_show_all_settings()` | `record` | all | stable |
| `sys.PgShowReplicationOriginStatus` | `pg_show_replication_origin_status()` | `record` | all | volatile |
| `sys.PgSizeBytes` | `pg_size_bytes(text)` | `int8` | all | immutable |
| `sys.PgSizePrettyInt8` | `pg_size_pretty(int8)` | `text` | all | immutable |
| `sys.PgSizePrettyNumeric` | `pg_size_pretty(numeric)` | `text` | all | immutable |
| `sys.PgSleep` | `pg_sleep(float8)` | `void` | all | volatile |
| `sys.PgSleepFor` | `pg_sleep_for(interval)` | `void` | all | volatile |
| `sys.PgSleepUntil` | `pg_sleep_until(timestamptz)` | `void` | all | volatile |
| `sys.PgSnapshotSend` | `pg_snapshot_send(pg_snapshot)` | `bytea` | all | immutable |
| `sys.PgSnapshotXip` | `pg_snapshot_xip(pg_snapshot)` | `xid8` | all | immutable |
| `sys.PgSnapshotXmax` | `pg_snapshot_xmax(pg_snapshot)` | `xid8` | all | immutable |
| `sys.PgSnapshotXmin` | `pg_snapshot_xmin(pg_snapshot)` | `xid8` | all | immutable |
| `sys.PgSplitWalfileName` | `pg_split_walfile_name(text)` | `record` | all | stable |
| `sys.PgStatClearSnapshot` | `pg_stat_clear_snapshot()` | `void` | all | volatile |
| `sys.PgStatFileText` | `pg_stat_file(text)` | `record` | all | volatile |
| `sys.PgStatFileTextBool` | `pg_stat_file(text,bool)` | `record` | all | volatile |
| `sys.PgStatForceNextFlush` | `pg_stat_force_next_flush()` | `void` | all | volatile |
| `sys.PgStatGetActivity` | `pg_stat_get_activity(int4)` | `record` | all | stable |
| `sys.PgStatGetAnalyzeCount` | `pg_stat_get_analyze_count(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetArchiver` | `pg_stat_get_archiver()` | `record` | all | stable |
| `sys.PgStatGetAutoanalyzeCount` | `pg_stat_get_autoanalyze_count(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetAutovacuumCount` | `pg_stat_get_autovacuum_count(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetAutovacuumScores` | `pg_stat_get_autovacuum_scores()` | `record` | 19+ | stable |
| `sys.PgStatGetBackendActivity` | `pg_stat_get_backend_activity(int4)` | `text` | all | stable, may return NULL |
| `sys.PgStatGetBackendActivityStart` | `pg_stat_get_backend_activity_start(int4)` | `timestamptz` | all | stable, may return NULL |
| `network.PgStatGetBackendClientAddr` | `pg_stat_get_backend_client_addr(int4)` | `inet` | all | stable, may return NULL |
| `sys.PgStatGetBackendClientPort` | `pg_stat_get_backend_client_port(int4)` | `int4` | all | stable, may return NULL |
| `sys.PgStatGetBackendDbid` | `pg_stat_get_backend_dbid(int4)` | `oid` | all | stable, may return NULL |
| `sys.PgStatGetBackendIdset` | `pg_stat_get_backend_idset()` | `int4` | all | stable |
| `sys.PgStatGetBackendIo` | `pg_stat_get_backend_io(int4)` | `record` | 18+ | volatile |
| `sys.PgStatGetBackendPid` | `pg_stat_get_backend_pid(int4)` | `int4` | all | stable, may return NULL |
| `sys.PgStatGetBackendStart` | `pg_stat_get_backend_start(int4)` | `timestamptz` | all | stable, may return NULL |
| `sys.PgStatGetBackendSubxact` | `pg_stat_get_backend_subxact(int4)` | `record` | all | stable |
| `sys.PgStatGetBackendUserid` | `pg_stat_get_backend_userid(int4)` | `oid` | all | stable, may return NULL |
| `sys.PgStatGetBackendWaitEvent` | `pg_stat_get_backend_wait_event(int4)` | `text` | all | stable, may return NULL |
| `sys.PgStatGetBackendWaitEventType` | `pg_stat_get_backend_wait_event_type(int4)` | `text` | all | stable, may return NULL |
| `sys.PgStatGetBackendWalInt4V18` | `pg_stat_get_backend_wal(int4)` | `record` | 18–18 | volatile |
| `sys.PgStatGetBackendWalInt4From19` | `pg_stat_get_backend_wal(int4)` | `record` | 19+ | volatile |
| `sys.PgStatGetBackendXactStart` | `pg_stat_get_backend_xact_start(int4)` | `timestamptz` | all | stable, may return NULL |
| `sys.PgStatGetBgwriterBufWrittenCheckpoints` | `pg_stat_get_bgwriter_buf_written_checkpoints()` | `int8` | up to 16 | stable, may return NULL |
| `sys.PgStatGetBgwriterBufWrittenClean` | `pg_stat_get_bgwriter_buf_written_clean()` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetBgwriterMaxwrittenClean` | `pg_stat_get_bgwriter_maxwritten_clean()` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetBgwriterRequestedCheckpoints` | `pg_stat_get_bgwriter_requested_checkpoints()` | `int8` | up to 16 | stable, may return NULL |
| `sys.PgStatGetBgwriterStatResetTime` | `pg_stat_get_bgwriter_stat_reset_time()` | `timestamptz` | all | stable, may return NULL |
| `sys.PgStatGetBgwriterTimedCheckpoints` | `pg_stat_get_bgwriter_timed_checkpoints()` | `int8` | up to 16 | stable, may return NULL |
| `sys.PgStatGetBlocksFetched` | `pg_stat_get_blocks_fetched(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetBlocksHit` | `pg_stat_get_blocks_hit(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetBufAlloc` | `pg_stat_get_buf_alloc()` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetBufFsyncBackend` | `pg_stat_get_buf_fsync_backend()` | `int8` | up to 16 | stable, may return NULL |
| `sys.PgStatGetBufWrittenBackend` | `pg_stat_get_buf_written_backend()` | `int8` | up to 16 | stable, may return NULL |
| `sys.PgStatGetCheckpointSyncTime` | `pg_stat_get_checkpoint_sync_time()` | `float8` | up to 16 | stable, may return NULL |
| `sys.PgStatGetCheckpointWriteTime` | `pg_stat_get_checkpoint_write_time()` | `float8` | up to 16 | stable, may return NULL |
| `sys.PgStatGetCheckpointerBuffersWritten` | `pg_stat_get_checkpointer_buffers_written()` | `int8` | 17+ | stable, may return NULL |
| `sys.PgStatGetCheckpointerNumPerformed` | `pg_stat_get_checkpointer_num_performed()` | `int8` | 18+ | stable, may return NULL |
| `sys.PgStatGetCheckpointerNumRequested` | `pg_stat_get_checkpointer_num_requested()` | `int8` | 17+ | stable, may return NULL |
| `sys.PgStatGetCheckpointerNumTimed` | `pg_stat_get_checkpointer_num_timed()` | `int8` | 17+ | stable, may return NULL |
| `sys.PgStatGetCheckpointerRestartpointsPerformed` | `pg_stat_get_checkpointer_restartpoints_performed()` | `int8` | 17+ | stable, may return NULL |
| `sys.PgStatGetCheckpointerRestartpointsRequested` | `pg_stat_get_checkpointer_restartpoints_requested()` | `int8` | 17+ | stable, may return NULL |
| `sys.PgStatGetCheckpointerRestartpointsTimed` | `pg_stat_get_checkpointer_restartpoints_timed()` | `int8` | 17+ | stable, may return NULL |
| `sys.PgStatGetCheckpointerSlruWritten` | `pg_stat_get_checkpointer_slru_written()` | `int8` | 18+ | stable, may return NULL |
| `sys.PgStatGetCheckpointerStatResetTime` | `pg_stat_get_checkpointer_stat_reset_time()` | `timestamptz` | 17+ | stable, may return NULL |
| `sys.PgStatGetCheckpointerSyncTime` | `pg_stat_get_checkpointer_sync_time()` | `float8` | 17+ | stable, may return NULL |
| `sys.PgStatGetCheckpointerWriteTime` | `pg_stat_get_checkpointer_write_time()` | `float8` | 17+ | stable, may return NULL |
| `sys.PgStatGetDbActiveTime` | `pg_stat_get_db_active_time(oid)` | `float8` | all | stable, may return NULL |
| `sys.PgStatGetDbBlkReadTime` | `pg_stat_get_db_blk_read_time(oid)` | `float8` | all | stable, may return NULL |
| `sys.PgStatGetDbBlkWriteTime` | `pg_stat_get_db_blk_write_time(oid)` | `float8` | all | stable, may return NULL |
| `sys.PgStatGetDbBlocksFetched` | `pg_stat_get_db_blocks_fetched(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbBlocksHit` | `pg_stat_get_db_blocks_hit(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbChecksumFailures` | `pg_stat_get_db_checksum_failures(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbChecksumLastFailure` | `pg_stat_get_db_checksum_last_failure(oid)` | `timestamptz` | all | stable, may return NULL |
| `sys.PgStatGetDbConflictAll` | `pg_stat_get_db_conflict_all(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbConflictBufferpin` | `pg_stat_get_db_conflict_bufferpin(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbConflictLock` | `pg_stat_get_db_conflict_lock(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbConflictLogicalslot` | `pg_stat_get_db_conflict_logicalslot(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbConflictSnapshot` | `pg_stat_get_db_conflict_snapshot(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbConflictStartupDeadlock` | `pg_stat_get_db_conflict_startup_deadlock(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbConflictTablespace` | `pg_stat_get_db_conflict_tablespace(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbDeadlocks` | `pg_stat_get_db_deadlocks(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbIdleInTransactionTime` | `pg_stat_get_db_idle_in_transaction_time(oid)` | `float8` | all | stable, may return NULL |
| `sys.PgStatGetDbNumbackends` | `pg_stat_get_db_numbackends(oid)` | `int4` | all | stable, may return NULL |
| `sys.PgStatGetDbParallelWorkersLaunched` | `pg_stat_get_db_parallel_workers_launched(oid)` | `int8` | 18+ | stable, may return NULL |
| `sys.PgStatGetDbParallelWorkersToLaunch` | `pg_stat_get_db_parallel_workers_to_launch(oid)` | `int8` | 18+ | stable, may return NULL |
| `sys.PgStatGetDbSessionTime` | `pg_stat_get_db_session_time(oid)` | `float8` | all | stable, may return NULL |
| `sys.PgStatGetDbSessions` | `pg_stat_get_db_sessions(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbSessionsAbandoned` | `pg_stat_get_db_sessions_abandoned(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbSessionsFatal` | `pg_stat_get_db_sessions_fatal(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbSessionsKilled` | `pg_stat_get_db_sessions_killed(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbStatResetTime` | `pg_stat_get_db_stat_reset_time(oid)` | `timestamptz` | all | stable, may return NULL |
| `sys.PgStatGetDbTempBytes` | `pg_stat_get_db_temp_bytes(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbTempFiles` | `pg_stat_get_db_temp_files(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbTuplesDeleted` | `pg_stat_get_db_tuples_deleted(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbTuplesFetched` | `pg_stat_get_db_tuples_fetched(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbTuplesInserted` | `pg_stat_get_db_tuples_inserted(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbTuplesReturned` | `pg_stat_get_db_tuples_returned(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbTuplesUpdated` | `pg_stat_get_db_tuples_updated(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbXactCommit` | `pg_stat_get_db_xact_commit(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDbXactRollback` | `pg_stat_get_db_xact_rollback(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetDeadTuples` | `pg_stat_get_dead_tuples(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetFunctionCalls` | `pg_stat_get_function_calls(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetFunctionSelfTime` | `pg_stat_get_function_self_time(oid)` | `float8` | all | stable, may return NULL |
| `sys.PgStatGetFunctionStatResetTime` | `pg_stat_get_function_stat_reset_time(oid)` | `timestamptz` | 19+ | stable, may return NULL |
| `sys.PgStatGetFunctionTotalTime` | `pg_stat_get_function_total_time(oid)` | `float8` | all | stable, may return NULL |
| `sys.PgStatGetInsSinceVacuum` | `pg_stat_get_ins_since_vacuum(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetIoFrom18` | `pg_stat_get_io()` | `record` | 18+ | volatile |
| `sys.PgStatGetIoUpTo17` | `pg_stat_get_io()` | `record` | up to 17 | volatile |
| `sys.PgStatGetLastAnalyzeTime` | `pg_stat_get_last_analyze_time(oid)` | `timestamptz` | all | stable, may return NULL |
| `sys.PgStatGetLastAutoanalyzeTime` | `pg_stat_get_last_autoanalyze_time(oid)` | `timestamptz` | all | stable, may return NULL |
| `sys.PgStatGetLastAutovacuumTime` | `pg_stat_get_last_autovacuum_time(oid)` | `timestamptz` | all | stable, may return NULL |
| `sys.PgStatGetLastVacuumTime` | `pg_stat_get_last_vacuum_time(oid)` | `timestamptz` | all | stable, may return NULL |
| `sys.PgStatGetLastscan` | `pg_stat_get_lastscan(oid)` | `timestamptz` | all | stable, may return NULL |
| `sys.PgStatGetLiveTuples` | `pg_stat_get_live_tuples(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetLock` | `pg_stat_get_lock()` | `record` | 19+ | volatile |
| `sys.PgStatGetModSinceAnalyze` | `pg_stat_get_mod_since_analyze(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetNumscans` | `pg_stat_get_numscans(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetProgressInfo` | `pg_stat_get_progress_info(text)` | `record` | all | stable |
| `sys.PgStatGetRecovery` | `pg_stat_get_recovery()` | `record` | 19+ | stable |
| `sys.PgStatGetRecoveryPrefetch` | `pg_stat_get_recovery_prefetch()` | `record` | all | volatile |
| `sys.PgStatGetReplicationSlotTextFrom19` | `pg_stat_get_replication_slot(text)` | `record` | 19+ | stable |
| `sys.PgStatGetReplicationSlotTextUpTo18` | `pg_stat_get_replication_slot(text)` | `record` | up to 18 | stable |
| `sys.PgStatGetSlru` | `pg_stat_get_slru()` | `record` | all | stable |
| `sys.PgStatGetSnapshotTimestamp` | `pg_stat_get_snapshot_timestamp()` | `timestamptz` | all | stable, may return NULL |
| `sys.PgStatGetStatResetTime` | `pg_stat_get_stat_reset_time(oid)` | `timestamptz` | 19+ | stable, may return NULL |
| `sys.PgStatGetSubscriptionOidUpTo16` | `pg_stat_get_subscription(oid)` | `record` | up to 16 | stable |
| `sys.PgStatGetSubscriptionOidFrom17` | `pg_stat_get_subscription(oid)` | `record` | 17+ | stable |
| `sys.PgStatGetSubscriptionStatsOidV18` | `pg_stat_get_subscription_stats(oid)` | `record` | 18–18 | stable |
| `sys.PgStatGetSubscriptionStatsOidUpTo17` | `pg_stat_get_subscription_stats(oid)` | `record` | up to 17 | stable |
| `sys.PgStatGetSubscriptionStatsOidFrom19` | `pg_stat_get_subscription_stats(oid)` | `record` | 19+ | stable |
| `sys.PgStatGetTotalAnalyzeTime` | `pg_stat_get_total_analyze_time(oid)` | `float8` | 18+ | stable, may return NULL |
| `sys.PgStatGetTotalAutoanalyzeTime` | `pg_stat_get_total_autoanalyze_time(oid)` | `float8` | 18+ | stable, may return NULL |
| `sys.PgStatGetTotalAutovacuumTime` | `pg_stat_get_total_autovacuum_time(oid)` | `float8` | 18+ | stable, may return NULL |
| `sys.PgStatGetTotalVacuumTime` | `pg_stat_get_total_vacuum_time(oid)` | `float8` | 18+ | stable, may return NULL |
| `sys.PgStatGetTuplesDeleted` | `pg_stat_get_tuples_deleted(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetTuplesFetched` | `pg_stat_get_tuples_fetched(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetTuplesHotUpdated` | `pg_stat_get_tuples_hot_updated(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetTuplesInserted` | `pg_stat_get_tuples_inserted(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetTuplesNewpageUpdated` | `pg_stat_get_tuples_newpage_updated(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetTuplesReturned` | `pg_stat_get_tuples_returned(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetTuplesUpdated` | `pg_stat_get_tuples_updated(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetVacuumCount` | `pg_stat_get_vacuum_count(oid)` | `int8` | all | stable, may return NULL |
| `sys.PgStatGetWalV18` | `pg_stat_get_wal()` | `record` | 18–18 | stable |
| `sys.PgStatGetWalUpTo17` | `pg_stat_get_wal()` | `record` | up to 17 | stable |
| `sys.PgStatGetWalFrom19` | `pg_stat_get_wal()` | `record` | 19+ | stable |
| `sys.PgStatGetWalReceiver` | `pg_stat_get_wal_receiver()` | `record` | all | stable |
| `sys.PgStatGetWalSenders` | `pg_stat_get_wal_senders()` | `record` | all | stable |
| `sys.PgStatGetXactBlocksFetched` | `pg_stat_get_xact_blocks_fetched(oid)` | `int8` | all | volatile, may return NULL |
| `sys.PgStatGetXactBlocksHit` | `pg_stat_get_xact_blocks_hit(oid)` | `int8` | all | volatile, may return NULL |
| `sys.PgStatGetXactFunctionCalls` | `pg_stat_get_xact_function_calls(oid)` | `int8` | all | volatile, may return NULL |
| `sys.PgStatGetXactFunctionSelfTime` | `pg_stat_get_xact_function_self_time(oid)` | `float8` | all | volatile, may return NULL |
| `sys.PgStatGetXactFunctionTotalTime` | `pg_stat_get_xact_function_total_time(oid)` | `float8` | all | volatile, may return NULL |
| `sys.PgStatGetXactNumscans` | `pg_stat_get_xact_numscans(oid)` | `int8` | all | volatile, may return NULL |
| `sys.PgStatGetXactTuplesDeleted` | `pg_stat_get_xact_tuples_deleted(oid)` | `int8` | all | volatile, may return NULL |
| `sys.PgStatGetXactTuplesFetched` | `pg_stat_get_xact_tuples_fetched(oid)` | `int8` | all | volatile, may return NULL |
| `sys.PgStatGetXactTuplesHotUpdated` | `pg_stat_get_xact_tuples_hot_updated(oid)` | `int8` | all | volatile, may return NULL |
| `sys.PgStatGetXactTuplesInserted` | `pg_stat_get_xact_tuples_inserted(oid)` | `int8` | all | volatile, may return NULL |
| `sys.PgStatGetXactTuplesNewpageUpdated` | `pg_stat_get_xact_tuples_newpage_updated(oid)` | `int8` | all | volatile, may return NULL |
| `sys.PgStatGetXactTuplesReturned` | `pg_stat_get_xact_tuples_returned(oid)` | `int8` | all | volatile, may return NULL |
| `sys.PgStatGetXactTuplesUpdated` | `pg_stat_get_xact_tuples_updated(oid)` | `int8` | all | volatile, may return NULL |
| `sys.PgStatHaveStatsTextOidInt8` | `pg_stat_have_stats(text,oid,int8)` | `bool` | 18+ | volatile |
| `sys.PgStatHaveStatsTextOidOid` | `pg_stat_have_stats(text,oid,oid)` | `bool` | up to 17 | volatile |
| `sys.PgStatReset` | `pg_stat_reset()` | `void` | all | volatile |
| `sys.PgStatResetBackendStats` | `pg_stat_reset_backend_stats(int4)` | `void` | 18+ | volatile |
| `sys.PgStatResetReplicationSlot` | `pg_stat_reset_replication_slot(text)` | `void` | all | volatile |
| `sys.PgStatResetShared` | `pg_stat_reset_shared(text)` | `void` | all | volatile |
| `sys.PgStatResetSingleFunctionCounters` | `pg_stat_reset_single_function_counters(oid)` | `void` | all | volatile |
| `sys.PgStatResetSingleTableCounters` | `pg_stat_reset_single_table_counters(oid)` | `void` | all | volatile |
| `sys.PgStatResetSlru` | `pg_stat_reset_slru(text)` | `void` | all | volatile |
| `sys.PgStatResetSubscriptionStats` | `pg_stat_reset_subscription_stats(oid)` | `void` | all | volatile |
| `sys.PgStatisticsObjIsVisible` | `pg_statistics_obj_is_visible(oid)` | `bool` | all | stable |
| `sys.PgStopMakingPinnedObjects` | `pg_stop_making_pinned_objects()` | `void` | all | volatile |
| `sys.PgSwitchWal` | `pg_switch_wal()` | `pg_lsn` | all | volatile |
| `sys.PgSyncReplicationSlots` | `pg_sync_replication_slots()` | `void` | 17+ | volatile |
| `sys.PgTableIsVisible` | `pg_table_is_visible(oid)` | `bool` | all | stable |
| `sys.PgTableSize` | `pg_table_size(regclass)` | `int8` | all | volatile |
| `sys.PgTablespaceDatabases` | `pg_tablespace_databases(oid)` | `oid` | all | stable |
| `sys.PgTablespaceLocation` | `pg_tablespace_location(oid)` | `text` | all | stable, may return NULL |
| `sys.PgTablespaceSizeName` | `pg_tablespace_size(name)` | `int8` | all | volatile |
| `sys.PgTablespaceSizeOid` | `pg_tablespace_size(oid)` | `int8` | all | volatile |
| `sys.PgTerminateBackend` | `pg_terminate_backend(int4,int8)` | `bool` | all | volatile |
| `sys.PgTimezoneAbbrevs` | `pg_timezone_abbrevs()` | `record` | up to 17 | stable |
| `sys.PgTimezoneAbbrevsAbbrevs` | `pg_timezone_abbrevs_abbrevs()` | `record` | 18+ | stable |
| `sys.PgTimezoneAbbrevsZone` | `pg_timezone_abbrevs_zone()` | `record` | 18+ | stable |
| `sys.PgTimezoneNames` | `pg_timezone_names()` | `record` | all | stable |
| `sys.PgTotalRelationSize` | `pg_total_relation_size(regclass)` | `int8` | all | volatile |
| `sys.PgTriggerDepth` | `pg_trigger_depth()` | `int4` | all | stable |
| `sys.PgTryAdvisoryLockInt4Int4` | `pg_try_advisory_lock(int4,int4)` | `bool` | all | volatile |
| `sys.PgTryAdvisoryLockInt8` | `pg_try_advisory_lock(int8)` | `bool` | all | volatile |
| `sys.PgTryAdvisoryLockSharedInt4Int4` | `pg_try_advisory_lock_shared(int4,int4)` | `bool` | all | volatile |
| `sys.PgTryAdvisoryLockSharedInt8` | `pg_try_advisory_lock_shared(int8)` | `bool` | all | volatile |
| `sys.PgTryAdvisoryXactLockInt4Int4` | `pg_try_advisory_xact_lock(int4,int4)` | `bool` | all | volatile |
| `sys.PgTryAdvisoryXactLockInt8` | `pg_try_advisory_xact_lock(int8)` | `bool` | all | volatile |
| `sys.PgTryAdvisoryXactLockSharedInt4Int4` | `pg_try_advisory_xact_lock_shared(int4,int4)` | `bool` | all | volatile |
| `sys.PgTryAdvisoryXactLockSharedInt8` | `pg_try_advisory_xact_lock_shared(int8)` | `bool` | all | volatile |
| `sys.PgTsConfigIsVisible` | `pg_ts_config_is_visible(oid)` | `bool` | all | stable |
| `sys.PgTsDictIsVisible` | `pg_ts_dict_is_visible(oid)` | `bool` | all | stable |
| `sys.PgTsParserIsVisible` | `pg_ts_parser_is_visible(oid)` | `bool` | all | stable |
| `sys.PgTsTemplateIsVisible` | `pg_ts_template_is_visible(oid)` | `bool` | all | stable |
| `sys.PgTypeIsVisible` | `pg_type_is_visible(oid)` | `bool` | all | stable |
| `sys.PgTypeof` | `pg_typeof(any)` | `regtype` | all | stable, may return NULL |
| `sys.PgVisibleInSnapshot` | `pg_visible_in_snapshot(xid8,pg_snapshot)` | `bool` | all | immutable |
| `sys.PgWalLsnDiff` | `pg_wal_lsn_diff(pg_lsn,pg_lsn)` | `numeric` | all | immutable |
| `sys.PgWalReplayPause` | `pg_wal_replay_pause()` | `void` | all | volatile |
| `sys.PgWalReplayResume` | `pg_wal_replay_resume()` | `void` | all | volatile |
| `sys.PgWalSummaryContents` | `pg_wal_summary_contents(int8,pg_lsn,pg_lsn)` | `record` | 17+ | volatile |
| `sys.PgWalfileName` | `pg_walfile_name(pg_lsn)` | `text` | all | immutable |
| `sys.PgWalfileNameOffset` | `pg_walfile_name_offset(pg_lsn)` | `record` | all | immutable |
| `sys.PgXactCommitTimestamp` | `pg_xact_commit_timestamp(xid)` | `timestamptz` | all | volatile |
| `sys.PgXactCommitTimestampOrigin` | `pg_xact_commit_timestamp_origin(xid)` | `record` | all | volatile |
| `sys.PgXactStatus` | `pg_xact_status(xid8)` | `text` | all | volatile |
| `fts.PhrasetoTsqueryRegconfigText` | `phraseto_tsquery(regconfig,text)` | `tsquery` | all | immutable |
| `fts.PhrasetoTsqueryText` | `phraseto_tsquery(text)` | `tsquery` | all | stable |
| `num.Pi` | `pi()` | `float8` | all | immutable |
| `fts.PlaintoTsqueryRegconfigText` | `plainto_tsquery(regconfig,text)` | `tsquery` | all | immutable |
| `fts.PlaintoTsqueryText` | `plainto_tsquery(text)` | `tsquery` | all | stable |
| `sys.PlpgsqlValidator` | `plpgsql_validator(oid)` | `void` | all | volatile |
| `geo.PointSend` | `point_send(point)` | `bytea` | all | immutable |
| `geo.PolyAbove` | `poly_above(polygon,polygon)` | `bool` | all | immutable |
| `geo.PolyBelow` | `poly_below(polygon,polygon)` | `bool` | all | immutable |
| `geo.PolyCenter` | `poly_center(polygon)` | `point` | all | immutable |
| `geo.PolyContain` | `poly_contain(polygon,polygon)` | `bool` | all | immutable |
| `geo.PolyContainPt` | `poly_contain_pt(polygon,point)` | `bool` | all | immutable |
| `geo.PolyContained` | `poly_contained(polygon,polygon)` | `bool` | all | immutable |
| `geo.PolyDistance` | `poly_distance(polygon,polygon)` | `float8` | all | immutable |
| `geo.PolyLeft` | `poly_left(polygon,polygon)` | `bool` | all | immutable |
| `geo.PolyNpoints` | `poly_npoints(polygon)` | `int4` | all | immutable |
| `geo.PolyOverabove` | `poly_overabove(polygon,polygon)` | `bool` | all | immutable |
| `geo.PolyOverbelow` | `poly_overbelow(polygon,polygon)` | `bool` | all | immutable |
| `geo.PolyOverlap` | `poly_overlap(polygon,polygon)` | `bool` | all | immutable |
| `geo.PolyOverleft` | `poly_overleft(polygon,polygon)` | `bool` | all | immutable |
| `geo.PolyOverright` | `poly_overright(polygon,polygon)` | `bool` | all | immutable |
| `geo.PolyRight` | `poly_right(polygon,polygon)` | `bool` | all | immutable |
| `geo.PolySame` | `poly_same(polygon,polygon)` | `bool` | all | immutable |
| `geo.PolySend` | `poly_send(polygon)` | `bytea` | all | immutable |
| `geo.Popen` | `popen(path)` | `path` | all | immutable |
| `binary.PositionBitBit` | `position(bit,bit)` | `int4` | all | immutable |
| `binary.PositionByteaBytea` | `position(bytea,bytea)` | `int4` | all | immutable |
| `text.Position` | `position(text,text)` | `int4` | all | immutable |
| `sys.PostgresqlFdwValidator` | `postgresql_fdw_validator(_text,oid)` | `bool` | all | immutable |
| `num.PowFloat8Float8` | `pow(float8,float8)` | `float8` | all | immutable |
| `num.PowNumericNumeric` | `pow(numeric,numeric)` | `numeric` | all | immutable |
| `num.PowerFloat8Float8` | `power(float8,float8)` | `float8` | all | immutable |
| `num.PowerNumericNumeric` | `power(numeric,numeric)` | `numeric` | all | immutable |
| `geo.PtContainedCircle` | `pt_contained_circle(point,circle)` | `bool` | all | immutable |
| `geo.PtContainedPoly` | `pt_contained_poly(point,polygon)` | `bool` | all | immutable |

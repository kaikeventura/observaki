-- 0003_ingest_dedupe.sql — retry deduplication for the ClickHouse writers.
--
-- AD-18: the writer acknowledges a batch only after ClickHouse confirms the
-- flush. If the writer crashes after the insert commits but before its local
-- checkpoint advances, it replays the same batch. Every spooled batch carries
-- a stable insert_deduplication_token, so ClickHouse drops the replayed block
-- instead of storing it twice.
--
-- Non-replicated MergeTree tables do not retain deduplication hashes by
-- default. The local single-node profile keeps the last 10,000 blocks, which
-- is larger than the 1-hour benchmark batch count. A cluster uses
-- ReplicatedMergeTree and its own replicated_deduplication_window; the writer
-- contract (stable token per batch) does not change.
--
-- This migration only changes merge-tree settings; it does not rewrite data
-- and is additive for existing tables (AD-9).
ALTER TABLE otel_logs MODIFY SETTING non_replicated_deduplication_window = 10000;
ALTER TABLE otel_traces MODIFY SETTING non_replicated_deduplication_window = 10000;

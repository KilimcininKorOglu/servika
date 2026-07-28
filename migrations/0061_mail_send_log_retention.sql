-- 0061 - ts index for the mail_send_log retention prune.
-- The existing indexes (mailbox_id, ts) and (domain_id, ts) are composite; the
-- periodic DELETE keyed on ts alone cannot use them and would scan the whole
-- table. A standalone ts index lets the hourly batch prune seek instead.
ALTER TABLE mail_send_log ADD INDEX ix_sendlog_ts (ts);

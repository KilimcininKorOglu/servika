-- 0082 - measured mailbox usage, so a quota can be shown next to what fills it.
--
-- mailboxes.quota_bytes is the LIMIT and has always been there; nothing recorded
-- how much of it was in use, so the panel could show a mailbox its ceiling but
-- never its level. Dovecot keeps that number in a maildirsize file inside each
-- Maildir, which is not reachable from a page render and goes stale whenever
-- messages arrive by any route other than Dovecot itself.
--
-- The measurement is stored rather than taken per request because a domain with
-- fifty mailboxes would otherwise walk fifty directory trees to draw one list.
-- usage_checked_at is NULL until the first measurement, so "never measured" stays
-- distinguishable from "measured, and it was empty".
ALTER TABLE mailboxes
  ADD COLUMN used_bytes BIGINT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN usage_checked_at TIMESTAMP NULL DEFAULT NULL;

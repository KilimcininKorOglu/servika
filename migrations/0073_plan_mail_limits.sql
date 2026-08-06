-- Plan-level mail send limits, and the flag that protects a hand-set mailbox.
--
-- 0 on the plan means the plan does not override: the mailbox keeps the
-- built-in 100/500 default. That is deliberate, because 0 already means
-- "unlimited" on the mailbox row, and a new plan starting at 0 must not quietly
-- remove the spam protection every mailbox has today.
ALTER TABLE service_plans
  ADD COLUMN mail_send_limit_hour INT NOT NULL DEFAULT 0,
  ADD COLUMN mail_send_limit_day  INT NOT NULL DEFAULT 0;

-- send_limits_manual marks a mailbox whose limits an operator set by hand. Plan
-- realignment skips those rows, because a plan default must not undo a decision
-- somebody made about one mailbox.
ALTER TABLE mailboxes
  ADD COLUMN send_limits_manual TINYINT(1) NOT NULL DEFAULT 0;

-- Existing rows predate the flag. Until now nothing but the per-mailbox screen
-- could write these two columns, so a row that still holds the schema defaults
-- was never touched by hand, and any other value was. Marking the second group
-- manual keeps the first plan change from erasing settings that were chosen
-- deliberately.
UPDATE mailboxes
   SET send_limits_manual = 1
 WHERE send_limit_hour <> 100 OR send_limit_day <> 500;

-- 002_rule_adjust: steps that change a number a rule's rows gave, such as
-- "1 lower while the Hunger tracker is 1 or more, never below 1". Kept as
-- JSON beside the rows for the same reason: a rule is read and written whole.
-- NULL is a rule with no adjustments.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE field_rules
    ADD COLUMN IF NOT EXISTS adjust_json JSON NULL;

-- +goose Up
INSERT OR IGNORE INTO account_kinds (
  code,
  account_class,
  base_kind,
  is_builtin,
  is_user_assignable,
  display_key,
  sort_order
) VALUES
  ('other_money_account', 'asset', 'bank_account', 1, 1, 'account_kind_other_money_account', 150),
  ('fund_holding', 'asset', 'security_holding', 1, 1, 'account_kind_fund_holding', 225);

-- +goose Down
DELETE FROM account_kinds
WHERE code IN ('other_money_account', 'fund_holding')
  AND NOT EXISTS (
    SELECT 1
    FROM account_versions
    WHERE account_kind IN ('other_money_account', 'fund_holding')
  );

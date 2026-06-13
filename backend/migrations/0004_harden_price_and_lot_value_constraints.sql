-- +goose Up
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS price_observations_positive_insert
BEFORE INSERT ON price_observations
WHEN NEW.price_value <= 0
BEGIN
  SELECT RAISE(ABORT, 'price observation value must be positive');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS price_observations_positive_update
BEFORE UPDATE ON price_observations
WHEN NEW.price_value <= 0
BEGIN
  SELECT RAISE(ABORT, 'price observation value must be positive');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS investment_lots_values_valid_insert
BEFORE INSERT ON investment_lots
WHEN NEW.quantity_value <= 0
  OR NEW.remaining_quantity_value < 0
  OR NEW.cost_basis_value < 0
  OR NEW.remaining_cost_basis_value < 0
BEGIN
  SELECT RAISE(ABORT, 'investment lot values are invalid');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS investment_lots_values_valid_update
BEFORE UPDATE ON investment_lots
WHEN NEW.quantity_value <= 0
  OR NEW.remaining_quantity_value < 0
  OR NEW.cost_basis_value < 0
  OR NEW.remaining_cost_basis_value < 0
BEGIN
  SELECT RAISE(ABORT, 'investment lot values are invalid');
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS investment_lots_values_valid_update;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TRIGGER IF EXISTS investment_lots_values_valid_insert;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TRIGGER IF EXISTS price_observations_positive_update;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TRIGGER IF EXISTS price_observations_positive_insert;
-- +goose StatementEnd

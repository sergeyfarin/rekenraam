-- +goose Up

ALTER TABLE account_versions ADD COLUMN cost_basis_method TEXT CHECK (
    cost_basis_method IS NULL OR cost_basis_method IN ('fifo', 'lifo', 'average_cost', 'specific_lot')
);

DROP VIEW IF EXISTS current_account_versions;

CREATE VIEW IF NOT EXISTS current_account_versions AS
SELECT av.*
FROM account_versions av
WHERE av.id = (
    SELECT current_av.id
    FROM account_versions current_av
    WHERE current_av.account_id = av.account_id
      AND current_av.effective_from <= date('now')
    ORDER BY current_av.effective_from DESC, current_av.version_seq DESC
    LIMIT 1
);

-- +goose Down

DROP VIEW IF EXISTS current_account_versions;

ALTER TABLE account_versions DROP COLUMN cost_basis_method;

CREATE VIEW IF NOT EXISTS current_account_versions AS
SELECT av.*
FROM account_versions av
WHERE av.id = (
    SELECT current_av.id
    FROM account_versions current_av
    WHERE current_av.account_id = av.account_id
      AND current_av.effective_from <= date('now')
    ORDER BY current_av.effective_from DESC, current_av.version_seq DESC
    LIMIT 1
);

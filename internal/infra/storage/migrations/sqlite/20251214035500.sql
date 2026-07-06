DELETE FROM budgets_elements_limits
WHERE id NOT IN (
    SELECT MAX(id)
    FROM budgets_elements_limits
    GROUP BY element_id, period
);
DROP INDEX element_period_idx_budgets_elements_limits;
DROP INDEX period_idx_budgets_elements_limits;
DROP INDEX IDX_406C516F1F1F2A24;
CREATE TEMPORARY TABLE __temp__budgets_elements_limits AS SELECT id, element_id, period, amount, created_at, updated_at FROM budgets_elements_limits;
DROP TABLE budgets_elements_limits;
CREATE TABLE budgets_elements_limits (
    id CHAR(36) NOT NULL COLLATE BINARY,
    element_id CHAR(36) NOT NULL COLLATE BINARY,
    period DATETIME NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    amount NUMERIC(19, 8) NOT NULL,
    PRIMARY KEY(id),
    CONSTRAINT FK_406C516F1F1F2A24 FOREIGN KEY (element_id) REFERENCES budgets_elements (id) ON DELETE CASCADE NOT DEFERRABLE INITIALLY IMMEDIATE
);
INSERT INTO budgets_elements_limits (id, element_id, period, amount, created_at, updated_at) SELECT id, element_id, period, amount, created_at, updated_at FROM __temp__budgets_elements_limits;
DROP TABLE __temp__budgets_elements_limits;
CREATE INDEX period_idx_budgets_elements_limits ON budgets_elements_limits (period);
CREATE INDEX IDX_406C516F1F1F2A24 ON budgets_elements_limits (element_id);
CREATE UNIQUE INDEX element_period_uniq_budgets_elements_limits ON budgets_elements_limits (element_id, period);

-- Журнал расходов админа для страницы «Расходы»: P/L = выручка месяца − сумма
-- записей. Категория и иконка — фиксированный набор на фронте; в БД строки.
-- Таблица новая и аддитивная: откат образа без отката миграции безопасен.

BEGIN;

CREATE TABLE IF NOT EXISTS admin_expense (
    id           BIGSERIAL      PRIMARY KEY,
    occurred_on  DATE           NOT NULL,
    amount_rub   NUMERIC(12, 2) NOT NULL CHECK (amount_rub > 0),
    category     VARCHAR(32)    NOT NULL,
    title        VARCHAR(200)   NOT NULL,
    note         TEXT           NOT NULL DEFAULT '',
    icon         VARCHAR(32)    NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admin_expense_occurred
    ON admin_expense (occurred_on DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_admin_expense_category_month
    ON admin_expense (category, occurred_on);

COMMIT;

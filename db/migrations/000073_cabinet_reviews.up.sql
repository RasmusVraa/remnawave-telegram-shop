-- Отзывы клиентов: один отзыв на аккаунт кабинета, публичный список и витрина на лендинге.

BEGIN;

CREATE TABLE cabinet_review (
    id            BIGSERIAL PRIMARY KEY,
    account_id    BIGINT       NOT NULL REFERENCES cabinet_account (id) ON DELETE CASCADE,
    customer_id   BIGINT       REFERENCES customer (id) ON DELETE SET NULL,
    rating        SMALLINT     NOT NULL,
    body          TEXT         NOT NULL,
    visibility    VARCHAR(16)  NOT NULL,
    display_name  VARCHAR(120) NOT NULL DEFAULT '',
    avatar_url    TEXT         NOT NULL DEFAULT '',
    client_since  DATE,
    tariff_id     BIGINT,
    tariff_name   VARCHAR(255) NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT cabinet_review_rating_chk CHECK (rating BETWEEN 1 AND 5),
    CONSTRAINT cabinet_review_visibility_chk CHECK (visibility IN ('anonymous', 'name', 'name_avatar')),
    CONSTRAINT cabinet_review_body_len_chk CHECK (char_length(body) BETWEEN 10 AND 2000)
);

CREATE UNIQUE INDEX idx_cabinet_review_account ON cabinet_review (account_id);
CREATE INDEX idx_cabinet_review_created ON cabinet_review (created_at DESC, id DESC);

COMMIT;

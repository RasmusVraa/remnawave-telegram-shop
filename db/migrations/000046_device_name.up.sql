-- Пользовательские названия устройств (HWID). Remnawave своё поле для имени
-- не хранит, а deviceModel/platform перезаписывает клиент при подключении,
-- поэтому название живёт у нас. Ключ — пара клиент + HWID: один HWID у разных
-- клиентов не конфликтует, а чужое устройство не переименовать.
CREATE TABLE IF NOT EXISTS device_name
(
    customer_id BIGINT      NOT NULL REFERENCES customer (id) ON DELETE CASCADE,
    hwid        TEXT        NOT NULL,
    name        VARCHAR(64) NOT NULL,
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (customer_id, hwid)
);

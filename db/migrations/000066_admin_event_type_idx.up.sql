-- Лента «Мониторинга» с фильтром по редкой категории («Ноды», «Админы») и
-- счётчики за месяц иначе листают admin_event почти целиком. CONCURRENTLY — чтобы
-- не блокировать запись событий на большой базе; поэтому одна команда на файл.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_admin_event_type_id ON admin_event (type, id DESC);

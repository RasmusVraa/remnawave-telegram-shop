-- Способ ЮKassa, который покупатель выбрал кнопкой (YOOKASA_PAYMENT_METHODS):
-- sbp, bank_card, sberbank, tinkoff_bank, yoo_money. NULL — форма ЮKassa со
-- всеми способами (как было до настройки) или не ЮKassa вовсе.
--
-- По ней кабинет подставляет на странице оплаты способ, которым человек платил
-- в прошлый раз. Колонка аддитивная и nullable: старый образ её не читает
-- (выборки идут по явному списку колонок), откат без отката миграции безопасен.
ALTER TABLE purchase ADD COLUMN IF NOT EXISTS yookasa_payment_method TEXT;

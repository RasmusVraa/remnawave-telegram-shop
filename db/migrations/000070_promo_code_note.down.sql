-- Заметки к промокодам теряются при откате: это только подписи админа, на выдачу кода не влияют.
ALTER TABLE promo_code DROP COLUMN IF EXISTS note;

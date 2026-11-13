-- Platega: одна кнопка без выбранного способа. Прежние platega_* остаются
-- допустимыми — на них ссылаются старые строки cabinet_checkout.
ALTER TABLE cabinet_checkout
    DROP CONSTRAINT IF EXISTS cabinet_checkout_provider_chk;

ALTER TABLE cabinet_checkout
    ADD CONSTRAINT cabinet_checkout_provider_chk
        CHECK (provider IN (
            'yookassa',
            'cryptopay',
            'telegram',
            'platega',
            'platega_sbp',
            'platega_cards',
            'platega_acquiring',
            'platega_worldwide',
            'platega_crypto',
            'heleket'
        ));

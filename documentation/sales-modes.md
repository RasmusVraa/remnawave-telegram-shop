# Тарифы и цены

Продажи идут через каталог тарифов в PostgreSQL: у каждого тарифа свои цены, трафик, устройства, squads. Покупка по шагам: тариф → период → оплата. Прежний режим `classic` (цены из env) больше не поддерживается, переменная `SALES_MODE` не читается.

## Какие переменные на что влияют

| Переменные | Как используются |
| ---------- | --------- |
| `RUB_PER_STAR` | Опционально: расчёт Stars при оплате, если в `tariff_price` нет `amount_stars`; также подсказки при вводе цен в админке |
| `TRAFFIC_LIMIT_RESET_STRATEGY` | Лимит и стратегия **на уровне каждого тарифа** в БД; при сиде `standard` стратегия берётся из этой env. Лимит `TRAFFIC_LIMIT` в env больше не используется: тариф без лимита безлимитный |
| `PAID_HWID_LIMIT`, `HWID_FALLBACK_DEVICE_LIMIT` | Лимит **на уровне тарифа** в БД; сид `standard` берёт устройства из этих настроек |
| `HWID_ADD_PRICE`, `HWID_ADD_STARS_PRICE` | То же: доплата к счёту при выборе extra HWID |
| `TRAFFIC_SPLIT_ENABLED`, `TRAFFIC_PACKS_ENABLED` | Лимит, ноды, сквад и пакеты задаются **на тарифе** (вкладка «Трафик»), см. [traffic-split.md](./traffic-split.md) |
| Промокоды (pending % скидка) | Вставка перед строкой «Способы оплаты» на шаге выбора провайдера, если в тексте есть тот же HTML-маркер |

Цены в env (`PRICE_*`, `STARS_PRICE_*`) больше не используются: цены задаются в тарифах. Для автосида первого тарифа `standard` в коде зашиты стартовые цены (150 / 400 / 750 / 1400 ₽), их можно сразу поменять в админке. Если активных тарифов нет, бот показывает «Покупка сейчас недоступна».

## Тексты сценария покупки (tariffs)

Настраиваются в `translations/ru.json` и `translations/en.json` (ключи вида `buy_tariff_*`, `tariff_selected_*`, `tariff_payment_methods_text`, `payment_tariff_*`).

См. также [customization.md](./customization.md).

## Админка тарифов

При настроенном `ADMIN_TELEGRAM_ID`:

- в Telegram-боте — раздел тарифов;
- в web-кабинете (`CABINET_ENABLED=true`) — `/cabinet/admin/*` (тарифы, цены, описание).

Подробнее по API и UI: `.cursor/docs/api/cabinet-rest.md`, `.cursor/docs/reference/admin-cabinet.md`.

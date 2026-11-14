# Миграция с Bedolaga

Перенос клиентов, тарифов и рефералов из [Bedolaga](https://github.com/BEDOLAGA-DEV/remnawave-bedolaga-telegram-bot) в Meows Remnawave Telegram Shop.

## Совместимость (важно)

| | |
|--|--|
| **Цель миграции** | Meows-бот + панель **Remnawave 3.3.\*–3.4.\*** ([compatibility.md](./compatibility.md)) |
| **Источник Bedolaga** | И **3.x** (напр. 3.60), и **4.0+** — версия бота Bedolaga сама по себе не блокер |

Разница поколений Bedolaga:

| Bedolaga | Обычно панель Remnawave | Для Meows |
|----------|-------------------------|-----------|
| **3.x** (напр. 3.60) | до 3.0 (ветка 2.x) | Данные переносим; сама панель 2.x боту 5.x уже не подходит — Meows нужно подключать к RW **3.3.\*–3.4.\*** |
| **4.0+** | **3.0+** | Нормальный кейс: same-panel **3.3.\*–3.4.\*** |

Мигратор:

- читает схему источника гибко (опциональные колонки 3.x / 4.x);
- в dry-run пишет `compat_warning` и `bedolaga_generation_hint` в `summary.json`;
- wizard спрашивает подтверждение, что Remnawave для Meows — **3.3.\*–3.4.\***.

Главное — корректный перенос данных. Если админ сидел на Bedolaga 3 + RW 2.x, а Meows
подключается к RW 3.3 — это уже смена панели (не «тихий» same-panel).

> **С бота 5.0.0** мигратор работает только с Remnawave **3.x**: в 3.0.0 у пользователя
> панели удалено поле `uuid`. Поэтому записи Bedolaga, у которых нет ни `telegram_id`,
> ни `short_uuid`, сопоставить больше не с чем — они попадут в отчёт как несопоставленные.
> Сопоставление по `telegram_id` и `short_uuid` работает как прежде.

## Что переносится

| Источник Bedolaga | Куда | Примечание |
|-------------------|------|------------|
| Тарифы | `tariff` / `tariff_price` | Имя, цены (→ 1/3/6/12 мес), internal/external squads, лимиты |
| Юзеры с Telegram | `customer` | Тариф, срок, language, username |
| Юзеры без TG (email/OAuth) | `customer` web-only + `cabinet_account` | Synthetic telegram_id; пароль только если `$argon2id$` |
| Баланс (кошелёк) | Срок подписки | Дни = `баланс_₽ × 30 / цена_1м_тарифа_юзера`; итог `max(дни_RW, дни_баланса)` |
| Реф.граф | `referral` | По telegram_id |
| История оплат | `purchase` (`invoice_type = bedolaga`) | Транзакции `subscription_payment` с настоящими датами и суммами; доп. устройства не переносятся. Без этого бот считает платного клиента «триалом» |

**Срок подписки:** берётся из Remnawave; если клиента в панели не нашли — из подписки Bedolaga (`end_date`). Закончившаяся давно подписка сохраняет настоящую дату окончания.

**Remnawave (same-panel 3.3.\*–3.4.\*):** по умолчанию только чтение. На шаге balance — только удлинение `expireAt`, если он короче целевого. Сквады, трафик, теги не меняются. Новые VPN-юзеры не создаются.

**Не переносится:** wallet как сущность, autopay, daily-биллинг, пополнения баланса, withdrawals.

## Быстрый старт (рекомендуется)

На Linux-сервере, где уже стоит бот (см. [установщик](../readme.md)), с Docker:

```bash
cd /opt/remnawave-telegram-shop   # или ваш путь
./scripts/meows-bedolaga-migrate.sh
```

Меню проведёт по шагам:

1. **`1`** — источник: файл `backup_*.tar.gz` из админки Bedolaga (проще всего), `pg_dump` или готовый DSN; подтверждение Remnawave **3.3.\*–3.4.\*** и запись `migrate.yaml`
2. **`2`** — «Перенести всё сразу»: пробный прогон → копия БД бота (`migrate-out/shop-db-before-migrate-*.sql.gz`) → apply: тарифы → клиенты → баланс → рефералы → история оплат → итог
3. Отчёты — в `migrate-out/` (смотрите `problems.csv`)

Go на сервере **не нужен**: движок собирается и запускается в контейнере `golang` в docker-сети бота (первый запуск 1–3 минуты, нужен интернет). Готовый linux-бинарь можно подложить через `MEOWS_MIGRATE_BIN`. Шаги можно запускать и по одному (пункты 4–8 меню).

## Бэкап Bedolaga

### Вариант A — `pg_dump` (предпочтительно)

На сервере Bedolaga:

```bash
docker compose exec -T postgres pg_dump -U bedolaga -d bedolaga --no-owner \
  | gzip > bedolaga-$(date +%F).sql.gz
```

Скопируйте файл на сервер с Meows и укажите путь в пункте 1 меню.

### Вариант B — бекап из админки Bedolaga (проще всего)

В админ-меню Bedolaga: «Создать бекап» → файл `backup_*.tar.gz` (внутри `database.json`). Мигратор разворачивает его сам: в пункте 1 меню выберите «Бекап из админки Bedolaga» и укажите путь. Восстанавливать их инструментом не нужно.

## Прямой вызов движка

```bash
cp tools/migrate-bedolaga/migrate.yaml.example migrate.yaml
# отредактируйте DSN / Remnawave token (панель 3.3.* - 3.4.*)

go run ./tools/migrate-bedolaga -config migrate.yaml -dry-run -step all
go run ./tools/migrate-bedolaga -config migrate.yaml -apply -step tariffs
go run ./tools/migrate-bedolaga -config migrate.yaml -apply -step customers
go run ./tools/migrate-bedolaga -config migrate.yaml -apply -step balance
go run ./tools/migrate-bedolaga -config migrate.yaml -apply -step referrals
go run ./tools/migrate-bedolaga -config migrate.yaml -apply -step history
```

Отчёты: `migrate-out/summary.json`, `customers.csv`, `tariffs.csv`, `balances.csv`, `problems.csv`.

## Важные коды в problems.csv

| code | Смысл |
|------|--------|
| `compat_warning` | Подсказка по поколению Bedolaga (3.x / 4.x) и версии Remnawave |
| `rw_missing` | Юзер не найден в Remnawave — customer создан, VPN не трогали |
| `cabinet_password_reset_required` | Хеш пароля не argon2id — нужен сброс в кабинете |
| `multi_sub` | Несколько активных подписок Bedolaga — взята одна |
| `balance_would_extend` / extend | Планируется / выполнено удлинение expire в RW |
| `referral_partial` | Нет telegram mapping у реферера/реферала |
| `daily_tariff` | Daily-тариф импортирован как обычный |

## Требования к целевому боту

- Схема БД актуальна (миграции при старте бота)
- Remnawave **3.3.\*–3.4.\*** (та же панель, к которой ходит Meows)
- Перед apply лучше остановить бота Bedolaga, чтобы не плодить новые оплаты в старую БД

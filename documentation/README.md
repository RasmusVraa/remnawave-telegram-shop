# Документация Meows Telegram Shop

Публичные гайды для владельцев проекта. Корневой [`readme.md`](../readme.md) — краткий старт; подробности — здесь.

**Канал проекта:** [Meows VPN Shop | Канал](https://t.me/meows_vpn_bot)

## С чего начать

| Документ | Когда открывать |
|----------|-----------------|
| [`../readme.md`](../readme.md) | Что это, установка за 5 минут |
| [`cabinet/SETUP-GUIDE-RU.md`](./cabinet/SETUP-GUIDE-RU.md) | Запуск web-кабинета с нуля (домен, SSL, OAuth) |
| [`env.md`](./env.md) | Все переменные `.env` с пояснениями |
| [`sales-modes.md`](./sales-modes.md) | Тарифы и цены |

## Разделы

| Файл | Тема |
|------|------|
| [env.md](./env.md) | Справочник переменных окружения |
| [sales-modes.md](./sales-modes.md) | Тарифы, цены, тексты покупки |
| [payments.md](./payments.md) | Платёжные системы, вебхуки и поллинг |
| [notifications.md](./notifications.md) | Уведомления: истечение, lifecycle, лента и push в кабинете, torrent blocker (Remnawave webhook) |
| [traffic-split.md](./traffic-split.md) | Раздельный трафик (обычные серверы и обход) и докупка пакетов трафика |
| [technical-promo-codes.md](./technical-promo-codes.md) | Служебные промокоды с префиксом `__`: якоря для скидок колеса и win-back, правило «админ их не видит» |
| [squads.md](./squads.md) | Squads Remnawave (платные и триал, применение состава тарифа к подписчикам) |
| [partner-program.md](./partner-program.md) | Партнёрская программа: проценты, потоки, холд, выплаты |
| [customization.md](./customization.md) | Тексты бота/кабинета, кнопки, emoji |
| [backup.md](./backup.md) | Резервное копирование и восстановление |
| [moynalog-proxy.md](./moynalog-proxy.md) | «Мой налог» через прокси вне РФ |
| [reverse-proxy.md](./reverse-proxy.md) | Обратный прокси (Traefik и др.) |
| [cabinet/landing.md](./cabinet/landing.md) | Публичный лендинг проекта: адреса, что настраивать |
| [cabinet/server-status.md](./cabinet/server-status.md) | Статус серверов: публичная страница, кабинет, поддержка, настройка и ключи |
| [updating.md](./updating.md) | Обновление Docker-образа |
| [compatibility.md](./compatibility.md) | Совместимость версий бота и Remnawave |
| [cabinet/](./cabinet/) | Web-кабинет: setup, upgrade, support bridge |
| [bedolaga-migration.md](./bedolaga-migration.md) | Миграция клиентов/тарифов с Bedolaga |

## Для разработчиков

Внутренняя база знаний (архитектура, API, техдолг): [`.cursor/docs/README.md`](../.cursor/docs/README.md).

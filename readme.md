# Meows Telegram Shop

Продажа VPN-подписок через **Telegram-бота** и опциональный **web-кабинет**.

<img width="auto" height="auto" alt="image" src="https://github.com/user-attachments/assets/d18aa092-13fb-4926-ad81-a7f4f46b4445" />
<img width="auto" height="auto" alt="image" src="https://github.com/user-attachments/assets/9e07f17a-970c-40f3-876e-d33e47f9266a" />
<img width="auto" height="auto" alt="image" src="https://github.com/user-attachments/assets/849f198d-3d74-4596-b736-dac9efc1f238" />

Пользователи покупают и продлевают подписку в боте или в браузере; выдача идёт через панель [**Remnawave**](https://remna.st/).

**Канал проекта:** [Meows VPN Shop | Канал](https://t.me/meows_vpn_bot)

**Совместимость:** Remnawave `3.3.*`–`3.4.*` ↔ бот `5.x`. Панель `2.8.*` — бот `4.x` (заморожен). Полная матрица — [documentation/compatibility.md](documentation/compatibility.md).

---

## Что умеет этот форк

- Web-кабинет: регистрация, покупка, привязка Telegram, профиль — без обязательного входа только через бота
- Управление устройствами (HWID), покупка дополнительных мест для устройств
- Несколько тарифов, промокоды, рефералы, программа лояльности, партнёрская программа с выплатами деньгами
- Оплаты: YooKassa (в том числе отдельными кнопками СБП, карта и т.д.), Platega, CryptoPay, Heleket, Telegram Stars
- Админка в Telegram и в кабинете: пользователи, статистика, платежи, рассылка в Telegram и в кабинет, настройки без перезапуска
- Страница статуса серверов (публичная и в кабинете) с проверкой из мира и России, в том числе для серверов обхода
- Колесо фортуны, чат поддержки в кабинете, уведомления в кабинете и push в браузер, напоминания «подписка скоро закончится» и lifecycle (не подключился / вернуть клиента)
И еще больше изменений под капотом и внутри бота

Подробнее по темам — папка [documentation/](documentation/).

---

## Установка

Нужны **Linux**, **Docker** и **Docker Compose**. По умолчанию проект ставится в `/opt/remnawave-telegram-shop`.

### Быстрый старт (рекомендуется)

Интерактивный установщик: клонирует репозиторий, помогает заполнить `.env`, поднимает контейнеры, при необходимости — кабинет, SSL и проверки.

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/MrMe0ws/remnawave-telegram-shop/main/scripts/meows-shop-setup.sh)
```

Повторный запуск из каталога проекта:

```bash
cd /opt/remnawave-telegram-shop
./scripts/meows-shop-setup.sh
```

В меню:

1. Установить только Telegram-бота  
2. Установить бота + web-кабинет  
3. Только web-кабинет (бот уже стоит)  
4. Reverse-proxy / SSL для кабинета  
5. Smoke-проверки  
6. Управление ботом  

### Ручная установка

```bash
git clone https://github.com/MrMe0ws/remnawave-telegram-shop.git
cd remnawave-telegram-shop
cp .env.sample .env
# отредактируйте .env — см. блок ниже и documentation/env.md
docker compose up -d
```

После смены `.env` пересоздайте контейнер: `docker compose up -d --force-recreate`.

### Миграция с Bedolaga

Интерактивный перенос клиентов и тарифов:

```bash
./scripts/meows-bedolaga-migrate.sh
```

Подробности: [`documentation/bedolaga-migration.md`](documentation/bedolaga-migration.md).

### Web-кабинет с нуля

Пошагово (домен, DNS, SSL, вход через Google/Яндекс/Telegram):  
[documentation/cabinet/SETUP-GUIDE-RU.md](documentation/cabinet/SETUP-GUIDE-RU.md)

---

## Что обязательно указать в `.env`

Установщик спросит главное сам. Если правите файл вручную — минимум:

| Переменная | Зачем |
|------------|--------|
| `TELEGRAM_TOKEN` | Токен бота от [@BotFather](https://t.me/BotFather) |
| `REMNAWAVE_URL`, `REMNAWAVE_TOKEN` | Адрес и токен панели Remnawave |
| `DATABASE_URL` или `POSTGRES_*` | База PostgreSQL |
| `PRICE_1` … `PRICE_12` | Цены за периоды (нужны даже в режиме тарифов) |
| `ADMIN_TELEGRAM_ID` | Ваш числовой Telegram ID — доступ в админку |
| `HEALTH_CHECK_PORT` | Порт сервиса (проверка «жив ли бот», кабинет, уведомления от платёжек) |

Оплаты включаются флагами вроде `YOOKASA_ENABLED`, `PLATEGA_ENABLED`, `TELEGRAM_STARS_ENABLED` — заполните ключи только для тех способов, которыми пользуетесь.

**Полный список переменных с пояснениями:** [documentation/env.md](documentation/env.md)  
Шаблон с комментариями: [`.env.sample`](.env.sample)

Режимы продаж (`classic` / `tariffs`): [documentation/sales-modes.md](documentation/sales-modes.md)

---

## Документация

| Тема | Файл |
|------|------|
| Оглавление | [documentation/README.md](documentation/README.md) |
| Все переменные `.env` | [documentation/env.md](documentation/env.md) |
| Платежи и вебхуки | [documentation/payments.md](documentation/payments.md) |
| Уведомления | [documentation/notifications.md](documentation/notifications.md) |
| Тексты и кнопки | [documentation/customization.md](documentation/customization.md) |
| Обновление | [documentation/updating.md](documentation/updating.md) |
| Резервное копирование | [documentation/backup.md](documentation/backup.md) |
| Кабинет | [documentation/cabinet/](documentation/cabinet/) |

---

## Обновление

```bash
git pull   # тексты бота (translations/) берутся с диска, а не из образа
docker compose pull
docker compose down && docker compose up -d
```

Подробнее: [documentation/updating.md](documentation/updating.md)

---

## Бэкапы

Своего механизма бэкапов у бота нет. Рекомендуется
[distillium/remnawave-backup-restore](https://github.com/distillium/remnawave-backup-restore) —
он бэкапит и панель Remnawave, и этого бота: база, папка проекта, расписание, отправка
в Telegram, восстановление.

В его меню выбора бота подойдёт пункт **«Бот от Иисуса»**: имена контейнера и тома
в этом форке сохранены, поэтому параметры совпадают. Версия бота значения не имеет —
скрипт снимает дамп всей базы и не разбирает её схему.

⚠️ В архив попадает `.env` со всеми секретами. Если бэкапы уходят в Telegram, учтите это
или исключите файл — подробности в [documentation/backup.md](documentation/backup.md).

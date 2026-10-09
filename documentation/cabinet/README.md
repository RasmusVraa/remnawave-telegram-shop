# Документация web-кабинета

Общее оглавление публичных гайдов: [../README.md](../README.md).  
Краткий старт проекта: [`../../readme.md`](../../readme.md).

## Для владельца проекта (запуск с нуля)

- Интерактивный установщик (Linux + Docker), one-liner в корневом `readme.md` (секция **Установка**):
  - `scripts/meows-shop-setup.sh` — внутреннее описание: `.cursor/docs/operations/scripts.md`
- Полный пошаговый гайд (домен, DNS, SSL, nginx/caddy, OAuth, Telegram auth 1.0/2.0, migrations, env, translations):
  - [SETUP-GUIDE-RU.md](./SETUP-GUIDE-RU.md)
- Все переменные `.env`: [../env.md](../env.md)

## Для обновления существующей установки

- Переход на версию с кабинетом:
  - [cabinet-upgrade-guide.md](./cabinet-upgrade-guide.md)
- Общее обновление Docker: [../updating.md](../updating.md)

## Публичный лендинг

- Витрина проекта на `/landing` (и `/cabinet/landing`): адреса, нужна ли правка
  reverse-proxy, что подтягивается из настроек, где править тексты:
  - [landing.md](./landing.md)

## Страница подключения устройства

- `/cabinet/connections` и `/connect` по приглашению: устройство экрана, откуда
  берутся тексты и кнопки, окно выбора устройства, блок помощи, план переверстки:
  - [connections-page.md](./connections-page.md)

## Шапка и меню на iPhone

- Почему на iOS 26 и в новом Telegram шапка и нижнее меню сделаны «сплошными» и как это устроено:
  - [iphone-edges.md](./iphone-edges.md)

## Чат поддержки (bridge к telegram-support-bot)

- Встроенный чат в кабинете при `SUPPORT_BOT_API=true` (миграция `000036_cabinet_support`).
- Настройка env, Docker-сеть shop ↔ support-bot, smoke-проверка:
  - [SETUP-GUIDE-RU.md](./SETUP-GUIDE-RU.md) — раздел **«18) Чат поддержки (support bridge)»**
- Документация support-bot: `README.md` в репозитории [telegram-support-bot](https://github.com/MrMe0ws/telegram-support-bot).

## Runtime и контент

- Контент из `/translations/cabinet/*`:
  - `GET /cabinet/api/content/faq` → `translations/cabinet/FAQ.json`
  - `GET /cabinet/api/content/app-config` → `translations/cabinet/app-config.json`
- Для изменения runtime-контента без ребилда используйте volume:
  - `./translations:/translations`
- Тексты SPA и кнопки бота: [../customization.md](../customization.md)

## Быстрые проверки после изменений

- Фронтенд:
  - `cd web/cabinet && npm run typecheck && npm run build`
- Backend cabinet:
  - `go test ./internal/cabinet/http/...`

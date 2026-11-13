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
  берутся тексты и кнопки, окно выбора устройства, блок помощи, какие приложения
  видны по приглашению при шифровании deep link:
  - [connections-page.md](./connections-page.md)

## Главная страница кабинета

- С 5.25.0 «Главная» и «Подписка» — одна страница: кольца трафика и устройств,
  продление, подключение и ряд быстрых действий.
- Кнопки быстрых действий на местах 3 и 4 настраиваются в админке без рестарта
  (`CABINET_HOME_QUICK_ACTIONS`); места 1 и 2 зависят от подписки:
  - [../env.md](../env.md)
- Способы оплаты ЮKassa показываются отдельными кнопками, способ можно выбрать
  заранее (`YOOKASA_PAYMENT_METHODS`, `CABINET_CHECKOUT_DEFAULT_METHOD`):
  - [../payments.md](../payments.md)

## Отклик кнопок при медленном интернете

- Со следующей версии после 5.26.0 после нажатия на кнопку, ссылку или пункт меню на месте иконки
  крутится спиннер, пока страница или ответ сервера не загрузились. Нажать
  второй раз нельзя. Так при медленном соединении видно, что кабинет не завис.
- Это не настройка, а правило разработки: каждая новая кнопка, после которой
  что-то грузится, обязана показывать загрузку (описано в `.cursor/AGENTS.md` и
  `.cursor/docs/frontend/cabinet-loading-states.md`, в публичный репозиторий
  не входят).

## Уведомления в кабинете

- Колокольчик в шапке и лента уведомлений есть у всех вошедших; какие из них
  дублировать push-уведомлением в браузере, задают переключатели
  `CABINET_PUSH_*` (админка: «Настройки бота → Уведомления»):
  - [../env.md](../env.md), раздел «Push в браузер из кабинета»
- Push работает только по HTTPS и не работает внутри Telegram Mini App (там
  уведомления и так приходят от бота). На iPhone — только у кабинета,
  добавленного на экран «Домой».

## Шапка и меню на iPhone

- Почему на iOS 26 и в новом Telegram шапка и нижнее меню сделаны «сплошными» и как это устроено:
  - [iphone-edges.md](./iphone-edges.md)

## Чат поддержки (bridge к telegram-support-bot)

- Встроенный чат в кабинете при `SUPPORT_BOT_API=true` (миграция `000036_cabinet_support`); изображения в чате — миграция `000064_cabinet_support_attachments`, support-bot с поддержкой вложений и `client_max_body_size 26m` в nginx на `/cabinet/api/support/messages`.
- Настройка env, Docker-сеть shop ↔ support-bot, smoke-проверка:
  - [SETUP-GUIDE-RU.md](./SETUP-GUIDE-RU.md) — раздел **«18) Чат поддержки (support bridge)»**
- Документация support-bot: `README.md` в репозитории [telegram-support-bot](https://github.com/MrMe0ws/telegram-support-bot).

## Runtime и контент

- Контент из `/translations/cabinet/*`:
  - `GET /cabinet/api/content/faq` → `translations/cabinet/FAQ.json`
  - `GET /cabinet/api/content/app-config` → `translations/cabinet/app-config.json`
- Для изменения runtime-контента без ребилда используйте volume:
  - `./translations:/translations`
- Тексты интерфейса кабинета: `GET /cabinet/api/content/i18n/{lang}` →
  `/translations/cabinet/i18n/{lang}.json` в контейнере (в `docker-compose.yaml`
  туда смонтирован `./web/cabinet/src/i18n`). Файл **накладывается поверх**
  встроенных в сборку текстов: если там лежит старая копия, она молча
  перекрывает новые тексты из обновления. Как править тексты —
  [../customization.md](../customization.md).
- Тексты SPA и кнопки бота: [../customization.md](../customization.md)

## Быстрые проверки после изменений

- Фронтенд:
  - `cd web/cabinet && npm run typecheck && npm run build`
- Backend cabinet:
  - `go test ./internal/cabinet/http/...`

# Кастомизация: тексты, кнопки, интерфейс

## Как меняются кнопки меню бота

Основные кнопки («Купить», подключение к VPN) всегда на месте.  
Дополнительные появляются только если задан URL в `.env`:

| Переменная | Кнопка |
|------------|--------|
| `SERVER_STATUS_URL` | Статус сервера (в кабинете есть своя страница, см. [server-status.md](./cabinet/server-status.md); эта ссылка — запасная) |
| `SUPPORT_URL` | Поддержка (в кабинете при `SUPPORT_BOT_API=true` — встроенный чат) |
| `FEEDBACK_URL` | Отзывы |
| `CHANNEL_URL` | Канал |
| `TOS_URL` | Условия (меню «Помощь»; в кабинете — запасной URL соглашения в дисклеймере) |
| `PUBLIC_OFFER_URL` | Публичная оферта («Помощь» + gate согласия в боте) |
| `PRIVACY_POLICY_URL` | Политика конфиденциальности («Помощь» + gate в боте + дисклеймер в кабинете) |
| `TERMS_OF_SERVICE_URL` | Пользовательское соглашение (опциональная кнопка в gate; приоритетная ссылка соглашения в кабинете) |
| `VIDEO_GUIDE_URL` | Видеоинструкция в «Помощь» |
| `SERVER_SELECTION_URL` | «Какой сервер выбрать» в «Помощь» |

В режиме меню `minimalism` (`CABINET_TELEGRAM_UI_MODE`) кнопки канала/отзывов ещё зависят от `CABINET_TELEGRAM_SHOW_*_BUTTON`. Часть ссылок можно менять из web-админки без рестарта.

## Юридическое согласие

Новых переменных нет — используются уже существующие URL из таблицы выше. Подробности env: [env.md](./env.md) (раздел «Ссылки в меню бота»).

### Telegram-бот

Если заданы **оба** URL — `PRIVACY_POLICY_URL` и `PUBLIC_OFFER_URL` — до главного меню показывается экран согласия:

- URL-кнопки: политика, оферта; при непустом `TERMS_OF_SERVICE_URL` — ещё пользовательское соглашение;
- «Принимаю» / «Отказываюсь»;
- принятие пишется в `customer.legal_accepted_at`;
- отказ остаётся на экране gate (с текстом об отказе);
- middleware блокирует остальные действия пользователя, пока согласие не принято.

Если хотя бы один из двух обязательных URL пуст — gate **выключен**, бот работает как раньше.

Миграция `000040_customer_legal_accepted_at` добавляет колонку и проставляет `legal_accepted_at` существующим клиентам (`COALESCE(created_at, now())`), чтобы не заставлять старых пользователей проходить gate заново.

Тексты gate — ключи `legal_gate_*`, `legal_accept_button`, `legal_decline_button` и кнопки документов в `translations/ru.json` / `en.json`.

### Web-кабинет

Только UI-дисклеймер (без записи в БД и без блокирующего gate):

- компонент `LegalContinueDisclaimer` на login / register и на экранах оплаты (checkout, доп. устройства);
- текст вида «Продолжая, Вы соглашаетесь…» со ссылками на политику и пользовательское соглашение;
- ссылки из `site_links` в `GET /cabinet/api/auth/bootstrap` (те же env, что у бота);
- соглашение: `TERMS_OF_SERVICE_URL` → иначе `PUBLIC_OFFER_URL` → иначе `TOS_URL`;
- если нет ни политики, ни соглашения — дисклеймер не показывается.

Строки: `legal.*` в `web/cabinet/src/i18n/ru.json` / `en.json`.

## Тексты Telegram-бота

Папка `translations/`:

- `ru.json`, `en.json` — пользовательские строки;
- `admin_ru.json`, `admin_en.json` — админские.

После правок перезапустите контейнер бота.

## Тексты web-кабинета

1. Редактируйте `web/cabinet/src/i18n/ru.json` и/или `en.json`.
2. В Docker должен быть volume вроде `./web/cabinet/src/i18n:/translations/cabinet/i18n` (см. `docker-compose.yaml`).
3. Перезапустите бота и обновите страницу в браузере.

Пересборка образа нужна при изменении **кода** React/Go, не при правке этих JSON.  
Чтобы строки попали в fallback внутри бандла: `cd web/cabinet && npm run build`, затем пересборка образа.

Контент FAQ / app-config: `translations/cabinet/` — см. [cabinet/README.md](./cabinet/README.md).

## Цвет и custom emoji у кнопок

В `translations/*.json` кнопка может быть строкой или объектом:

```json
{
  "buy_button": {
    "text": "💰 Купить",
    "style": "blue",
    "emoji_id": "1234567890123456789"
  }
}
```

### Значения `style`

- `blue` (алиас `primary`)
- `green` (алиас `success`, также `sucess`)
- `red` (алиас `danger`)

Если `style` не указан — цвет не применяется. Ошибочный стиль игнорируется.

### Про `emoji_id`

Это ID **кастомного** emoji Telegram (не обычный Unicode). Берётся из объектов Telegram API при выборе custom emoji.

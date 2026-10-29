# Полный гайд по запуску бота с Web-кабинетом

Этот документ предназначен для владельца сервера: по шагам от домена и DNS до рабочего Telegram-бота с web-кабинетом, оплатами, OAuth и прокси.

## Что вы получите после настройки

- Telegram-бот работает как раньше.
- Web-кабинет доступен по вашему домену (например, `https://cabinet.example.com/cabinet/`).
- Авторизация работает через email/пароль, Google, Yandex, VK, Telegram.
- Мини-приложение Telegram (Mini App) и web-кабинет используют один backend.
- Reverse proxy, SSL, CORS и cookie настроены корректно.

## 1) Минимальные требования

- Linux-сервер с публичным IP (VPS/VM).
- Домен, которым вы управляете.
- Docker + Docker Compose.
- PostgreSQL (в compose или отдельно).
- Доступ к BotFather и кабинетам OAuth-провайдеров.

## 2) Архитектура простыми словами

Пока **`CABINET_ENABLED=false`**, бот ведёт себя как обычно: маршруты `/cabinet/*` и `/cabinet/api/*` не поднимаются.

- Backend (Go) обслуживает:
  - Telegram bot API webhook/long-polling,
  - API кабинета `/cabinet/api/*`,
  - SPA кабинета `/cabinet/*`.
- Фронтенд кабинета (`web/cabinet`) встраивается в бинарник через `go:embed` из `internal/cabinet/web/dist`.
- Папка `translations/` подключается как runtime-контент (без ребилда для JSON-контента кабинета).

## 3) Домен и DNS

Рекомендуемая схема:

- `cabinet.example.com` -> кабинет.

Минимально:

1. Создайте `A` запись для `cabinet.example.com` на IP сервера.
2. Подождите обновления DNS (обычно 1-30 минут).
3. Проверьте:

```bash
nslookup cabinet.example.com
```

## 4) SSL: как получить сертификат

### Вариант A: Caddy (самый простой)

Caddy автоматически получает и продлевает Let's Encrypt сертификат.

Пример `Caddyfile`:

```caddy
cabinet.example.com {
  encode zstd gzip
  reverse_proxy 127.0.0.1:3000
}
```

Где `3000` = `HEALTH_CHECK_PORT`.

### Вариант B: Nginx + certbot

1. Настроить серверный блок на `:80` и проксирование.
2. Выполнить:

```bash
sudo certbot --nginx -d cabinet.example.com
```

3. Certbot сам добавит SSL-конфиг и авто-редирект HTTP -> HTTPS.

## 5) Reverse proxy: Nginx и Caddy

## Nginx (пример)

```nginx
server {
    listen 80;
    server_name cabinet.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name cabinet.example.com;

    ssl_certificate     /etc/letsencrypt/live/cabinet.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/cabinet.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Caddy (пример)

```caddy
cabinet.example.com {
  encode zstd gzip
  reverse_proxy 127.0.0.1:3000
}
```

Важно:

- Проксируйте весь трафик на один порт backend (`HEALTH_CHECK_PORT`).
- Не публикуйте backend напрямую в интернет без reverse proxy.

### Nginx (усиленный вариант для продакшена)

Ниже пример «жестче дефолта»: ограничение частоты/соединений на edge, таймауты, ограничение тела запроса и базовые security headers.

```nginx
# --- http context (обычно в /etc/nginx/nginx.conf) ---
limit_req_zone  $binary_remote_addr  zone=cabinet_api_ip:10m  rate=8r/s;
limit_conn_zone $binary_remote_addr  zone=cabinet_conn_ip:10m;

server {
    listen 80;
    server_name cabinet.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name cabinet.example.com;

    ssl_certificate      /etc/nginx/ssl/cabinet.example.com/fullchain.pem;
    ssl_certificate_key  /etc/nginx/ssl/cabinet.example.com/privkey.pem;
    ssl_trusted_certificate /etc/nginx/ssl/cabinet.example.com/fullchain.pem;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    # Общие hardening-настройки на сервер.
    client_max_body_size 2m;
    client_body_timeout 15s;
    send_timeout 30s;
    keepalive_timeout 30s;

    # Базовый лимит соединений/IP.
    limit_conn cabinet_conn_ip 30;

    location = / {
        return 302 /cabinet/;
    }

    # Более строгий лимит на auth-маршруты (защита от перебора).
    location ~ ^/cabinet/api/auth/(login|register|password/forgot)$ {
        limit_req zone=cabinet_api_ip burst=10 nodelay;
        proxy_pass http://127.0.0.1:3002;
        proxy_http_version 1.1;
        proxy_connect_timeout 10s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host  $host;
        proxy_set_header X-Forwarded-Port  $server_port;
    }

    # Загрузка вложения для рассылки в web-админке (картинка или видео).
    location = /cabinet/api/admin/broadcast/upload-media {
        client_max_body_size 50m;
        # Серверный client_body_timeout (15s в примере ниже) рассчитан на
        # обычные запросы кабинета. Заливка видео с мобильного интернета в него
        # не укладывается, и nginx обрывает загрузку на середине.
        client_body_timeout 300s;
        proxy_pass http://127.0.0.1:3002;
        proxy_http_version 1.1;
        proxy_connect_timeout 10s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host  $host;
        proxy_set_header X-Forwarded-Port  $server_port;
    }

    # Остальной трафик кабинета.
    location / {
        limit_req zone=cabinet_api_ip burst=40 nodelay;
        proxy_pass http://127.0.0.1:3002;
        proxy_http_version 1.1;
        proxy_connect_timeout 10s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host  $host;
        proxy_set_header X-Forwarded-Port  $server_port;
    }
}
```

Практические заметки:

- `limit_req_zone`/`limit_conn_zone` объявляются в `http {}` (глобально), а не внутри `server {}`.
- Значения `rate`/`burst` подбирайте по вашему трафику: начните с примера выше и смотрите 429/latency в метриках.
- Этот слой **дополняет** rate-limit в приложении, а не заменяет его.
- `HEALTH_CHECK_PORT` бота держите на loopback (`127.0.0.1:3002:3002`) и не открывайте в интернет.

### Вариант из прода: Remnawave + Nginx в одном compose, bot в другом контейнере

Если у вас:

- `remnawave-nginx` запущен отдельно (например, `network_mode: host`),
- `remnawave-telegram-shop-bot` в другом compose,
- порт бота проброшен на хост как `127.0.0.1:3002:3002`,

то рабочая схема такая:

- nginx проксирует `cabinet.domain.com` -> `http://127.0.0.1:3002`,
- backend бота обслуживает и `/cabinet/*`, и `/cabinet/api/*`,
- корень `/` редиректится в `/cabinet/`.

Пример server-блока:

```nginx
server {
    server_name cabinet.example.com;

    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;

    ssl_certificate      /etc/nginx/ssl/cabinet.example.com/fullchain.pem;
    ssl_certificate_key  /etc/nginx/ssl/cabinet.example.com/privkey.pem;
    ssl_trusted_certificate /etc/nginx/ssl/cabinet.example.com/fullchain.pem;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    location = / {
        return 302 /cabinet/;
    }

    location / {
        proxy_pass http://127.0.0.1:3002;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host  $host;
        proxy_set_header X-Forwarded-Port  $server_port;
    }
}

server {
    listen 80;
    server_name cabinet.example.com;
    return 301 https://$host$request_uri;
}
```

Важно:

- Фронтенд кабинета отдаёт сам бот (через `go:embed`).
- При `network_mode: host` nginx увидит проброс `127.0.0.1:3002`.

#### Пояснение директив в `location /` (если добавляете их в свой конфиг)

- `proxy_connect_timeout 10s` — максимум времени на **установку TCP-соединения** nginx → backend (бот). Если бот не отвечает, nginx оборвёт попытку быстрее, чем «висеть» долго.
- `proxy_send_timeout 60s` — таймаут **отправки запроса** от nginx к upstream (в т.ч. тело POST). Длинные загрузки редки для кабинета; при необходимости увеличьте.
- `proxy_read_timeout 60s` — таймаут **ожидания ответа** от upstream. Если бот долго обрабатывает запрос (тяжёлый checkout, внешние API), увеличьте значение, иначе клиент получит 502.
- `client_max_body_size 2m` — максимальный размер **тела запроса** от браузера к nginx (загрузки, большие JSON). Для обычного кабинета 1–4 МБ обычно достаточно; под аватарки/файлы — поднимайте осознанно. Исключение — загрузка вложения для рассылки (`/cabinet/api/admin/broadcast/upload-media`): на этом location лимит поднят до `50m`. С общими 2m PNG-скриншоты отбивались `413 Request Entity Too Large` ещё на nginx, не доходя до бота (JPEG проходил только потому, что весит меньше), а видео не проходило бы и подавно. 50 MiB — предел Bot API на файл, отправляемый ботом, столько же принимает и сам бот; ставить больше бессмысленно, Telegram всё равно откажет. Важно: у этого `location` обязателен свой `proxy_pass` со всеми заголовками — точное совпадение (`location = …`) перехватывает запрос целиком и в `location /` его не передаёт, так что блок из одной строки с лимитом превратит загрузку в 404. И применяйте правку там, где nginx на самом деле работает: если он в контейнере (`remnawave-nginx`), то хостовый `nginx -t` проверит совсем другой файл и напишет «ok» про него, а перезагрузка нужна внутри — `docker exec remnawave-nginx nginx -t` и `nginx -s reload`. Убедиться, что контейнер видит правку: `docker exec remnawave-nginx nginx -T | grep -A4 upload-media`.

В минимальном примере выше эти директивы опущены — nginx использует дефолты. Добавляйте их, если нужен явный контроль таймаутов.

#### Сертификаты в `docker-compose` для nginx

Сертификаты Let's Encrypt лежат на **хосте** (например `/etc/letsencrypt/live/<имя>/...`). Контейнер nginx должен **видеть те же файлы**, что прописаны в `ssl_certificate` / `ssl_certificate_key` внутри контейнера — для этого монтируют read-only:

```yaml
  remnawave-nginx:
    image: nginx:1.28
    network_mode: host
    volumes:
      - ./nginx.conf:/etc/nginx/conf.d/default.conf:ro
      # Пример: сертификат кабинета (путь на хосте → путь в контейнере, как в nginx.conf)
      - /etc/letsencrypt/live/cabinet.example.com/fullchain.pem:/etc/nginx/ssl/cabinet.example.com/fullchain.pem:ro
      - /etc/letsencrypt/live/cabinet.example.com/privkey.pem:/etc/nginx/ssl/cabinet.example.com/privkey.pem:ro
```

Имя каталога в `live/` у certbot может быть с суффиксом (`cabinet.example.com-0001`) — важно монтировать **реальные** пути с хоста, они должны совпадать с путями в `ssl_certificate` внутри `nginx.conf`.

После `certbot renew` файлы обычно обновляются на месте; достаточно `nginx -s reload` в контейнере.

#### Имя кабинета и логотип

Настраивается **переменными окружения бота** (не nginx), см. комментарии в `.env.sample`:

- `CABINET_BRAND_NAME` — название в шапке и на страницах входа.
- `CABINET_BRAND_LOGO_URL` — полный URL картинки или путь относительно `CABINET_PUBLIC_URL`.
- `CABINET_BRAND_LOGO_FILE` — файл на диске процесса бота; отдаётся как `GET …/cabinet/api/public/brand-logo` (и как favicon). В Docker удобно положить файл рядом с бинарём и/или задать `CABINET_BRAND_LOGO_FILE_BASE`.

Если логотип лежит рядом с compose как `./vpn_cat.png`, в сервисе **бота** смонтируйте его в контейнер (путь должен совпадать с `CABINET_BRAND_LOGO_FILE`).

#### Рекомендуемые `volumes` для контейнера бота

Чтобы править тексты бота и runtime-контент кабинета **без пересборки образа**:

```yaml
services:
  bot:
    volumes:
      - ./translations:/translations
      - ./vpn_cat.png:/vpn_cat.png:ro
```

- `./translations:/translations` — `translations/cabinet/FAQ.json`, `app-config.json` и языки бота; эндпоинты `GET /cabinet/api/content/*` читают файлы с этого mount.
- `./vpn_cat.png:/vpn_cat.png:ro` — пример файла логотипа; в `.env` задайте `CABINET_BRAND_LOGO_FILE=/vpn_cat.png` (или путь, куда смонтировали).

### Как автоматически получать SSL для такого nginx-контейнера

Есть 2 нормальных пути:

1. **Certbot на хосте (рекомендуется для вашей схемы):**
   - certbot получает/продлевает сертификаты в `/etc/letsencrypt/...`;
   - nginx-контейнер монтирует эти файлы read-only;
   - после продления делаете reload nginx (`docker exec remnawave-nginx nginx -s reload`).

2. **acme.sh/lego на хосте** с тем же принципом (серты на хосте, в контейнер только mount).

Пример для certbot:

```bash
sudo certbot certonly --nginx -d cabinet.example.com
sudo systemctl enable certbot.timer
sudo systemctl start certbot.timer
```

Проверка автообновления:

```bash
sudo certbot renew --dry-run
```

Если certbot не умеет сам reload контейнера, добавьте cron/systemd hook:

```bash
docker exec remnawave-nginx nginx -s reload
```

## 6) Подготовка `.env`

Скопируйте образец:

```bash
cp .env.sample .env
```

Минимум для кабинета:

- `CABINET_ENABLED=true`
- `CABINET_PUBLIC_URL=https://cabinet.example.com`
- `CABINET_ALLOWED_ORIGINS=https://cabinet.example.com`
- `CABINET_JWT_SECRET=<секрет 32+ байт>`

Генерация секрета:

```bash
openssl rand -hex 32
```

Рекомендуемые:

- `CABINET_COOKIE_DOMAIN=` (пусто обычно ок, берется host из `CABINET_PUBLIC_URL`)
- `CABINET_ACCESS_TTL_MINUTES=15`
- `CABINET_REFRESH_TTL_DAYS=30`
- SMTP (`CABINET_SMTP_*`, `CABINET_MAIL_FROM`) для писем.

### Как работают `CABINET_ACCESS_TTL_MINUTES` и `CABINET_REFRESH_TTL_DAYS`

- `CABINET_ACCESS_TTL_MINUTES` — жизнь access token (короткая, например 15 минут).
- `CABINET_REFRESH_TTL_DAYS` — жизнь refresh-сессии в cookie (длинная, например 30 дней).

Практический пример "сессия не отваливается месяц, если юзер заходит раз в неделю":

```env
CABINET_ACCESS_TTL_MINUTES=15
CABINET_REFRESH_TTL_DAYS=30
```

Почему это работает:

- фронт периодически делает `POST /cabinet/api/auth/refresh`;
- refresh токен ротируется, и сессия продлевается от текущего момента;
- если пользователь активен и заходит хотя бы раз в несколько дней, он обычно остаётся залогинен.

Когда точно разлогинит:

- если пользователь не заходил дольше `CABINET_REFRESH_TTL_DAYS`,
- или refresh-cookie потерялась/очищена,
- или сессия отозвана (logout/security event).

## 7) Миграции: зачем их много (начиная с `000017`)

Кабинет развивался поэтапно. После `000017` добавлялись:

- базовая схема кабинета,
- checkout и провайдеры оплат,
- merge/link и аудит,
- OAuth-провайдеры и soft unlink,
- защитные миграции и вспомогательные таблицы.

Почему нельзя "свернуть в одну":

- пользователи уже на проде, нужна безопасная эволюция схемы;
- миграции фиксируют точную историю изменений и позволяют обновляться без потери данных;
- rollback проще и прозрачнее.

Практика:

- на новой установке применяются все миграции по порядку;
- на существующей установке применяются только новые.

## 8) Нужно ли каждый раз билдить фронтенд

Коротко:

- Если меняли `web/cabinet/src/*` -> да, нужно `npm run build`, чтобы обновить `internal/cabinet/web/dist` перед `go build`/Docker build.
- Если меняли только `translations/cabinet/*.json` и у вас volume `./translations:/translations` -> ребилд не нужен.

Почему:

- SPA вшивается в бинарник через `go:embed` из `dist`.
- Без свежего `dist` в бинарник попадет старый UI.

## 9) Зачем меняли Dockerfile

Новый Dockerfile делает multi-stage сборку:

1. Собирает фронтенд кабинета.
2. Копирует `dist` в backend-стадию.
3. Собирает Go-бинарник уже со свежим `go:embed`.

Итог:

- меньше "человеческих ошибок" в релизе,
- всегда согласованные backend + frontend в одном образе,
- проще CI/CD.

## 10) Папка `translations`: как работает

- `translations/ru.json`, `translations/en.json` — тексты Telegram-бота и части UI.
- `translations/cabinet/FAQ.json` — контент FAQ кабинета.
- `translations/cabinet/app-config.json` — конфиг страницы подключений (`/cabinet/connections`), платформы/кнопки/шаги.

Runtime-эндпоинты:

- `GET /cabinet/api/content/faq`
- `GET /cabinet/api/content/app-config`

Что значит "runtime-эндпоинты":

- это API, которые читают JSON-файлы **во время работы сервиса** с диска (`/translations/cabinet/*`);
- в отличие от фронтенда, их не нужно вшивать через `go:embed` и не нужно пересобирать образ после каждого изменения контента;
- поменяли `translations/cabinet/FAQ.json` -> сразу обновился ответ `GET /cabinet/api/content/faq`.

Чтобы обновлять без ребилда образа:

- используйте volume `./translations:/translations`.

## 11) OAuth и Telegram auth 1.0 vs 2.0

### Google / Yandex / VK

Нужны пары:

- `CLIENT_ID`
- `CLIENT_SECRET`
- `REDIRECT_URL` (должен совпадать 1:1 в кабинете провайдера)

Переменные:

- Google: `CABINET_GOOGLE_CLIENT_ID`, `CABINET_GOOGLE_CLIENT_SECRET`, `CABINET_GOOGLE_REDIRECT_URL`
- Yandex: `CABINET_YANDEX_CLIENT_ID`, `CABINET_YANDEX_CLIENT_SECRET`, `CABINET_YANDEX_REDIRECT_URL`
- VK: `CABINET_VK_CLIENT_ID`, `CABINET_VK_CLIENT_SECRET`, `CABINET_VK_CLIENT_REDIRECT_URL`

Примеры callback URL (для домена `https://cabinet.example.com`):

- Google callback:
  - `https://cabinet.example.com/cabinet/api/auth/google/callback`
- Yandex callback:
  - `https://cabinet.example.com/cabinet/api/auth/yandex/callback`
- VK callback:
  - `https://cabinet.example.com/cabinet/api/auth/vk/callback`
- Telegram callback:
  - `https://cabinet.example.com/cabinet/api/auth/telegram/callback`  

Где это настраивать:

- Google Cloud Console -> APIs & Services -> Credentials -> OAuth 2.0 Client -> Authorized redirect URIs
- Yandex OAuth -> кабинет приложения -> Redirect URI
- VK ID / VK OAuth app settings -> Redirect URI

### Telegram Auth 1.0 (Widget) и 2.0 (OIDC)

`CABINET_TELEGRAM_WEB_AUTH_MODE`:

- `widget` -> legacy Telegram Login Widget 1.0.
- `oidc` -> Telegram OAuth 2.0 / OIDC (рекомендуется).

Отличие:

- `widget`: старый сценарий, опирается на HMAC-данные виджета.
- `oidc`: современный OAuth flow с `client_id/client_secret/redirect_uri`, лучше подходит для web auth и дальнейшего расширения.

Важно: Mini App вход внутри Telegram не заменяется OIDC и использует `init_data`.

## 12) Настройка Telegram OIDC (рекомендуемый путь)

1. В BotFather настройте Login/OIDC для вашего бота.
2. В `.env`:
   - `CABINET_TELEGRAM_WEB_AUTH_MODE=oidc`
   - `CABINET_TELEGRAM_OIDC_CLIENT_ID=...`
   - `CABINET_TELEGRAM_OIDC_CLIENT_SECRET=...`
   - `CABINET_TELEGRAM_OIDC_REDIRECT_URL=https://cabinet.example.com/cabinet/api/auth/telegram/callback`
3. В BotFather добавьте:
   - тот же Redirect URI,
   - Trusted Origin: `https://cabinet.example.com`.
4. Выполните `/setdomain` на тот же домен.

Проверочный минимальный env-блок:

```env
CABINET_TELEGRAM_WEB_AUTH_MODE=oidc
CABINET_TELEGRAM_OIDC_CLIENT_ID=your_client_id
CABINET_TELEGRAM_OIDC_CLIENT_SECRET=your_client_secret
CABINET_TELEGRAM_OIDC_REDIRECT_URL=https://cabinet.example.com/cabinet/api/auth/telegram/callback
```

ВАЖНО по Telegram Auth 1.0/2.0:

- в вашей эксплуатации зафиксировано: после перехода бота в BotFather на Telegram Auth 2.0 вернуться к 1.0 через UI BotFather уже нельзя;
- фактически откат возможен только через пересоздание бота/новую сущность.

Полноценная работа Telegram Auth была проверена только на 2.0(OIDC), на 1.0 могут быть баги - пишите, буду фиксить.



## 13) Полный чеклист запуска

1. Поднять DNS.
2. Настроить proxy (Nginx/Caddy).
3. Получить SSL.
4. Заполнить `.env`.
5. Применить миграции.
6. Собрать/обновить сервис.
7. Настроить OAuth провайдеры.
8. Настроить Telegram (`/setdomain`, OIDC/widget).
9. Проверить health и auth bootstrap.
10. Проверить регистрацию/логин/checkout/подключения.

## 14) Smoke-тесты

```bash
curl -fsS "http://127.0.0.1:${HEALTH_CHECK_PORT}/healthcheck"
curl -fsS "http://127.0.0.1:${HEALTH_CHECK_PORT}/cabinet/api/healthz"
curl -fsS "http://127.0.0.1:${HEALTH_CHECK_PORT}/cabinet/api/auth/bootstrap"
```

Ручная проверка:

- открывается `/cabinet/`,
- можно войти и обновить токен,
- открывается `/cabinet/connections`,
- checkout доходит до статуса `paid`.

## 15) Раздел "CABINET_* переменные"

Подробные комментарии по каждой переменной уже поддерживаются в `.env.sample`.
Используйте его как единственный источник правды: там даны значения, дефолты и пояснения по рискам.

Для прод-стартера ориентируйтесь минимум на:

- `CABINET_ENABLED`
- `CABINET_PUBLIC_URL`
- `CABINET_ALLOWED_ORIGINS`
- `CABINET_JWT_SECRET`
- `CABINET_TELEGRAM_WEB_AUTH_MODE`
- `CABINET_TELEGRAM_OIDC_*` или `CABINET_TELEGRAM_LOGIN_BOT_USERNAME` (если widget)
- `CABINET_SMTP_*` + `CABINET_MAIL_FROM`
- `CABINET_TURNSTILE_*` (если включаете антибот)
- PWA (опционально): `CABINET_PWA_ENABLED=true` и при желании `CABINET_PWA_APP_NAME`, `CABINET_PWA_SHORT_NAME`; манифест отдаётся как `GET /cabinet/api/public/pwa-manifest.webmanifest`.

## 16) Частые ошибки

- Старый UI после деплоя -> не пересобран `web/cabinet` или старый образ.
- Telegram login не работает -> mismatch redirect/origin или забыли `/setdomain`.
- CORS/cookie проблемы -> неверный `CABINET_PUBLIC_URL`/`CABINET_ALLOWED_ORIGINS`/домен cookie.
- FAQ/Connections не обновляются -> не смонтирована папка `translations`.

## 17) Метрики, проверка логов в CI, rate limit, Turnstile, удаление аккаунта

### Зачем `GET /cabinet/api/metrics` и что с ним делать

Эндпоинт отдаёт метрики в формате **Prometheus / OpenMetrics** (отдельный registry только кабинета). Это **не обязательная** часть для «просто завести кабинет»: без Prometheus можно не трогать.

Зачем операторам и разработчикам:

- подключить **Prometheus** (или совместимый сборщик) с `scrape` на этот URL и строить **дашборды в Grafana**: попытки входа, checkout, длительность HTTP, merge;
- завести **алерты** (например, всплеск неуспешных логинов, аномалии по checkout);
- смотреть gauges **`cabinet_active_sessions`** и **`cabinet_web_only_customers`** (в коде они обновляются примерно **раз в 5 минут** — не для субсекундного мониторинга «прямо сейчас»).

Метрики включаются парой `CABINET_METRICS_USER` + `CABINET_METRICS_PASSWORD`: эндпоинт потребует HTTP Basic-auth с этими данными. Если пара не задана, `GET /cabinet/api/metrics` отвечает **404** — метрики выключены и посторонним недоступны.

### Rate limit в кабинете и заголовки `X-Forwarded-For`

На чувствительных маршрутах (логин, регистрация, forgot password, часть OAuth/link, оплаты и т.д.) стоит **in-memory** ограничение частоты запросов: при превышении клиент получает **HTTP 429** и заголовок `Retry-After`.

Ключ лимита часто включает **IP**. Заголовки `X-Forwarded-For` / `X-Real-IP` учитываются **только если** TCP-соединение пришло с **доверенного** адреса (loopback / частные сети — типичный случай: nginx на том же хосте или в docker-сети). `X-Forwarded-For` разбирается справа налево: адреса доверенных прокси пропускаются, первый чужой адрес считается клиентом — подставленная клиентом левая часть заголовка ни на что не влияет. Если опубликовать backend **напрямую** в интернет, злоумышленник может подставить фальшивый `X-Forwarded-For` и **исказить или обойти** привязку лимита к реальному клиенту.

Если перед nginx стоит ещё один прокси с публичным адресом (CDN, например Cloudflare), перечислите его диапазоны в `CABINET_TRUSTED_PROXIES` — иначе кабинет примет адрес CDN за адрес клиента и все пользователи попадут под один общий лимит.

Практическое правило: **всегда** ставьте nginx/Caddy перед ботом, не открывайте `HEALTH_CHECK_PORT` в мир без прокси — это и для TLS, и для корректного rate limit по IP.

### Как мониторить здоровье бота и не открывать `HEALTH_CHECK_PORT` наружу

`HEALTH_CHECK_PORT` — это порт всего HTTP-сервера бота (не только `/healthcheck`, но и `/cabinet/*`, `/cabinet/api/*` при включенном кабинете). Поэтому публиковать его в интернет как `0.0.0.0:3002:3002` нежелательно.

Безопасный вариант:

- публикуйте бот только на loopback хоста: `127.0.0.1:3002:3002`;
- внешний трафик пускайте через nginx/caddy на `127.0.0.1:3002`;
- Uptime Kuma держите в той же docker-сети и мониторьте бот по внутреннему DNS-имени сервиса.

Пример для вашей схемы:

- в compose бота:
  - `ports: ["127.0.0.1:3002:3002"]`
- в Kuma monitor URL:
  - `http://bot:3002/healthcheck`

Почему так безопаснее:

- нет прямого доступа с интернета к backend-порту бота;
- меньше поверхность атаки на auth/API маршруты;
- сложнее массово перебирать auth-эндпоинты в обход внешнего reverse-proxy политики.

### Cloudflare Turnstile

При `CABINET_TURNSTILE_ENABLED=true` backend ожидает заголовок **`X-Turnstile-Token`** на:

- `POST /cabinet/api/auth/register`
- `POST /cabinet/api/auth/login`
- `POST /cabinet/api/auth/password/forgot`

Встроенная SPA сама читает `turnstile_enabled` и `turnstile_site_key` из `GET /cabinet/api/auth/bootstrap`, запрашивает виджет только при включённом флаге и подставляет заголовок только для этих запросов. При `CABINET_TURNSTILE_ENABLED=false` поведение форм не меняется.

### Удаление аккаунта и Remnawave

`POST /cabinet/api/me/account/delete`: в актуальной логике сначала удаляется пользователь в **Remnawave** (если найден), затем cabinet-аккаунт — чтобы не оставлять активную подписку «висящей» в панели без записи в кабинете.

### Как мониторить здоровье бота и не открывать `HEALTH_CHECK_PORT` наружу

`HEALTH_CHECK_PORT` — это порт всего HTTP-сервера бота (не только `/healthcheck`, но и `/cabinet/*`, `/cabinet/api/*` при включенном кабинете). Поэтому публиковать его в интернет как `0.0.0.0:3002:3002` нежелательно.

Безопасный вариант:

- публикуйте бот только на loopback хоста: `127.0.0.1:3002:3002`;
- внешний трафик пускайте через nginx/caddy на `127.0.0.1:3002`;
- Uptime Kuma держите в той же docker-сети и мониторьте бот по внутреннему DNS-имени сервиса.

Пример для вашей схемы:

- в compose бота:
  - `ports: ["127.0.0.1:3002:3002"]`
- в Kuma monitor URL:
  - `http://bot:3002/healthcheck`

Почему так безопаснее:

- нет прямого доступа с интернета к backend-порту бота;
- меньше поверхность атаки на auth/API маршруты;
- сложнее массово перебирать auth-эндпоинты в обход внешнего reverse-proxy политики.

## 18) Чат поддержки (support bridge)

Опциональная интеграция web-кабинета с отдельным сервисом **[telegram-support-bot](https://github.com/Jolymmiels/telegram-support-bot)**: пользователь пишет в SPA, саппорт отвечает в топике Telegram-группы, ответ возвращается в кабинет.

### Архитектура

```
Кабинет (SPA) → shop API → support-bot HTTP → топик в супергруппе
Саппорт отвечает в TG → shop webhook → polling в кабинете
Telegram DM в support-боте — без изменений (source=telegram)
```

При **`SUPPORT_BOT_API=false`** (по умолчанию) кнопка «Поддержка» в кабинете ведёт на **`SUPPORT_URL`**, как раньше.

### Требования

- Работающий web-кабинет (`CABINET_ENABLED=true`).
- Отдельно развёрнутый **telegram-support-bot** (форум-супергруппа, `/setprivacy` → Disable — см. README support-bot).
- Миграция shop: **`000036_cabinet_support`** (`cabinet_support_ticket`, `cabinet_support_message`).

### Переменные shop (`.env`)

| Переменная | Описание |
|------------|----------|
| `SUPPORT_BOT_API` | `true` — включить встроенный чат |
| `SUPPORT_BOT_API_URL` | Базовый URL HTTP API support-bot, напр. `http://support-bot:8080` |
| `SUPPORT_BRIDGE_SECRET` | Общий секрет (Bearer); тот же, что в `.env` support-bot |

При `SUPPORT_BOT_API=true` без `SUPPORT_BOT_API_URL` или `SUPPORT_BRIDGE_SECRET` процесс shop не стартует.

Фронт узнаёт о включении из **`GET /cabinet/api/auth/bootstrap`** → поле **`support_chat_enabled`**.

### Переменные support-bot (`.env`)

| Переменная | Описание |
|------------|----------|
| `SHOP_WEBHOOK_URL` | URL webhook shop, напр. `http://shop:8080/cabinet/api/internal/support/webhook` |
| `SUPPORT_BRIDGE_SECRET` | Тот же секрет, что в shop |
| `HTTP_PORT` | Порт HTTP sidecar (по умолчанию `8080`) |

### Docker: одна сеть

Оба контейнера должны видеть друг друга по **имени сервиса** из compose, не через `localhost` на хосте.

**Shop `.env` (пример):**

```env
SUPPORT_BOT_API=true
SUPPORT_BOT_API_URL=http://support-bot:8080
SUPPORT_BRIDGE_SECRET=<общий-секрет>
```

**Support-bot `.env` (пример):**

```env
SHOP_WEBHOOK_URL=http://shop:8080/cabinet/api/internal/support/webhook
SUPPORT_BRIDGE_SECRET=<тот-же-секрет>
HTTP_PORT=8080
```

Имена `shop` / `support-bot` — примеры; подставьте реальные `service:` из ваших `docker-compose.yml`. Порт в `SHOP_WEBHOOK_URL` — **`HEALTH_CHECK_PORT`** shop (не публичный 443 nginx).

Если compose в разных файлах — подключите оба сервиса к одной external network.

### API shop

Пользовательские (JWT + CSRF на POST):

- `GET /cabinet/api/support/summary` — open-тикет, счётчик непрочитанных
- `GET /cabinet/api/support/conversation` — сообщения open-тикета
- `POST /cabinet/api/support/messages` — `{"text":"..."}` (только текст)
- `POST /cabinet/api/support/read` — отметить входящие прочитанными

Внутренний webhook (Bearer `SUPPORT_BRIDGE_SECRET`):

- `POST /cabinet/api/internal/support/webhook` — ответы саппорта и событие закрытия тикета

Idempotency: исходящие сообщения пользователя сохраняют **`client_message_id`** (UUID) в `cabinet_support_message`; повтор при сетевом retry не создаёт дубликат.

### Ограничения MVP

- Только **текст** от пользователя кабинета.
- **Медиа** от саппорта в кабинет **не доставляются**.
- **Один open-тикет** на `account_id`.
- После **`/close`** в топике история в модале **не показывается**; новое сообщение открывает **новый** тикет.

### Проверка после настройки

1. Применить миграции shop (в т.ч. `000036`).
2. Перезапустить shop и support-bot с согласованным секретом.
3. Bootstrap:

```bash
curl -fsS "http://127.0.0.1:${HEALTH_CHECK_PORT}/cabinet/api/auth/bootstrap" | jq '.support_chat_enabled'
# ожидается true
```

4. В кабинете (авторизованным пользователем): «Поддержка» → модальный чат → отправить текст → в группе support-bot появился топик.
5. Ответ саппорта в топике → через несколько секунд (polling SPA) текст в кабинете.
6. `/close` в топике → чат в кабинете закрывается; новое сообщение — новый тикет.

**Код:** `internal/cabinet/service/support.go`, `internal/cabinet/supportbot/client.go`, `web/cabinet/src/features/support/`.

---



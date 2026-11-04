# Обратный прокси

Бот слушает `HEALTH_CHECK_PORT` (healthcheck, кабинет, вебхуки оплат). Снаружи этот порт лучше не открывать напрямую — поставьте nginx, Caddy или Traefik.

## Пути, которые нужно пропустить

| Путь | Что это |
|------|---------|
| `/cabinet/` | SPA кабинета |
| `/cabinet/api/` | API кабинета |
| `/landing` | публичный лендинг на корне домена (см. [cabinet/landing.md](./cabinet/landing.md)) |
| `/healthcheck` | проба живости |
| вебхуки оплат | пути из `.env` (`YOOKASA_WEBHOOK_URL`, `PLATEGA_WEBHOOK_URL`, `HELEKET_WEBHOOK_URL`, `TRIBUTE_WEBHOOK_URL`) |
| вебхук панели | `REMNAWAVE_WEBHOOK_PATH`, если панель шлёт его через прокси ([notifications.md](./notifications.md)) |

Кабинет берёт IP клиента из `X-Forwarded-For` и доверяет только loopback и частным сетям. Если перед вашим nginx стоит ещё один прокси с публичным адресом (например CDN), перечислите его адреса в `CABINET_TRUSTED_PROXIES` — иначе все клиенты получат IP CDN и общий лимит запросов. См. [env.md](./env.md#web-кабинет).

Если конфиг проксирует на бота всё (`location /`), делать ничего не нужно. Если
посторонние пути уводятся редиректом на `/cabinet/` — для `/landing` нужен свой
`location`, иначе короткий адрес не откроется. Шаблон в
`scripts/meows-shop-setup.sh` это уже учитывает; конфиг, собранный вручную,
придётся дописать. Лендинг всегда доступен и по `/cabinet/landing` — этот путь
работает без правок прокси.

Для web-кабинета с доменом и SSL удобнее следовать гайду:

- [cabinet/SETUP-GUIDE-RU.md](./cabinet/SETUP-GUIDE-RU.md)
- пункт меню установщика `scripts/meows-shop-setup.sh` → **Reverse-proxy / SSL**

## Пример Traefik

Если не используете ngrok из `docker-compose`, можно так:

```yaml
http:
  routers:
    remnawave-telegram-shop:
      rule: "Host(`bot.example.com`)"
      entrypoints:
        - http
      middlewares:
        - redirect-to-https
      service: remnawave-telegram-shop

    remnawave-telegram-shop-secure:
      rule: "Host(`bot.example.com`)"
      entrypoints:
        - https
      tls:
        certResolver: letsencrypt
      service: remnawave-telegram-shop

  middlewares:
    redirect-to-https:
      redirectScheme:
        scheme: https

  services:
    remnawave-telegram-shop:
      loadBalancer:
        servers:
          - url: "http://bot:3002"
```

Подставьте свой хост и порт сервиса из вашего `docker-compose` (значение `HEALTH_CHECK_PORT` / проброс порта контейнера).

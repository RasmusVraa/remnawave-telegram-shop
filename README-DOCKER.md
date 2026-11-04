# Инструкция по использованию Docker образа

## Авторизация в GitHub Container Registry

Если пакет приватный, нужно авторизоваться в Docker:

```bash
# Создайте Personal Access Token с правами read:packages
# Затем выполните:
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin
```

Или добавьте в `~/.docker/config.json`:

```json
{
  "auths": {
    "ghcr.io": {
      "auth": "BASE64_ENCODED_USERNAME:TOKEN"
    }
  }
}
```

## Использование образа

### Production (готовый образ из GitHub Packages)

```bash
docker compose up -d
```

Использует образ: `ghcr.io/mrme0ws/remnawave-telegram-shop-bot:latest`

Теги образа:

- `latest` и номер версии (`5.26.0`, а также `5.26`) — собираются из git-тегов релизов; теги без префикса `v`;
- `dev-<короткий sha>` — сборка каждого коммита в `main`, для тестовых стендов.

### Локальная сборка

`Dockerfile` сам собирает фронтенд кабинета и встраивает его в бинарь:

```bash
docker build -t remnawave-telegram-shop-bot:local .
```

Затем в `docker-compose.yaml` укажите `image: remnawave-telegram-shop-bot:local`.

## Проверка доступности образа

```bash
docker pull ghcr.io/mrme0ws/remnawave-telegram-shop-bot:latest
```

## Использование конкретной версии

В `docker-compose.yaml` замените:

```yaml
image: ghcr.io/mrme0ws/remnawave-telegram-shop-bot:latest
```

На нужную версию, например:

```yaml
image: ghcr.io/mrme0ws/remnawave-telegram-shop-bot:5.26.0
```

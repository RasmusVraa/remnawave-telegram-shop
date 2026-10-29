# tools/release

Локальный CLI: GitHub Release + анонс в Telegram-форум из `RELEASE_NOTES.md`.

Не входит в Docker-образ магазина (сборка только `./cmd/app`; каталог `tools/` в `.dockerignore`).

## Setup

```bash
cp .env.release.sample .env.release
# заполни RELEASE_TG_BOT_TOKEN и при необходимости chat/thread
```

Бот должен быть админом супергруппы и иметь право писать в тему (`RELEASE_TG_MESSAGE_THREAD_ID`).

Если `api.telegram.org` резолвится в `198.18.*` (Clash fake-IP) — задай `RELEASE_TG_PROXY_URL` на локальный HTTP/SOCKS прокси (для FlClash обычно `http://127.0.0.1:7890`).

## Prerequisites before `github` / `publish`

- Нужные коммиты уже на целевой ветке (`main`), с которой `gh` возьмёт HEAD для нового тега.
- Теги **без** префикса `v`: `4.12.2`, не `v4.12.2` (как остальные релизы репо). CLI сам срежет ведущий `v`, если он попал в `RELEASE_NOTES.md`.
- Если тег `X.Y.Z` ещё не существует, `gh release create` создаст его на текущем HEAD remote default branch — убедись, что это нужный коммит.
- Повторный `github`/`publish` для того же тега упадёт (релиз уже есть); повторный `telegram` отправит дубликат поста.

## Commands

Из корня репозитория:

```bash
go run ./tools/release preview
go run ./tools/release github
go run ./tools/release telegram
go run ./tools/release publish
```

`publish` = `github` затем `telegram`.

## Скриншоты к посту

Положи картинки (`png`, `jpg`, `webp`, до 10 МБ каждая, не больше 10 штук) в
`release-screenshots/` в корне репозитория — `telegram` и `publish` отправят их
альбомом в ту же тему. Порядок — по имени файла,
поэтому удобно начинать имена с `01-`, `02-`. Папка в `.gitignore`; другая —
через `RELEASE_TG_PHOTOS_DIR`. Нет папки — нет альбома. `preview` показывает,
какие файлы уйдут.

Подпись к фото в Telegram режется на 1024 символах. Пост, который в них
влезает (считается видимый текст, без тегов), уходит подписью к альбому — одним
сообщением. Длинный пост уходит отдельно, а альбом — ответом на него.

## Recovery

Если `publish` создал GitHub Release, но Telegram упал:

```bash
go run ./tools/release telegram
```

Если пост ушёл, а альбом нет — повтори только альбом, иначе пост задвоится
(номер поста CLI печатает в выводе):

```bash
go run ./tools/release photos <message_id>
```

Лимит текста Telegram — 4096 символов (руны); при превышении сократи блок «Вариант для Telegram».

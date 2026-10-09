#!/usr/bin/env bash
# Meows Remnawave Telegram Shop — интерактивный установщик (Linux + Docker).
# Меню: bot only | bot+cabinet | cabinet only | proxy/SSL | smoke.
#
# Быстрый старт с GitHub (одна команда на чистой VDS):
#   bash <(curl -fsSL https://raw.githubusercontent.com/MrMe0ws/remnawave-telegram-shop/main/scripts/meows-shop-setup.sh)
#
# Опционально:
#   MEOWS_DIR=/opt/remnawave-telegram-shop   — куда клонировать (если репо ещё нет)
#   MEOWS_CHOICE=2                          — сразу пункт меню (1..5), без первого вопроса
#   MEOWS_IMAGE_TAG=latest|4.12.1           — тег Docker-образа бота (без вопроса)
#
# Что скачивается:
#   • git clone https://github.com/MrMe0ws/remnawave-telegram-shop.git (ветка main)
#     по умолчанию в /opt/remnawave-telegram-shop
#   • docker pull образа ghcr.io/mrme0ws/remnawave-telegram-shop-bot:<tag>
#     tag = latest или версия вроде 4.12.1 (выбор в wizard / MEOWS_IMAGE_TAG)
#
set -euo pipefail

SCRIPT_VERSION="1.1.0"
DEFAULT_IMAGE_REPO="ghcr.io/mrme0ws/remnawave-telegram-shop-bot"
DEFAULT_IMAGE="${DEFAULT_IMAGE_REPO}:latest"
DEFAULT_NETWORK="remnawave-network"
DEFAULT_PORT="3002"
DEFAULT_REPO_URL="https://github.com/MrMe0ws/remnawave-telegram-shop.git"
DEFAULT_BRANCH="main"
DEFAULT_INSTALL_DIR="${MEOWS_DIR:-/opt/remnawave-telegram-shop}"
RAW_SETUP_URL="https://raw.githubusercontent.com/MrMe0ws/remnawave-telegram-shop/${DEFAULT_BRANCH}/scripts/meows-shop-setup.sh"

# --- colors (TTY only), стиль как у Remnawave reverse-proxy: cyan / yellow / gray ---
if [[ -t 1 ]]; then
  C_RESET=$'\033[0m'
  C_BOLD=$'\033[1m'
  C_DIM=$'\033[2m'
  C_GREEN=$'\033[1;32m'    # яркий зелёный — название скрипта / OK
  C_YELLOW=$'\033[1;33m'   # яркий жёлтый — меню и вопросы
  C_RED=$'\033[1;31m'
  C_CYAN=$'\033[1;36m'     # яркий cyan — заголовки, ?
  C_GRAY=$'\033[90m'       # серый — мета, скобки
  C_WHITE=$'\033[37m'
else
  C_RESET= C_BOLD= C_DIM= C_GREEN= C_YELLOW= C_RED= C_CYAN= C_GRAY= C_WHITE=
fi

# Промпт в стиле: [?] текст
prompt_prefix() {
  printf '%s[%s%s?%s%s]%s %s%s%s' \
    "${C_GRAY}" "${C_RESET}" "${C_CYAN}" "${C_RESET}" "${C_GRAY}" "${C_RESET}" \
    "${C_YELLOW}" "$1" "${C_RESET}"
}

log()  { printf '%s\n' "$*"; }
menu() { printf '%s%s%s\n' "${C_YELLOW}" "$*" "${C_RESET}"; }
meta() { printf '%s%s%s\n' "${C_GRAY}" "$*" "${C_RESET}"; }
info() { printf '%s%s%s\n' "${C_CYAN}" "$*" "${C_RESET}"; }
ok()   { printf '%s%s%s\n' "${C_GREEN}" "✓ $*" "${C_RESET}"; }
warn() { printf '%s%s%s\n' "${C_YELLOW}" "! $*" "${C_RESET}"; }
err()  { printf '%s%s%s\n' "${C_RED}" "✗ $*" "${C_RESET}" >&2; }
die()  { err "$*"; exit 1; }

header() {
  # Заголовок секции — cyan, как title в примере
  printf '\n%s%s%s\n' "${C_CYAN}" "$*" "${C_RESET}"
  printf '%s%s%s\n\n' "${C_GRAY}" "────────────────────────────────────────" "${C_RESET}"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "Нужна команда: $1"
}

ask() {
  # ask VAR "prompt" ["default"]
  # Дефолт подставляется в строку ввода (readline -i): Enter = оставить, или отредактировать.
  # Внутри НЕ используем имя __ans — иначе printf -v "__ans" при set -u ломает вызывающий ask_choice_123.
  local __var="$1" __prompt="$2" __default="${3-}" __reply="" __pfx
  __pfx="$(prompt_prefix "${__prompt}:")"
  if [[ -n "$__default" ]]; then
    if [[ -t 0 ]]; then
      read -e -i "$__default" -r -p "${__pfx} " __reply || true
    else
      read -r -p "${__pfx} " __reply || true
    fi
    __reply="${__reply:-$__default}"
  else
    if [[ -t 0 ]]; then
      read -e -r -p "${__pfx} " __reply || true
    else
      read -r -p "${__pfx} " __reply || true
    fi
  fi
  printf -v "$__var" '%s' "${__reply-}"
}

ask_secret() {
  # Видимый ввод (токены удобно вставлять/проверять на VDS). Не скрываем символы.
  # ask_secret VAR "prompt" ["default"]
  ask "$@"
}

ask_yn() {
  # ask_yn VAR "prompt" [Y|N]
  local __var="$1" __prompt="$2" __def="${3:-N}" __reply="" __hint __pfx
  if [[ "$__def" =~ ^[Yy]$ ]]; then __hint="Y/n"; else __hint="y/N"; fi
  __pfx="$(prompt_prefix "${__prompt} (${__hint}):")"
  if [[ -t 0 ]]; then
    read -e -r -p "${__pfx} " __reply || true
  else
    read -r -p "${__pfx} " __reply || true
  fi
  __reply="${__reply:-$__def}"
  case "$__reply" in
    [Yy]|[Yy][Ee][Ss]) printf -v "$__var" 'y' ;;
    *) printf -v "$__var" 'n' ;;
  esac
}

confirm_or_exit() {
  local ans
  ask_yn ans "Продолжить?" Y
  [[ "$ans" == "y" ]] || die "Отменено пользователем"
}

# --- paths ---
REPO_ROOT=""
ENV_FILE=""
COMPOSE_FILE=""
BACKUP_DIR=""

script_is_piped() {
  # curl|bash / bash <(curl ...) — BASH_SOURCE часто /dev/fd/* или stdin
  local src="${BASH_SOURCE[0]-}"
  [[ ! -f "$src" ]] && return 0
  [[ "$src" == /dev/fd/* || "$src" == /proc/self/fd/* ]] && return 0
  return 1
}

resolve_repo_root() {
  local candidate src_dir
  if [[ -f "./docker-compose.yaml" && -f "./.env.sample" ]]; then
    REPO_ROOT="$(pwd -P)"
    return 0
  fi
  # Не резолвим parent от /dev/fd при curl|bash
  if ! script_is_piped; then
    src_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
    candidate="$(cd "${src_dir}/.." && pwd -P)"
    if [[ -f "$candidate/docker-compose.yaml" && -f "$candidate/.env.sample" ]]; then
      REPO_ROOT="$candidate"
      return 0
    fi
  fi
  if [[ -n "${MEOWS_DIR:-}" && -f "${MEOWS_DIR}/docker-compose.yaml" && -f "${MEOWS_DIR}/.env.sample" ]]; then
    REPO_ROOT="$(cd "$MEOWS_DIR" && pwd -P)"
    return 0
  fi
  # Типичный путь установки — не клонировать заново при запуске из /opt
  if [[ -f "${DEFAULT_INSTALL_DIR}/docker-compose.yaml" && -f "${DEFAULT_INSTALL_DIR}/.env.sample" ]]; then
    REPO_ROOT="$(cd "$DEFAULT_INSTALL_DIR" && pwd -P)"
    return 0
  fi
  return 1
}

init_paths() {
  ENV_FILE="${REPO_ROOT}/.env"
  COMPOSE_FILE="${REPO_ROOT}/docker-compose.yaml"
  BACKUP_DIR="${REPO_ROOT}/.setup-backups"
  mkdir -p -m 700 "$BACKUP_DIR" 2>/dev/null || mkdir -p "$BACKUP_DIR"
  chmod 700 "$BACKUP_DIR" 2>/dev/null || true
}

clone_or_update_repo() {
  local dest="$1" clone_url="$2" branch="$3"
  need_cmd git
  if [[ -d "$dest/.git" ]]; then
    warn "Каталог уже есть: $dest"
    local do_pull
    ask_yn do_pull "git fetch/pull в этом каталоге?" Y
    if [[ "$do_pull" == "y" ]]; then
      git -C "$dest" fetch --depth 1 origin "$branch" || git -C "$dest" fetch origin || true
      git -C "$dest" checkout "$branch" 2>/dev/null || true
      git -C "$dest" pull --ff-only origin "$branch" 2>/dev/null \
        || warn "git pull не удался — продолжаю с тем, что есть"
    fi
  elif [[ -d "$dest" ]] && [[ ! -f "$dest/docker-compose.yaml" ]]; then
    die "Каталог $dest существует, но это не репозиторий shop (нет docker-compose.yaml)"
  elif [[ -d "$dest" ]]; then
    ok "Использую существующий каталог: $dest"
  else
    info "Клонирую ${clone_url} (ветка ${branch}) → ${dest}"
    git clone --branch "$branch" --depth 1 "$clone_url" "$dest" \
      || git clone --depth 1 "$clone_url" "$dest"
  fi
}

ensure_repo() {
  # Быстрый bootstrap: нет локального репо → клон с GitHub
  if [[ -n "${REPO_ROOT}" ]]; then
    init_paths
    cd "$REPO_ROOT"
    return 0
  fi
  if resolve_repo_root; then
    init_paths
    cd "$REPO_ROOT"
    ok "Проект: $REPO_ROOT"
    return 0
  fi

  header "Клонирование Meows Shop с GitHub"
  info "Нужны: docker-compose.yaml, .env.sample, translations."
  info "Бот запускается из Docker-образа ghcr.io (не сборка из исходников)."
  info "Источник: ${DEFAULT_REPO_URL} (ветка ${DEFAULT_BRANCH})"
  local dest
  ask dest "Каталог установки" "$DEFAULT_INSTALL_DIR"
  dest="${dest:-$DEFAULT_INSTALL_DIR}"
  # Если указали только /opt — ставим в /opt/remnawave-telegram-shop
  if [[ "$dest" == "/opt" || "$dest" == "/opt/" ]]; then
    dest="/opt/remnawave-telegram-shop"
    info "Использую: $dest"
  fi
  # Уже установлено ранее — не клонируем поверх
  if [[ -f "$dest/docker-compose.yaml" && -f "$dest/.env.sample" ]]; then
    ok "Найдена существующая установка: $dest"
    cd "$dest"
    REPO_ROOT="$(pwd -P)"
    init_paths
    ok "Проект: $REPO_ROOT"
    return 0
  fi
  # Родитель должен существовать (/opt обычно есть)
  local parent
  parent="$(dirname "$dest")"
  if [[ ! -d "$parent" ]]; then
    info "Создаю родительский каталог: $parent"
    mkdir -p "$parent" || die "Не удалось создать $parent (нужен root/sudo?)"
  fi
  clone_or_update_repo "$dest" "$DEFAULT_REPO_URL" "$DEFAULT_BRANCH"
  cd "$dest"
  REPO_ROOT="$(pwd -P)"
  init_paths
  ok "Проект установлен в: $REPO_ROOT"
}

# Список запущенных контейнеров с nginx в имени (для выбора reload).
detect_nginx_containers() {
  docker ps --format '{{.Names}}' 2>/dev/null | grep -iE 'nginx|caddy' || true
}

pick_nginx_container() {
  local __out="$1" __default="${2:-remnawave-nginx}" __pick __n=0
  local __list=()
  local __line
  while IFS= read -r __line; do
    [[ -n "$__line" ]] && __list+=("$__line")
  done < <(detect_nginx_containers)
  if [[ ${#__list[@]} -eq 0 ]]; then
    warn "Запущенных контейнеров nginx/caddy не видно (docker ps)."
    info "Типичное имя у Remnawave: remnawave-nginx (это НЕ путь к nginx.conf!)"
    ask __pick "Имя контейнера nginx (колонка NAMES в docker ps)" "$__default"
  elif [[ ${#__list[@]} -eq 1 ]]; then
    info "Найден контейнер: ${__list[0]}"
    ask __pick "Имя контейнера nginx" "${__list[0]}"
  else
    info "Найдены контейнеры:"
    local i
    for i in "${!__list[@]}"; do
      info "  $((i + 1))) ${__list[$i]}"
    done
    ask __n "Номер из списка (или 0 — ввести имя вручную)" "1"
    if [[ "$__n" =~ ^[1-9][0-9]*$ ]] && (( __n >= 1 && __n <= ${#__list[@]} )); then
      __pick="${__list[$((__n - 1))]}"
    else
      ask __pick "Имя контейнера nginx" "$__default"
    fi
  fi
  # Частая ошибка: указали путь к conf вместо имени контейнера
  if [[ "$__pick" == *.conf || "$__pick" == /* ]]; then
    warn "«${__pick}» похоже на путь к файлу, а нужно имя контейнера (например remnawave-nginx)."
    ask __pick "Имя контейнера nginx" "$__default"
  fi
  printf -v "$__out" '%s' "$__pick"
}

ask_choice_123() {
  # ask_choice_123 VAR "prompt" default(1|2|3)
  local __var="$1" __prompt="$2" __def="${3:-1}" __choice=""
  while true; do
    ask __choice "$__prompt" "$__def"
    case "$__choice" in
      1|2|3) printf -v "$__var" '%s' "$__choice"; return 0 ;;
      *) warn "Введите только 1, 2 или 3 (не «12»)" ;;
    esac
  done
}

wait_nginx_running() {
  local name="$1" tries="${2:-30}" i status
  for ((i = 1; i <= tries; i++)); do
    status="$(docker inspect -f '{{.State.Status}}' "$name" 2>/dev/null || echo missing)"
    if [[ "$status" == "running" ]]; then
      # не в restart-loop: RestartingCount стабилен и процесс жив
      if docker exec "$name" true >/dev/null 2>&1; then
        return 0
      fi
    fi
    sleep 1
  done
  return 1
}

reload_nginx_container() {
  local name="$1"
  [[ -n "$name" ]] || return 1

  if docker ps -a --format '{{.Names}}' | grep -qx "$name"; then
    local st
    st="$(docker inspect -f '{{.State.Status}}' "$name" 2>/dev/null || true)"
    if [[ "$st" != "running" ]]; then
      info "Контейнер $name в состоянии «${st}» — перезапускаю…"
      docker start "$name" >/dev/null 2>&1 || docker restart "$name" >/dev/null 2>&1 || true
    fi
  else
    warn "Контейнер «${name}» не найден."
    return 1
  fi

  if ! wait_nginx_running "$name" 40; then
    err "nginx не вышел в running (возможен restart-loop из‑за ошибки в conf / отсутствовал SSL)."
    warn "Последние логи:"
    docker logs "$name" --tail 40 2>&1 || true
    info "Почините conf и: docker restart $name"
    return 1
  fi

  if docker exec "$name" nginx -t; then
    docker exec "$name" nginx -s reload
    ok "nginx reload OK ($name)"
    return 0
  fi
  warn "nginx -t failed в контейнере $name"
  docker exec "$name" nginx -t 2>&1 || true
  warn "Откат: бэкапы в ${BACKUP_DIR:-.setup-backups}"
  return 1
}

# Вставить или заменить блок кабинета (по маркерам Meows) + обновить ssl_ paths для домена.
upsert_cabinet_nginx_block() {
  local conf="$1" snippet="$2" domain="$3" cert_full="$4" cert_key="$5"
  local begin="# --- Meows Shop Web Cabinet (generated by meows-shop-setup.sh) ---"
  local end="# --- end Meows Shop Web Cabinet ---"
  local tmp

  backup_file "$conf"

  # Обновить пути LE для этого домена во всём conf (старый блок мог ссылаться на несуществующий cert)
  if grep -qF "server_name ${domain}" "$conf" 2>/dev/null; then
    sed -i -E \
      -e "s|ssl_certificate[[:space:]]+[^;]*${domain}[^;]*;|ssl_certificate     ${cert_full};|g" \
      -e "s|ssl_certificate_key[[:space:]]+[^;]*${domain}[^;]*;|ssl_certificate_key ${cert_key};|g" \
      "$conf" || true
    # Типичные пути LE без опечаток в домене
    sed -i -E \
      -e "s|ssl_certificate[[:space:]]+/etc/letsencrypt/live/${domain}/fullchain\.pem;|ssl_certificate     ${cert_full};|g" \
      -e "s|ssl_certificate_key[[:space:]]+/etc/letsencrypt/live/${domain}/privkey\.pem;|ssl_certificate_key ${cert_key};|g" \
      "$conf" || true
    ok "Обновлены ssl_certificate* для ${domain} (если были)"
  fi

  if grep -qF "$begin" "$conf" 2>/dev/null; then
    tmp="$(mktemp)"
    awk -v begin="$begin" -v end="$end" -v snipfile="$snippet" '
      BEGIN { skip=0 }
      index($0, begin) == 1 {
        skip=1
        while ((getline line < snipfile) > 0) print line
        close(snipfile)
        next
      }
      index($0, end) == 1 { skip=0; next }
      skip == 0 { print }
    ' "$conf" >"$tmp"
    mv "$tmp" "$conf"
    ok "Заменён существующий блок Meows Cabinet в $conf"
    return 0
  fi

  if grep -qF "server_name ${domain}" "$conf" 2>/dev/null; then
    warn "Блок server_name ${domain} есть, но без маркеров Meows — не дублирую server{}."
    info "ssl_ пути обновлены выше. При ошибке nginx — замените блок вручную из snippet."
    return 0
  fi

  printf '\n' >>"$conf"
  cat "$snippet" >>"$conf"
  ok "Блоки кабинета добавлены в $conf"
}

# Файл серта реально читается ВНУТРИ контейнера (не путать с mount Source на хосте).
cert_visible_in_container() {
  local name="$1" path="$2"
  docker exec "$name" test -f "$path" >/dev/null 2>&1
}

# Destination mount именно /etc/letsencrypt (целиком), а не отдельные live/*.pem → /etc/nginx/ssl/...
container_has_full_letsencrypt_dest() {
  local name="$1"
  docker inspect "$name" --format '{{range .Mounts}}{{.Destination}}{{println}}{{end}}' 2>/dev/null \
    | grep -qx '/etc/letsencrypt'
}

find_remnawave_compose() {
  local c
  for c in \
    /opt/remnawave/docker-compose.yml \
    /opt/remnawave/docker-compose.yaml \
    /root/remnawave/docker-compose.yml \
    /root/remnawave/docker-compose.yaml; do
    if [[ -f "$c" ]]; then
      printf '%s' "$c"
      return 0
    fi
  done
  return 1
}

# Добавить volume /etc/letsencrypt:/etc/letsencrypt:ro в compose nginx и recreate контейнер.
ensure_nginx_sees_letsencrypt() {
  local name="$1" domain="$2"
  local host_full="/etc/letsencrypt/live/${domain}/fullchain.pem"
  local in_full="/etc/letsencrypt/live/${domain}/fullchain.pem"
  local compose="" service="" do_patch="n"

  [[ -n "$name" ]] || return 1
  [[ -f "$host_full" ]] || {
    err "На хосте нет $host_full"
    return 1
  }

  # Контейнер в restart-loop всё равно может ответить на inspect; exec часто падает — пробуем
  if docker ps --format '{{.Names}}' | grep -qx "$name" && cert_visible_in_container "$name" "$in_full"; then
    ok "Внутри $name файл виден: $in_full"
    return 0
  fi

  if container_has_full_letsencrypt_dest "$name"; then
    warn "Mount /etc/letsencrypt есть, но файла $in_full внутри не видно."
    warn "Проверьте на хосте: ls -la /etc/letsencrypt/live/${domain}/ (это symlink → archive/)."
  else
    warn "Контейнер $name НЕ монтирует каталог /etc/letsencrypt целиком."
    warn "Поэтому пути ssl_certificate /etc/letsencrypt/live/... внутри nginx «No such file»."
    info "Нужно:  - /etc/letsencrypt:/etc/letsencrypt:ro  в docker-compose сервиса nginx, затем recreate."
  fi

  compose="$(find_remnawave_compose || true)"
  if [[ -z "$compose" ]]; then
    ask compose "Путь к docker-compose Remnawave (для правки volumes nginx)" "/opt/remnawave/docker-compose.yml"
  fi
  [[ -f "$compose" ]] || die "Не найден compose Remnawave: $compose"

  if grep -qE '[[:space:]]/etc/letsencrypt:/etc/letsencrypt' "$compose"; then
    ok "В $compose уже есть mount /etc/letsencrypt"
  else
    ask_yn do_patch "Добавить volume /etc/letsencrypt:/etc/letsencrypt:ro в $compose?" Y
    if [[ "$do_patch" == "y" ]]; then
      backup_file "$compose"
      if grep -qE 'nginx\.conf:/etc/nginx/conf\.d' "$compose"; then
        sed -i '/nginx\.conf:\/etc\/nginx\/conf\.d/a\      - /etc/letsencrypt:/etc/letsencrypt:ro' "$compose"
        ok "Volume добавлен после строки nginx.conf"
      else
        warn "Не нашёл строку nginx.conf в compose — добавьте вручную в volumes nginx:"
        warn "  - /etc/letsencrypt:/etc/letsencrypt:ro"
        die "Сначала добавьте volume, затем повторите пункт 4"
      fi
    else
      die "Без mount /etc/letsencrypt кабинетный SSL в этом nginx не заработает"
    fi
  fi

  # Имя сервиса в compose (часто remnawave-nginx)
  service="$(awk -v cname="$name" '
    /^[[:space:]]*[a-zA-Z0-9_-]+:/ { svc=$1; sub(/:/,"",svc); gsub(/^[[:space:]]+/,"",svc) }
    /container_name:/ {
      gsub(/['\''"]/,"",$2)
      if ($2 == cname) { print svc; exit }
    }
  ' "$compose" 2>/dev/null || true)"
  [[ -n "$service" ]] || service="remnawave-nginx"

  info "Пересоздаю контейнер ($service) чтобы подхватить volume…"
  (
    cd "$(dirname "$compose")"
    if docker compose version >/dev/null 2>&1; then
      docker compose -f "$(basename "$compose")" up -d --force-recreate "$service"
    else
      docker-compose -f "$(basename "$compose")" up -d --force-recreate "$service"
    fi
  ) || warn "compose up не удался — сделайте вручную: cd $(dirname "$compose") && docker compose up -d --force-recreate $service"

  sleep 3
  if wait_nginx_running "$name" 45 && cert_visible_in_container "$name" "$in_full"; then
    ok "После recreate сертификат виден внутри контейнера"
    return 0
  fi

  # Последняя проверка через docker run bind (диагностика)
  err "Сертификат всё ещё не виден в $name как $in_full"
  info "Диагностика:"
  info "  ls -la $host_full"
  info "  docker inspect $name --format '{{range .Mounts}}{{.Source}} -> {{.Destination}}{{\"\\n\"}}{{end}}'"
  info "  docker exec $name ls -la /etc/letsencrypt/live/${domain}/ || true"
  return 1
}

# Выпуск LE-серта «за руку»: webroot → fallback standalone (stop nginx → certbot → start).
# Возвращает 0 если /etc/letsencrypt/live/<domain>/fullchain.pem есть.
issue_letsencrypt_cert() {
  local domain="$1" nginx_container="${2-}"
  local host_full="/etc/letsencrypt/live/${domain}/fullchain.pem"
  local host_key="/etc/letsencrypt/live/${domain}/privkey.pem"
  local wroot="/var/www/html"
  local was_running="n" email=""

  if [[ -f "$host_full" && -f "$host_key" ]]; then
    ok "Сертификат уже есть: $host_full"
    return 0
  fi

  need_cmd certbot
  header "Выпуск SSL (Let's Encrypt) для ${domain}"
  info "Скрипт сам выпустит сертификат и вернёт nginx панели, если его останавливали."
  warn "Кратко могут быть недоступны сайты на этом nginx (панель Remnawave / sub / …)."
  ask email "Email для Let's Encrypt (пусто = без email)" ""
  local email_args=(--agree-tos --non-interactive)
  if [[ -n "$email" ]]; then
    email_args+=(--email "$email")
  else
    email_args+=(--register-unsafely-without-email)
  fi

  sudo mkdir -p "$wroot"
  # 1) Пробуем webroot без остановки nginx (если :80 уже отдаёт /.well-known)
  info "Пробую certbot --webroot (nginx не останавливаем)…"
  if sudo certbot certonly --webroot -w "$wroot" -d "$domain" "${email_args[@]}"; then
    if [[ -f "$host_full" ]]; then
      ok "Сертификат выпущен через webroot"
      return 0
    fi
  fi
  warn "webroot не сработал — перехожу на standalone (нужен свободный порт 80)."

  if [[ -n "$nginx_container" ]] && docker ps --format '{{.Names}}' | grep -qx "$nginx_container"; then
    was_running="y"
    info "Останавливаю контейнер ${nginx_container} на время выпуска сертификата…"
    docker stop "$nginx_container" >/dev/null
    ok "Контейнер остановлен: $nginx_container"
  elif [[ -n "$nginx_container" ]]; then
    warn "Контейнер $nginx_container сейчас не running — standalone без stop"
  else
    warn "Имя nginx-контейнера не задано — убедитесь, что порт 80 свободен"
  fi

  local ok_standalone="n"
  if sudo certbot certonly --standalone -d "$domain" "${email_args[@]}"; then
    [[ -f "$host_full" ]] && ok_standalone="y"
  fi

  # Всегда поднимаем nginx обратно, даже если certbot упал
  if [[ "$was_running" == "y" ]]; then
    info "Запускаю ${nginx_container} обратно…"
    if docker start "$nginx_container" >/dev/null; then
      sleep 2
      ok "Контейнер запущен: $nginx_container"
    else
      err "Не удалось docker start $nginx_container — поднимите вручную: docker start $nginx_container"
      err "Или: cd /opt/remnawave && docker compose up -d"
    fi
  fi

  if [[ "$ok_standalone" == "y" ]]; then
    ok "Сертификат выпущен: $host_full"
    return 0
  fi
  err "Не удалось выпустить сертификат для $domain"
  warn "Проверьте DNS A → этот сервер и что 80/443 доступны с интернета."
  return 1
}

require_repo() {
  ensure_repo
}

# --- preflight ---
preflight() {
  header "Проверки окружения"
  [[ "$(uname -s)" == "Linux" ]] || die "Скрипт только для Linux (сейчас: $(uname -s))"
  need_cmd docker
  if docker compose version >/dev/null 2>&1; then
    COMPOSE_CMD=(docker compose)
  elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE_CMD=(docker-compose)
  else
    die "Нужен Docker Compose (plugin: docker compose)"
  fi
  need_cmd curl
  need_cmd openssl
  need_cmd sed
  need_cmd awk
  need_cmd grep
  docker info >/dev/null 2>&1 || die "Docker daemon недоступен (нужны права / группа docker)"
  ok "Docker + Compose OK"
  if command -v certbot >/dev/null 2>&1; then
    ok "certbot найден"
  else
    warn "certbot не найден — выпуск SSL через скрипт будет недоступен (можно указать готовые сертификаты)"
  fi
}

# --- .env helpers ---
backup_file() {
  local f="$1"
  [[ -f "$f" ]] || return 0
  local ts base dest
  ts="$(date +%Y%m%d-%H%M%S)"
  base="$(basename "$f")"
  dest="${BACKUP_DIR}/${base}.${ts}.bak"
  cp -a "$f" "$dest"
  chmod 600 "$dest" 2>/dev/null || true
  ok "Бэкап: $dest"
}

# Escape value for .env double-quoted assignment when needed.
# Avoid [[ =~ ]] charclass with backtick — bash parses ` as command substitution.
env_needs_quotes() {
  local v="$1"
  [[ "$v" == *"="* ]] && return 0
  [[ "$v" == *[[:space:]]* ]] && return 0
  [[ "$v" == *"#"* ]] && return 0
  [[ "$v" == *"\""* ]] && return 0
  [[ "$v" == *"'"* ]] && return 0
  # RHS is a glob pattern: *\\* matches a single backslash
  [[ "$v" == *\\* ]] && return 0
  [[ "$v" == *'$'* ]] && return 0
  [[ "$v" == *'`'* ]] && return 0
  return 1
}

set_env() {
  # set_env KEY VALUE — upsert in ENV_FILE (preserves other lines)
  local key="$1" value="$2" tmp quoted
  [[ -n "$key" ]] || die "set_env: empty key"
  [[ -f "$ENV_FILE" ]] || touch "$ENV_FILE"

  if env_needs_quotes "$value"; then
    quoted="$(printf '%s' "$value" | sed 's/\\/\\\\/g; s/"/\\"/g')"
    value="\"${quoted}\""
  fi

  tmp="$(mktemp)"
  if grep -qE "^[[:space:]]*${key}=" "$ENV_FILE" 2>/dev/null; then
    awk -v k="$key" -v v="$value" '
      BEGIN { done=0 }
      {
        if ($0 ~ "^[[:space:]]*" k "=") {
          if (!done) { print k "=" v; done=1 }
          next
        }
        print
      }
      END { if (!done) print k "=" v }
    ' "$ENV_FILE" >"$tmp"
  else
    cat "$ENV_FILE" >"$tmp"
    # ensure trailing newline
    [[ -s "$tmp" ]] && [[ "$(tail -c1 "$tmp" | wc -l)" -eq 0 ]] && printf '\n' >>"$tmp"
    printf '%s=%s\n' "$key" "$value" >>"$tmp"
  fi
  mv "$tmp" "$ENV_FILE"
}

get_env() {
  # get_env KEY → stdout value (unquoted best-effort)
  local key="$1" line val
  [[ -f "$ENV_FILE" ]] || { printf ''; return 0; }
  line="$(grep -E "^[[:space:]]*${key}=" "$ENV_FILE" | tail -n1 || true)"
  [[ -n "$line" ]] || { printf ''; return 0; }
  val="${line#*=}"
  val="${val%$'\r'}"
  if [[ "$val" =~ ^\".*\"$ ]]; then
    val="${val:1:${#val}-2}"
    val="${val//\\\"/\"}"
  elif [[ "$val" =~ ^\'.*\'$ ]]; then
    val="${val:1:${#val}-2}"
  fi
  printf '%s' "$val"
}

ensure_env_from_sample() {
  if [[ ! -f "$ENV_FILE" ]]; then
    [[ -f "${REPO_ROOT}/.env.sample" ]] || die "Нет .env.sample"
    cp "${REPO_ROOT}/.env.sample" "$ENV_FILE"
    ok "Создан .env из .env.sample"
  else
    ok ".env уже есть — обновляю нужные ключи"
  fi
  backup_file "$ENV_FILE"
}

rand_hex() { openssl rand -hex "${1:-32}"; }

rand_alnum() {
  # URL-safe / .env-safe; avoid head|pipefail SIGPIPE
  openssl rand -hex 14
}

# Minimal URL-encode for postgres password in DATABASE_URL
urlencode() {
  local s="$1" i c out=""
  local LC_ALL=C
  for ((i = 0; i < ${#s}; i++)); do
    c="${s:i:1}"
    case "$c" in
      [a-zA-Z0-9.~_-]) out+="$c" ;;
      *) printf -v out '%s%%%02X' "$out" "'$c" ;;
    esac
  done
  printf '%s' "$out"
}

ensure_docker_network() {
  local name="${1:-$DEFAULT_NETWORK}"
  if docker network inspect "$name" >/dev/null 2>&1; then
    ok "Docker-сеть уже есть: $name"
  else
    docker network create --driver bridge "$name" >/dev/null
    ok "Создана Docker-сеть: $name"
  fi
}

detect_public_ip() {
  local ip=""
  ip="$(curl -4 -fsS --max-time 5 https://ifconfig.me 2>/dev/null || true)"
  [[ -n "$ip" ]] || ip="$(curl -4 -fsS --max-time 5 https://api.ipify.org 2>/dev/null || true)"
  printf '%s' "$ip"
}

check_dns() {
  local host="$1" expect_ip="${2-}"
  local resolved=""
  if command -v getent >/dev/null 2>&1; then
    resolved="$(getent ahostsv4 "$host" 2>/dev/null | awk '{print $1; exit}' || true)"
  fi
  if [[ -z "$resolved" ]] && command -v dig >/dev/null 2>&1; then
    resolved="$(dig +short A "$host" 2>/dev/null | head -n1 || true)"
  fi
  if [[ -z "$resolved" ]]; then
    warn "DNS для $host пока не резолвится (это нормально, если A-запись только что добавили)"
    return 1
  fi
  ok "DNS $host → $resolved"
  if [[ -n "$expect_ip" && "$resolved" != "$expect_ip" ]]; then
    warn "Ожидали IP $expect_ip, получили $resolved — проверьте A-запись"
    return 1
  fi
  return 0
}

compose_up() {
  header "Запуск docker compose"
  info "Тяну свежие образы (bot :latest и postgres)…"
  (cd "$REPO_ROOT" && "${COMPOSE_CMD[@]}" pull) || warn "docker compose pull завершился с ошибкой — пробую up с локальным кэшем"
  # --force-recreate обязателен: иначе смена .env (CABINET_ENABLED и т.д.) не попадёт в процесс
  (cd "$REPO_ROOT" && "${COMPOSE_CMD[@]}" up -d --force-recreate --remove-orphans)
  ok "Контейнеры подняты"
}

# Имя контейнера бота из compose (container_name или service).
bot_container_name() {
  local n
  n="$(docker ps --format '{{.Names}}' | grep -E 'remnawave-telegram-shop-bot|^bot$' | head -n1 || true)"
  if [[ -z "$n" ]]; then
    n="$(cd "$REPO_ROOT" && "${COMPOSE_CMD[@]}" ps -q bot 2>/dev/null | head -n1 | xargs -r docker inspect -f '{{.Name}}' 2>/dev/null | sed 's#^/##' || true)"
  fi
  printf '%s' "${n:-remnawave-telegram-shop-bot}"
}

# Образ бота минимальный (нет printenv/sh) — читаем Env через docker inspect.
container_env_get() {
  local cname="$1" key="$2"
  docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$cname" 2>/dev/null \
    | sed -n "s/^${key}=//p" | head -n1
}

bot_container_running() {
  local cname="${1:-$(bot_container_name)}"
  [[ "$(docker inspect -f '{{.State.Running}}' "$cname" 2>/dev/null || echo false)" == "true" ]]
}

# Поднять bot(+db) если контейнера нет / не Running / не отвечает healthcheck.
# force=1 — всегда --force-recreate (после смены .env).
ensure_bot_up() {
  local port="${1:-$(get_env HEALTH_CHECK_PORT)}"
  local force="${2:-0}"
  port="${port:-$DEFAULT_PORT}"
  local cname hc
  cname="$(bot_container_name)"
  hc="http://127.0.0.1:${port}/healthcheck"

  if [[ "$force" != "1" ]] && bot_container_running "$cname"; then
    if curl -fsS --max-time 3 "$hc" >/dev/null 2>&1; then
      return 0
    fi
    warn "Контейнер $cname Up, но $hc не отвечает — пересоздаю…"
    force=1
  fi

  if [[ "$force" == "1" ]]; then
    info "docker compose up -d --force-recreate (bot подтянет .env)…"
    (cd "$REPO_ROOT" && "${COMPOSE_CMD[@]}" up -d --force-recreate --no-deps bot) || \
      (cd "$REPO_ROOT" && "${COMPOSE_CMD[@]}" up -d --force-recreate bot) || return 1
  else
    info "Бот не запущен — поднимаю docker compose…"
    (cd "$REPO_ROOT" && "${COMPOSE_CMD[@]}" up -d) || return 1
  fi
  sleep 3
  cname="$(bot_container_name)"
  info "Жду $hc …"
  if wait_http "$hc" 45; then
    ok "healthcheck OK ($cname)"
    return 0
  fi
  err "healthcheck не ответил после up. Логи: docker logs $cname --tail 80"
  return 1
}

# После смены CABINET_* в .env контейнер мог остаться со старым env → 404 на /cabinet/*.
# Также: если бот просто не запущен (отказались от compose up) — поднимаем сами.
ensure_bot_has_cabinet_routes() {
  local port="${1:-$(get_env HEALTH_CHECK_PORT)}"
  port="${port:-$DEFAULT_PORT}"
  local host_cab cont_cab cname need_recreate=0
  host_cab="$(get_env CABINET_ENABLED)"
  [[ "$host_cab" == "true" ]] || return 0

  cname="$(bot_container_name)"
  if ! bot_container_running "$cname"; then
    warn "Контейнер бота не Running — без этого кабинет/smoke не заработают."
    ensure_bot_up "$port" 1 || return 1
    cname="$(bot_container_name)"
  fi

  cont_cab="$(container_env_get "$cname" CABINET_ENABLED)"
  info "CABINET_ENABLED: host=.env(${host_cab}) container=${cname}(${cont_cab:-<нет>})"

  if [[ "$cont_cab" != "true" ]]; then
    warn "В контейнере CABINET_ENABLED≠true — роуты /cabinet/* не смонтированы (будет 404)."
    need_recreate=1
  elif ! curl -fsS --max-time 3 "http://127.0.0.1:${port}/cabinet/api/healthz" >/dev/null 2>&1; then
    # Env уже true, но healthz мёртв: старый процесс / не подхватил кабинет / порт
    warn "CABINET_ENABLED=true в контейнере, но /cabinet/api/healthz не отвечает — force-recreate."
    need_recreate=1
  fi

  if [[ "$need_recreate" -eq 1 ]]; then
    info "Пересоздаю bot, чтобы подтянуть .env…"
    ensure_bot_up "$port" 1 || return 1
    cname="$(bot_container_name)"
    cont_cab="$(container_env_get "$cname" CABINET_ENABLED)"
    info "После recreate: container CABINET_ENABLED=${cont_cab:-<нет>}"
  fi

  if [[ "$cont_cab" != "true" ]]; then
    err "Контейнер всё ещё без CABINET_ENABLED=true. Проверьте env_file в docker-compose и .env"
    return 1
  fi

  # В логах при успехе: cabinet routes mounted (только свежий хвост)
  if docker logs --tail 200 "$cname" 2>&1 | grep -qi 'cabinet routes mounted'; then
    ok "В логах есть «cabinet routes mounted»"
  else
    warn "В логах нет «cabinet routes mounted» — смотрите старт: docker logs $cname 2>&1 | tail -80"
  fi

  info "Жду http://127.0.0.1:${port}/cabinet/api/healthz …"
  if wait_http "http://127.0.0.1:${port}/cabinet/api/healthz" 40; then
    ok "Кабинет отвечает на bot:${port}"
    return 0
  fi
  err "healthz кабинета всё ещё 404/недоступен"
  info "  docker inspect $cname --format '{{range .Config.Env}}{{println .}}{{end}}' | grep '^CABINET_'"
  info "  docker logs $cname 2>&1 | grep -i cabinet | tail -20"
  info "  Меню → 6) Управление ботом → 6) Логи / 4) Restart"
  return 1
}

wait_http() {
  local url="$1" tries="${2:-40}" i
  for ((i=1; i<=tries; i++)); do
    if curl -fsS --max-time 3 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  return 1
}

# Ports for public cabinet access via reverse-proxy (not the bot HEALTH_CHECK_PORT).
check_public_ports() {
  local bot_port="${1:-$DEFAULT_PORT}"
  header "Порты и firewall (кабинет)"
  info "Снаружи (интернет → VDS) должны быть доступны:"
  info "  • TCP 80  — HTTP / ACME / редирект на HTTPS"
  info "  • TCP 443 — HTTPS кабинета (nginx/caddy)"
  info "Порт бота ${bot_port} наружу открывать НЕ нужно — только 127.0.0.1:${bot_port} для proxy."
  warn "Скрипт сам не открывает порты в облачной панели (Security Group / Firewall VPS) — только подсказки."

  local p listening
  for p in 80 443; do
    listening="n"
    if command -v ss >/dev/null 2>&1; then
      ss -lnt "sport = :$p" 2>/dev/null | grep -q ":$p" && listening="y" || true
    elif command -v netstat >/dev/null 2>&1; then
      netstat -lnt 2>/dev/null | grep -qE ":${p}[[:space:]]" && listening="y" || true
    fi
    if [[ "$listening" == "y" ]]; then
      ok "На машине что-то слушает порт $p"
    else
      warn "Порт $p сейчас никто не слушает (поднимите nginx/caddy или проверьте compose)"
    fi
  done

  # Warn if bot port is bound on all interfaces (0.0.0.0) — insecure
  if command -v ss >/dev/null 2>&1; then
    if ss -lnt "sport = :${bot_port}" 2>/dev/null | grep -qE '0\.0\.0\.0:'"${bot_port}"'|:::'"${bot_port}"; then
      warn "Порт бота ${bot_port} слушает 0.0.0.0/:: — лучше только 127.0.0.1 (как в docker-compose)"
    fi
  fi

  if command -v ufw >/dev/null 2>&1; then
    local ufw_status
    ufw_status="$(sudo ufw status 2>/dev/null | head -n1 || true)"
    info "ufw: ${ufw_status:-не удалось прочитать (нужен sudo)}"
    if echo "$ufw_status" | grep -qi 'active'; then
      local open_ufw
      ask_yn open_ufw "Открыть в ufw TCP 80 и 443 (sudo ufw allow)?" N
      if [[ "$open_ufw" == "y" ]]; then
        sudo ufw allow 80/tcp || warn "ufw allow 80 не удалось"
        sudo ufw allow 443/tcp || warn "ufw allow 443 не удалось"
        sudo ufw status | head -n 20 || true
        ok "ufw: запрошены allow 80/443"
      fi
    else
      warn "ufw не active — если firewall выключен, смотрите firewall облака (Hetzner/AWS/…)"
    fi
  elif command -v firewall-cmd >/dev/null 2>&1; then
    info "Обнаружен firewalld"
    local open_fwd
    ask_yn open_fwd "Открыть в firewalld сервисы http/https?" N
    if [[ "$open_fwd" == "y" ]]; then
      sudo firewall-cmd --permanent --add-service=http || warn "firewalld http fail"
      sudo firewall-cmd --permanent --add-service=https || warn "firewalld https fail"
      sudo firewall-cmd --reload || true
      ok "firewalld: http/https"
    fi
  else
    warn "ufw/firewalld не найдены. Проверьте вручную:"
    warn "  • локальный firewall (iptables/nftables)"
    warn "  • Security Group / Network ACL у провайдера VPS — inbound 80, 443"
  fi

  info "Быстрая проверка с другой машины / телефона: https://<домен>/cabinet/"
  info "Если DNS ок, а сайт не открывается — почти всегда закрыты 80/443 у провайдера."
}

# Выбор тега образа бота: latest или конкретная версия (4.12.1).
# Результат: полный image ref в переменной, имя которой передано первым аргументом.
pick_bot_image() {
  local __out_var="$1"
  local image_repo tag_choice tag cur_image cur_tag
  image_repo="$DEFAULT_IMAGE_REPO"

  cur_image="$(grep -E '^\s*image:' "$COMPOSE_FILE" | head -n1 | awk '{print $2}' || true)"
  if [[ "$cur_image" == *:* ]]; then
    image_repo="${cur_image%:*}"
    cur_tag="${cur_image##*:}"
  else
    cur_tag="latest"
  fi
  [[ -n "$image_repo" ]] || image_repo="$DEFAULT_IMAGE_REPO"

  if [[ -n "${MEOWS_IMAGE_TAG:-}" ]]; then
    tag="$MEOWS_IMAGE_TAG"
    tag="${tag#v}"
    info "MEOWS_IMAGE_TAG=${tag} → ${image_repo}:${tag}"
    printf -v "$__out_var" '%s' "${image_repo}:${tag}"
    return 0
  fi

  header "Версия Docker-образа бота"
  info "Реестр: ${image_repo}"
  info "  1) latest — всегда самый свежий образ из ghcr.io"
  info "  2) Указать версию вручную (например 4.12.1 или 3.4.0)"
  if [[ -n "$cur_tag" && "$cur_tag" != "latest" ]]; then
    info "Сейчас в compose: ${cur_tag}"
  fi
  ask tag_choice "Вариант" "1"
  case "$tag_choice" in
    2)
      ask tag "Версия / тег образа" "${cur_tag}"
      tag="${tag#v}"
      tag="${tag#"${image_repo}:"}"
      [[ -n "$tag" ]] || die "Тег/версия не может быть пустым"
      if [[ ! "$tag" =~ ^[A-Za-z0-9._-]+$ ]]; then
        die "Некорректный тег: используйте латиницу/цифры/._- (пример: 4.12.1)"
      fi
      ;;
    *)
      tag="latest"
      ;;
  esac

  ok "Образ бота: ${image_repo}:${tag}"
  printf -v "$__out_var" '%s' "${image_repo}:${tag}"
}

# ===================== BOT =====================

setup_bot() {
  header "Установка Telegram bot (Meows Shop)"
  require_repo
  ensure_env_from_sample

  local token admin_id pg_user pg_pass pg_db port image
  local remna_topo remna_url remna_token remna_mode
  local sales_mode

  token="$(get_env TELEGRAM_TOKEN)"
  if [[ -z "$token" || "$token" == "token" ]]; then
    ask_secret token "TELEGRAM_TOKEN (от BotFather)"
  else
    local reuse
    ask_yn reuse "TELEGRAM_TOKEN уже задан — оставить?" Y
    [[ "$reuse" == "y" ]] || ask_secret token "TELEGRAM_TOKEN (от BotFather)"
  fi
  [[ -n "$token" ]] || die "TELEGRAM_TOKEN обязателен"

  admin_id="$(get_env ADMIN_TELEGRAM_ID)"
  if [[ -z "$admin_id" || "$admin_id" == "123123123" ]]; then
    ask admin_id "ADMIN_TELEGRAM_ID (ваш числовой Telegram ID)"
  else
    ask admin_id "ADMIN_TELEGRAM_ID" "$admin_id"
  fi
  [[ "$admin_id" =~ ^-?[0-9]+$ ]] || die "ADMIN_TELEGRAM_ID должен быть числом"

  local port_default sales_default
  port_default="$(get_env HEALTH_CHECK_PORT)"
  port_default="${port_default:-$DEFAULT_PORT}"
  ask port "HEALTH_CHECK_PORT" "$port_default"
  port="${port:-$DEFAULT_PORT}"

  pick_bot_image image

  sales_default="$(get_env SALES_MODE)"
  sales_default="${sales_default:-tariffs}"
  ask sales_mode "SALES_MODE (classic|tariffs)" "$sales_default"
  sales_mode="${sales_mode:-tariffs}"
  [[ "$sales_mode" == "classic" || "$sales_mode" == "tariffs" ]] || die "SALES_MODE: classic или tariffs"

  header "PostgreSQL"
  local pg_volume_exists="n"
  if docker volume inspect remnawave-telegram-shop-db-data >/dev/null 2>&1; then
    pg_volume_exists="y"
    warn "Docker volume remnawave-telegram-shop-db-data уже есть."
    warn "Смена POSTGRES_PASSWORD в .env НЕ меняет пароль внутри уже инициализированной БД."
  fi

  pg_user="$(get_env POSTGRES_USER)"
  pg_user="${pg_user:-postgres}"
  ask pg_user "POSTGRES_USER" "$pg_user"
  pg_pass="$(get_env POSTGRES_PASSWORD)"
  if [[ "$pg_volume_exists" == "y" ]]; then
    # Never auto-rotate: image init runs only once; .env drift breaks db login.
    if [[ -z "$pg_pass" ]]; then
      die "Volume БД уже есть, но POSTGRES_PASSWORD пуст в .env — укажите пароль, с которым volume создавался"
    fi
    info "Оставляю текущий POSTGRES_PASSWORD (volume уже инициализирован)"
    local change_pass
    ask_yn change_pass "Всё равно изменить пароль в .env? (обычно НЕТ — сломает доступ к БД)" N
    if [[ "$change_pass" == "y" ]]; then
      ask_secret pg_pass "POSTGRES_PASSWORD (должен совпадать с уже инициализированным volume)"
      [[ -n "$pg_pass" ]] || die "POSTGRES_PASSWORD обязателен"
      warn "Пароль в volume сам не меняется — только значение в .env / DATABASE_URL."
    fi
  else
    if [[ -z "$pg_pass" || "$pg_pass" == "postgres" ]]; then
      pg_pass="$(rand_alnum)"
      info "Сгенерирован POSTGRES_PASSWORD"
    fi
    local keep_pass
    ask_yn keep_pass "Оставить сгенерированный/текущий пароль Postgres?" Y
    [[ "$keep_pass" == "y" ]] || ask_secret pg_pass "POSTGRES_PASSWORD"
    [[ -n "$pg_pass" ]] || die "POSTGRES_PASSWORD обязателен"
  fi
  pg_db="$(get_env POSTGRES_DB)"
  ask pg_db "POSTGRES_DB" "${pg_db:-postgres}"

  header "Связь с Remnawave"
  info "1) Панель на ЭТОЙ же машине, общая docker-сеть ${DEFAULT_NETWORK}"
  info "2) Панель на другой машине / доступна по URL (remote)"
  ask remna_topo "Вариант" "1"
  remna_token="$(get_env REMNAWAVE_TOKEN)"
  if [[ "$remna_topo" == "2" ]]; then
    remna_mode="remote"
    ask remna_url "REMNAWAVE_URL (доступный ИЗ контейнера бота)" "$(get_env REMNAWAVE_URL)"
    [[ -n "$remna_url" ]] || die "REMNAWAVE_URL обязателен"
  else
    remna_mode="local"
    remna_url="$(get_env REMNAWAVE_URL)"
    if [[ -z "$remna_url" || "$remna_url" == "token" || "$remna_url" == http://remnawave:3000 ]]; then
      remna_url="http://remnawave:3000"
    fi
    ask remna_url "REMNAWAVE_URL (обычно http://remnawave:3000)" "$remna_url"
  fi
  if [[ -z "$remna_token" || "$remna_token" == "token" ]]; then
    ask_secret remna_token "REMNAWAVE_TOKEN (API token панели)"
  else
    local reuse_t
    ask_yn reuse_t "REMNAWAVE_TOKEN уже задан — оставить?" Y
    [[ "$reuse_t" == "y" ]] || ask_secret remna_token "REMNAWAVE_TOKEN"
  fi
  [[ -n "$remna_token" ]] || die "REMNAWAVE_TOKEN обязателен"

  ensure_docker_network "$DEFAULT_NETWORK"

  # Patch compose image if needed
  if grep -qE 'image:\s*.*remnawave-telegram-shop' "$COMPOSE_FILE"; then
    backup_file "$COMPOSE_FILE"
    sed -i -E "s|image:\s*.*remnawave-telegram-shop[^[:space:]]*|image: ${image}|g" "$COMPOSE_FILE" || true
  fi

  # Ensure loopback port publish matches HEALTH_CHECK_PORT (first host-bind = bot in stock compose)
  if ! grep -qE "127\.0\.0\.1:${port}:${port}" "$COMPOSE_FILE"; then
    if grep -qE '127\.0\.0\.1:[0-9]+:[0-9]+' "$COMPOSE_FILE"; then
      backup_file "$COMPOSE_FILE"
      # GNU sed: only the first 127.0.0.1:HOST:CONTAINER (bot), keep db :5432
      sed -i -E "0,/127\\.0\\.0\\.1:[0-9]+:[0-9]+/s|127\\.0\\.0\\.1:[0-9]+:[0-9]+|127.0.0.1:${port}:${port}|" "$COMPOSE_FILE"
      ok "ports: 127.0.0.1:${port}:${port}"
    else
      warn "В docker-compose.yaml нет публикации 127.0.0.1:${port}:${port} — проверьте ports вручную"
    fi
  fi

  set_env TELEGRAM_TOKEN "$token"
  set_env ADMIN_TELEGRAM_ID "$admin_id"
  set_env HEALTH_CHECK_PORT "$port"
  set_env SALES_MODE "$sales_mode"
  set_env POSTGRES_USER "$pg_user"
  set_env POSTGRES_PASSWORD "$pg_pass"
  set_env POSTGRES_DB "$pg_db"
  set_env DATABASE_URL "postgres://${pg_user}:$(urlencode "$pg_pass")@db:5432/${pg_db}?sslmode=disable"
  set_env REMNAWAVE_URL "$remna_url"
  set_env REMNAWAVE_MODE "$remna_mode"
  set_env REMNAWAVE_TOKEN "$remna_token"

  # Payments / cabinet / URL placeholders — only on fresh sample or explicit confirm
  local cab_now reset_payments wipe_urls
  cab_now="$(get_env CABINET_ENABLED)"
  if [[ "$cab_now" == "true" ]]; then
    warn "CABINET_ENABLED=true — пункт «только бот» НЕ выключает кабинет."
  else
    set_env CABINET_ENABLED "false"
  fi

  ask_yn reset_payments "Выставить минимальные платежи (только Stars; YooKassa/Crypto/Platega выкл)?" N
  if [[ "$reset_payments" == "y" ]]; then
    set_env TELEGRAM_STARS_ENABLED "true"
    set_env YOOKASA_ENABLED "false"
    set_env CRYPTO_PAY_ENABLED "false"
    set_env PLATEGA_ENABLED "false"
    set_env TRIBUTE_WEBHOOK_URL ""
    set_env MOYNALOG_ENABLED "false"
    set_env SUPPORT_BOT_API "false"
    info "Платежи: только Telegram Stars. Эквайринг можно включить позже в .env."
  else
    info "Платёжные флаги в .env не трогаю."
  fi

  local sampleish="n"
  local st_url
  st_url="$(get_env SERVER_STATUS_URL)"
  if [[ "$st_url" == "https://example.com/status" || -z "$st_url" ]]; then
    sampleish="y"
  fi
  ask_yn wipe_urls "Очистить placeholder URL кнопок (support/channel/…) если это sample?" "${sampleish}"
  if [[ "$wipe_urls" == "y" ]]; then
    set_env SERVER_STATUS_URL ""
    set_env SUPPORT_URL ""
    set_env FEEDBACK_URL ""
    set_env CHANNEL_URL ""
    set_env TOS_URL ""
    set_env VIDEO_GUIDE_URL ""
    set_env SERVER_SELECTION_URL ""
    set_env PUBLIC_OFFER_URL ""
    set_env PRIVACY_POLICY_URL ""
    set_env TERMS_OF_SERVICE_URL ""
  fi

  local do_up
  ask_yn do_up "Запустить docker compose сейчас?" Y
  if [[ "$do_up" == "y" ]]; then
    compose_up
    local hc="http://127.0.0.1:${port}/healthcheck"
    info "Жду healthcheck: $hc"
    if wait_http "$hc" 45; then
      ok "healthcheck OK"
      curl -fsS "$hc" | head -c 400 || true
      printf '\n'
    else
      warn "healthcheck ещё не ответил. Смотрите: docker compose logs -f bot"
      warn "Частая причина: Remnawave API недоступен по REMNAWAVE_URL из сети ${DEFAULT_NETWORK}"
    fi
  else
    warn "Compose не запускали — контейнер бота может быть остановлен или со старым .env."
    warn "Кабинет (п.2/3), proxy и smoke сами попробуют поднять bot, если он не отвечает."
  fi

  ok "Бот настроен. Напишите боту /start в Telegram."
  BOT_PORT="$port"
}

# ===================== CABINET =====================

print_nginx_cabinet_block() {
  local domain="$1" port="$2" cert_full="$3" cert_key="$4"
  cat <<EOF
# --- Meows Shop Web Cabinet (generated by meows-shop-setup.sh) ---
server {
    listen 80;
    server_name ${domain};
    location /.well-known/acme-challenge/ {
        root /var/www/html;
        allow all;
    }
    location / {
        return 301 https://\$host\$request_uri;
    }
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name ${domain};

    ssl_certificate     ${cert_full};
    ssl_certificate_key ${cert_key};

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    client_max_body_size 2m;

    # Публичный лендинг: https://${domain}/  (бот отдаёт SPA на GET /).
    location = / {
        proxy_pass http://127.0.0.1:${port};
        proxy_http_version 1.1;
        proxy_connect_timeout 10s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        proxy_set_header Host              \$host;
        proxy_set_header X-Real-IP         \$remote_addr;
        proxy_set_header X-Forwarded-For   \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-Host  \$host;
        proxy_set_header X-Forwarded-Port  \$server_port;
    }

    # Старый адрес лендинга: https://${domain}/landing.
    # Бот отдаёт его сам (mux.Handle("/landing") в internal/cabinet/http/router.go),
    # поэтому проксируем как есть — без редиректа на /cabinet/.
    location /status {
        proxy_pass http://127.0.0.1:${port};
        proxy_http_version 1.1;
        proxy_connect_timeout 10s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        proxy_set_header Host              \$host;
        proxy_set_header X-Real-IP         \$remote_addr;
        proxy_set_header X-Forwarded-For   \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-Host  \$host;
        proxy_set_header X-Forwarded-Port  \$server_port;
    }

    location /landing {
        proxy_pass http://127.0.0.1:${port};
        proxy_http_version 1.1;
        proxy_connect_timeout 10s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        proxy_set_header Host              \$host;
        proxy_set_header X-Real-IP         \$remote_addr;
        proxy_set_header X-Forwarded-For   \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-Host  \$host;
        proxy_set_header X-Forwarded-Port  \$server_port;
    }

    location /cabinet/ {
        proxy_pass http://127.0.0.1:${port};
        proxy_http_version 1.1;
        proxy_connect_timeout 10s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        proxy_set_header Host              \$host;
        proxy_set_header X-Real-IP         \$remote_addr;
        proxy_set_header X-Forwarded-For   \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-Host  \$host;
        proxy_set_header X-Forwarded-Port  \$server_port;
    }

    location /cabinet/api/ {
        # Загрузка картинки для рассылки в web-админке: с общим лимитом 2m
        # PNG-скриншоты отбивались 413 ещё на nginx, не доходя до бота.
        client_max_body_size 4m;

        proxy_pass http://127.0.0.1:${port};
        proxy_http_version 1.1;
        proxy_connect_timeout 10s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        proxy_set_header Host              \$host;
        proxy_set_header X-Real-IP         \$remote_addr;
        proxy_set_header X-Forwarded-For   \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-Host  \$host;
        proxy_set_header X-Forwarded-Port  \$server_port;
    }

    # Остальное (на всякий случай) тоже на бота — но корень уже редиректится выше.
    location / {
        return 302 /cabinet/;
    }
}
# --- end Meows Shop Web Cabinet ---
EOF
}

# Проверка кабинета локально и через nginx (--resolve, без hairpin-проблем).
verify_cabinet_http() {
  local domain="$1" port="$2"
  local cab_en local_hc pub_hc root_code
  cab_en="$(get_env CABINET_ENABLED)"
  info "CABINET_ENABLED=${cab_en:-<пусто>}"
  if [[ "$cab_en" != "true" ]]; then
    err "CABINET_ENABLED не true — бот отдаст 404 на /cabinet/. Включите кабинет (пункт 3) и перезапустите bot."
    return 1
  fi

  local_hc="$(curl -fsS --max-time 8 "http://127.0.0.1:${port}/cabinet/api/healthz" 2>/dev/null || true)"
  if [[ -n "$local_hc" ]]; then
    ok "Локально бот: /cabinet/api/healthz OK"
  else
    warn "Локальный healthz кабинета недоступен — поднимаю/recreate bot с актуальным .env…"
    if ! curl -fsS --max-time 3 "http://127.0.0.1:${port}/healthcheck" >/dev/null 2>&1; then
      warn "Даже /healthcheck молчит — бот скорее всего не запущен (на шаге установки ответили «n»?)."
    fi
    ensure_bot_has_cabinet_routes "$port" || return 1
    local_hc="$(curl -fsS --max-time 8 "http://127.0.0.1:${port}/cabinet/api/healthz" 2>/dev/null || true)"
    [[ -n "$local_hc" ]] || return 1
  fi

  # Через nginx на этой же машине (не через публичный routing/hairpin)
  root_code="$(curl -k -sS -o /dev/null -w '%{http_code}' --max-time 8 \
    --resolve "${domain}:443:127.0.0.1" "https://${domain}/" 2>/dev/null || echo 000)"
  info "https://${domain}/ → HTTP ${root_code} (ожидаем 200, лендинг)"
  if [[ "$root_code" != "200" ]]; then
    warn "Корень не отдал лендинг. Проверьте location = / в nginx: он должен проксировать на бота, а не редиректить на /cabinet/."
  fi

  pub_hc="$(curl -k -fsS --max-time 8 --resolve "${domain}:443:127.0.0.1" \
    "https://${domain}/cabinet/api/healthz" 2>/dev/null || true)"
  if [[ -n "$pub_hc" ]]; then
    ok "Через nginx (127.0.0.1 + Host/SNI): /cabinet/api/healthz OK"
    return 0
  fi
  warn "Через nginx healthz не ответил. Проверьте server_name и proxy_pass в nginx.conf"
  info "  grep -n \"${domain}\\|proxy_pass\\|location\" /opt/remnawave/nginx.conf | head -40"
  return 1
}

print_caddy_cabinet_block() {
  local domain="$1" port="$2"
  cat <<EOF
# Meows Shop Web Cabinet
${domain} {
  encode zstd gzip
  redir / /cabinet/ 302
  reverse_proxy 127.0.0.1:${port}
}
EOF
}

setup_proxy() {
  header "Reverse proxy / SSL для кабинета"
  require_repo

  local domain port proxy_kind nginx_conf nginx_container cert_mode
  local cert_full cert_key snippet_path do_patch

  domain="$(get_env CABINET_PUBLIC_URL)"
  domain="${domain#https://}"
  domain="${domain#http://}"
  domain="${domain%%/*}"
  ask domain "Домен кабинета" "$domain"
  [[ -n "$domain" ]] || die "Домен обязателен"

  port="$(get_env HEALTH_CHECK_PORT)"
  ask port "Порт бота на хосте (HEALTH_CHECK_PORT)" "${port:-$DEFAULT_PORT}"

  info "Где крутится reverse-proxy?"
  info "  1) Nginx в Docker (compose Remnawave или отдельный) — укажете путь к conf"
  info "  2) Caddy в Docker — укажете путь к Caddyfile"
  info "  3) Только сохранить snippet на диск (вставите сами)"
  warn "Snippet проксирует на 127.0.0.1:${port} — это работает, если nginx/caddy в network_mode: host"
  warn "(типичный Remnawave nginx). В bridge-сети 127.0.0.1 — сам proxy-контейнер, не хост."
  ask_choice_123 proxy_kind "Вариант reverse-proxy" "1"

  local pub_ip
  pub_ip="$(detect_public_ip)"
  [[ -n "$pub_ip" ]] && info "Публичный IP этой машины (примерно): $pub_ip"
  check_dns "$domain" "$pub_ip" || true

  case "$proxy_kind" in
    2)
      local caddyfile
      ask caddyfile "Полный путь к Caddyfile на хосте"
      [[ -n "$caddyfile" ]] || die "Нужен путь"
      snippet_path="${REPO_ROOT}/.setup-backups/cabinet.caddy.${domain}.snippet"
      print_caddy_cabinet_block "$domain" "$port" >"$snippet_path"
      chmod 600 "$snippet_path" 2>/dev/null || true
      ok "Snippet: $snippet_path"
      ask_yn do_patch "Дописать блок в Caddyfile?" Y
      if [[ "$do_patch" == "y" ]]; then
        [[ -f "$caddyfile" ]] || die "Файл не найден: $caddyfile"
        backup_file "$caddyfile"
        if grep -qF "$domain" "$caddyfile" 2>/dev/null; then
          warn "Домен уже упоминается в Caddyfile — не дублирую. Проверьте вручную."
        else
          printf '\n' >>"$caddyfile"
          cat "$snippet_path" >>"$caddyfile"
          ok "Блок добавлен в $caddyfile"
        fi
        pick_nginx_container nginx_container "caddy"
        if [[ -n "$nginx_container" ]]; then
          docker exec "$nginx_container" caddy reload --config /etc/caddy/Caddyfile 2>/dev/null \
            || docker exec "$nginx_container" caddy reload 2>/dev/null \
            || warn "Не удалось reload Caddy — перезапустите контейнер вручную"
        fi
      fi
      ;;
    3)
      snippet_path="${REPO_ROOT}/.setup-backups/cabinet.nginx.${domain}.snippet"
      cert_full="/etc/letsencrypt/live/${domain}/fullchain.pem"
      cert_key="/etc/letsencrypt/live/${domain}/privkey.pem"
      print_nginx_cabinet_block "$domain" "$port" "$cert_full" "$cert_key" >"$snippet_path"
      chmod 600 "$snippet_path" 2>/dev/null || true
      ok "Snippet сохранён: $snippet_path"
      info "Вставьте его в nginx.conf и смонтируйте сертификаты в контейнер nginx."
      ;;
    1)
      info "Пример пути conf Remnawave: /opt/remnawave/nginx.conf"
      local nginx_conf_default=""
      for nginx_conf_default in /opt/remnawave/nginx.conf ./nginx.conf; do
        [[ -f "$nginx_conf_default" ]] && break
        nginx_conf_default=""
      done
      ask nginx_conf "Полный путь к nginx.conf на хосте" "${nginx_conf_default}"
      [[ -n "$nginx_conf" && -f "$nginx_conf" ]] || die "Файл nginx conf не найден: ${nginx_conf:-<пусто>}"
      # Имя контейнера ≠ имя файла conf
      pick_nginx_container nginx_container "remnawave-nginx"

      # Пути как в типичном Remnawave nginx (host paths = paths inside container при mount /etc/letsencrypt)
      cert_full="/etc/letsencrypt/live/${domain}/fullchain.pem"
      cert_key="/etc/letsencrypt/live/${domain}/privkey.pem"

      header "SSL для ${domain}"
      if [[ -f "$cert_full" && -f "$cert_key" ]]; then
        ok "Сертификат уже есть — используем его"
      else
        info "Сертификата нет — скрипт выпустит Let's Encrypt автоматически."
        info "При необходимости кратко остановит ${nginx_container}, затем запустит снова (панель вернётся)."
        ask_yn _do_cert "Выпустить сертификат сейчас?" Y
        if [[ "$_do_cert" == "y" ]]; then
          issue_letsencrypt_cert "$domain" "$nginx_container" || die "Без SSL публичный кабинет не заработает — исправьте DNS/80 и повторите пункт 4"
        else
          die "Без сертификата HTTPS-кабинет не поднять. Запустите пункт 4 позже."
        fi
      fi
      # Важно: серт на хосте ≠ серт внутри nginx. Нужен volume /etc/letsencrypt:/etc/letsencrypt:ro
      ensure_nginx_sees_letsencrypt "$nginx_container" "$domain" \
        || die "Сначала почините mount LE в compose Remnawave (см. выше), иначе nginx в restart-loop"

      snippet_path="${REPO_ROOT}/.setup-backups/cabinet.nginx.${domain}.snippet"
      print_nginx_cabinet_block "$domain" "$port" "$cert_full" "$cert_key" >"$snippet_path"
      chmod 600 "$snippet_path" 2>/dev/null || true
      ok "Snippet: $snippet_path"

      # Server-блоки — по умолчанию да; если блок уже был (до появления cert) — обновляем пути/маркерный блок
      ask_yn do_patch "Прописать/обновить server-блоки кабинета в ${nginx_conf}?" Y
      if [[ "$do_patch" == "y" ]]; then
        upsert_cabinet_nginx_block "$nginx_conf" "$snippet_path" "$domain" "$cert_full" "$cert_key"
        info "Перезапускаю ${nginx_container} с актуальным conf + SSL…"
        docker restart "$nginx_container" >/dev/null 2>&1 || docker start "$nginx_container" >/dev/null 2>&1 || true
        reload_nginx_container "$nginx_container" || true
      else
        warn "Без server-блока в nginx публичный URL кабинета не откроется."
      fi

      sleep 1
      verify_cabinet_http "$domain" "$port" || true
      info "Откройте в браузере именно: https://${domain}/cabinet/"
      info "(корень / без редиректа у бота = текст «404 page not found»)"
      ;;
  esac

  info "DNS A ${domain} → этот сервер; бот только 127.0.0.1:${port}; панель Remnawave на своём server_name не трогаем."
  check_public_ports "$port"
}

optional_google() {
  local want id secret redirect public
  ask_yn want "Настроить Google OAuth? (можно пропустить)" N
  [[ "$want" == "y" ]] || { ok "Google пропущен"; return 0; }
  public="$(get_env CABINET_PUBLIC_URL)"
  redirect="${public%/}/cabinet/api/auth/google/callback"
  ask id "CABINET_GOOGLE_CLIENT_ID"
  ask_secret secret "CABINET_GOOGLE_CLIENT_SECRET"
  ask redirect "CABINET_GOOGLE_REDIRECT_URL" "$redirect"
  set_env CABINET_GOOGLE_CLIENT_ID "$id"
  set_env CABINET_GOOGLE_CLIENT_SECRET "$secret"
  set_env CABINET_GOOGLE_REDIRECT_URL "$redirect"
  info "В Google Cloud Console → Credentials → Authorized redirect URIs добавьте ровно:"
  info "  $redirect"
  info "Authorized JavaScript origins: ${public}"
}

optional_logo() {
  local want logo_host logo_cont brand
  ask_yn want "Настроить логотип / название кабинета?" N
  [[ "$want" == "y" ]] || return 0
  ask brand "CABINET_BRAND_NAME" "$(get_env CABINET_BRAND_NAME)"
  brand="${brand:-Cabinet}"
  set_env CABINET_BRAND_NAME "$brand"
  ask logo_host "Путь к файлу логотипа на хосте (пусто = пропуск файла)"
  if [[ -n "$logo_host" ]]; then
    [[ -f "$logo_host" ]] || die "Файл не найден: $logo_host"
    local dest_name
    dest_name="$(basename "$logo_host")"
    [[ -n "$dest_name" ]] || dest_name="brand-logo.png"
    logo_cont="/${dest_name}"
    ask logo_cont "Путь файла ВНУТРИ контейнера бота" "$logo_cont"
    [[ "$logo_cont" == /* ]] || logo_cont="/${logo_cont}"
    dest_name="$(basename "$logo_cont")"
    local logo_dest="${REPO_ROOT}/${dest_name}"
    if [[ "$logo_host" == "$logo_dest" || "$logo_host" -ef "$logo_dest" ]]; then
      ok "Логотип уже на месте: $logo_dest"
    else
      cp -f "$logo_host" "$logo_dest"
    fi
    set_env CABINET_BRAND_LOGO_FILE "/${dest_name}"
    if ! grep -qF "./${dest_name}:/${dest_name}" "$COMPOSE_FILE" 2>/dev/null; then
      warn "Добавьте volume в docker-compose.yaml сервиса bot:"
      warn "  - ./${dest_name}:/${dest_name}:ro"
      local patch_compose
      ask_yn patch_compose "Попробовать добавить volume рядом с ./translations?" Y
      if [[ "$patch_compose" == "y" ]]; then
        backup_file "$COMPOSE_FILE"
        if grep -q 'vpn_cat.png' "$COMPOSE_FILE"; then
          sed -i "s|./vpn_cat.png:/vpn_cat.png:ro|./${dest_name}:/${dest_name}:ro|" "$COMPOSE_FILE" || true
        elif grep -q './translations:/translations' "$COMPOSE_FILE"; then
          sed -i "/\.\/translations:\/translations/a\\      - ./${dest_name}:/${dest_name}:ro" "$COMPOSE_FILE" || true
        else
          warn "Не удалось безопасно пропатчить compose — добавьте volume вручную"
        fi
      fi
    fi
    ok "Логотип: ${REPO_ROOT}/${dest_name} → /${dest_name}"
  fi
}

optional_translations_mount() {
  local want t_host i18n_host
  ask_yn want "Монтировать локальную папку translations (не из образа)?" N
  [[ "$want" == "y" ]] || return 0
  ask t_host "Путь к translations на хосте" "${REPO_ROOT}/translations"
  [[ -d "$t_host" ]] || die "Каталог не найден: $t_host"
  if [[ "$t_host" != "${REPO_ROOT}/translations" ]]; then
    warn "Compose по умолчанию ждёт ./translations — используйте путь относительно REPO_ROOT или поправьте volumes"
  fi
  if ! grep -q './translations:/translations' "$COMPOSE_FILE"; then
    warn "В compose нет './translations:/translations'."
    warn "Добавьте вручную в volumes сервиса bot (не в top-level volumes:):"
    warn "  - ./translations:/translations"
  else
    ok "translations volume уже есть: ./translations:/translations"
  fi

  if [[ -d "${REPO_ROOT}/web/cabinet/src/i18n" ]]; then
    local want_i18n
    ask_yn want_i18n "Также монтировать web/cabinet/src/i18n для UI-строк кабинета?" Y
    if [[ "$want_i18n" == "y" ]]; then
      if ! grep -q 'cabinet/i18n' "$COMPOSE_FILE"; then
        backup_file "$COMPOSE_FILE"
        sed -i '/\.\/translations:\/translations/a\      - ./web/cabinet/src/i18n:/translations/cabinet/i18n:ro' "$COMPOSE_FILE" || true
      fi
      ok "i18n volume добавлен"
    fi
  fi
}

setup_cabinet() {
  header "Настройка Web-кабинета"
  require_repo
  [[ -f "$ENV_FILE" ]] || die "Сначала нужен .env (пункт установки бота)"
  backup_file "$ENV_FILE"

  local domain public origins jwt port bot_user
  local smtp_host smtp_port smtp_user smtp_pass smtp_tls mail_from
  local pub_ip snippet_path

  port="$(get_env HEALTH_CHECK_PORT)"
  port="${port:-$DEFAULT_PORT}"

  ask domain "Домен кабинета (без https://)" ""
  [[ -n "$domain" ]] || die "Домен обязателен"
  public="https://${domain}"
  ask public "CABINET_PUBLIC_URL" "$public"
  origins="$public"
  ask origins "CABINET_ALLOWED_ORIGINS" "$origins"

  jwt="$(get_env CABINET_JWT_SECRET)"
  if [[ -z "$jwt" || ${#jwt} -lt 32 ]]; then
    jwt="$(rand_hex 32)"
    info "Сгенерирован CABINET_JWT_SECRET"
  else
    local reuse
    ask_yn reuse "CABINET_JWT_SECRET уже есть — оставить?" Y
    [[ "$reuse" == "y" ]] || jwt="$(rand_hex 32)"
  fi

  # Telegram widget (default)
  set_env CABINET_TELEGRAM_WEB_AUTH_MODE "widget"
  bot_user="$(get_env CABINET_TELEGRAM_LOGIN_BOT_USERNAME)"
  ask bot_user "Username бота для Login Widget (без @)" "$bot_user"
  [[ -n "$bot_user" ]] || die "CABINET_TELEGRAM_LOGIN_BOT_USERNAME обязателен для widget"
  bot_user="${bot_user#@}"

  header "SMTP (email регистрация / сброс пароля)"
  ask smtp_host "CABINET_SMTP_HOST" "$(get_env CABINET_SMTP_HOST)"
  [[ -n "$smtp_host" ]] || die "SMTP обязателен для рабочего email-auth"
  ask smtp_port "CABINET_SMTP_PORT" "$(get_env CABINET_SMTP_PORT)"
  smtp_port="${smtp_port:-465}"
  ask smtp_user "CABINET_SMTP_USER" "$(get_env CABINET_SMTP_USER)"
  smtp_pass="$(get_env CABINET_SMTP_PASSWORD)"
  if [[ -z "$smtp_pass" ]]; then
    ask_secret smtp_pass "CABINET_SMTP_PASSWORD"
  else
    local reuse_p
    ask_yn reuse_p "SMTP пароль уже задан — оставить?" Y
    [[ "$reuse_p" == "y" ]] || ask_secret smtp_pass "CABINET_SMTP_PASSWORD"
  fi
  ask smtp_tls "CABINET_SMTP_TLS (true/false)" "$(get_env CABINET_SMTP_TLS)"
  smtp_tls="${smtp_tls:-true}"
  ask mail_from "CABINET_MAIL_FROM" "$(get_env CABINET_MAIL_FROM)"
  [[ -n "$mail_from" ]] || mail_from="VPN <noreply@${domain}>"

  set_env CABINET_ENABLED "true"
  set_env CABINET_PUBLIC_URL "$public"
  set_env CABINET_ALLOWED_ORIGINS "$origins"
  set_env CABINET_JWT_SECRET "$jwt"
  set_env CABINET_COOKIE_DOMAIN ""
  set_env CABINET_ACCESS_TTL_MINUTES "15"
  set_env CABINET_REFRESH_TTL_DAYS "30"
  set_env CABINET_TELEGRAM_WEB_AUTH_MODE "widget"
  set_env CABINET_TELEGRAM_LOGIN_BOT_USERNAME "$bot_user"
  set_env CABINET_SMTP_HOST "$smtp_host"
  set_env CABINET_SMTP_PORT "$smtp_port"
  set_env CABINET_SMTP_USER "$smtp_user"
  set_env CABINET_SMTP_PASSWORD "$smtp_pass"
  set_env CABINET_SMTP_TLS "$smtp_tls"
  set_env CABINET_MAIL_FROM "$mail_from"

  # Clear OIDC placeholders that could confuse; widget mode ignores them if empty
  # Keep existing Google if any — optional step may set

  pub_ip="$(detect_public_ip)"
  check_dns "$domain" "$pub_ip" || true

  optional_google
  optional_logo
  optional_translations_mount

  header "Reverse-proxy / SSL (обязательно для публичного кабинета)"
  info "Без HTTPS на домене кабинет с интернета не откроется:"
  info "  cookies Secure, OAuth/Telegram widget, браузерный вход."
  info "Локально API уже будет на 127.0.0.1:${port} — снаружи нужен nginx/caddy + 80/443."
  info "Внутри мастера можно выбрать «только snippet на диск», если conf правите сами."
  setup_proxy

  info "Ручные шаги BotFather (обязательно):"
  info "  1) /setdomain → ${domain}"
  info "  2) Для Login Widget домен должен совпадать с кабинетом"
  info "Google (если настраивали) — redirect URI в консоли Google."

  # После CABINET_* в .env контейнер обязан быть пересоздан. Если бот уже мёртв —
  # не спрашиваем, поднимаем сами (типичный кейс: на шаге бота ответили «n» на compose up).
  local cname do_up
  cname="$(bot_container_name)"
  if ! bot_container_running "$cname" || \
     ! curl -fsS --max-time 3 "http://127.0.0.1:${port}/healthcheck" >/dev/null 2>&1; then
    warn "Бот не Running / не отвечает на :${port} — поднимаю с актуальным CABINET_* …"
    compose_up
    ensure_bot_has_cabinet_routes "$port" || warn "Кабинет в bot ещё не поднят — см. логи / меню 6"
  else
    ask_yn do_up "Пересоздать bot (подхватить CABINET_* из .env)?" Y
    if [[ "$do_up" == "y" ]]; then
      compose_up
      ensure_bot_has_cabinet_routes "$port" || warn "Кабинет в bot ещё не поднят — см. логи / меню 6"
    else
      warn "Без recreate контейнер может остаться со старым CABINET_ENABLED → 404 на /cabinet/*"
      ensure_bot_has_cabinet_routes "$port" || true
    fi
  fi
  if curl -fsS --max-time 5 "http://127.0.0.1:${port}/cabinet/api/auth/bootstrap" >/dev/null 2>&1; then
    info "bootstrap:"
    curl -fsS "http://127.0.0.1:${port}/cabinet/api/auth/bootstrap" | head -c 800 || true
    printf '\n'
  fi

  ok "Кабинет сконфигурирован: ${public}/cabinet/"
  info "Публичный URL заработает после DNS + proxy/SSL + открытых портов 80/443."
}

# ===================== SMOKE =====================

run_smoke() {
  header "Smoke-проверки"
  require_repo
  local port public
  port="$(get_env HEALTH_CHECK_PORT)"
  port="${port:-$DEFAULT_PORT}"
  public="$(get_env CABINET_PUBLIC_URL)"

  local local_urls=(
    "http://127.0.0.1:${port}/healthcheck"
  )
  local cab
  cab="$(get_env CABINET_ENABLED)"
  if [[ "$cab" == "true" ]]; then
    local_urls+=(
      "http://127.0.0.1:${port}/cabinet/api/healthz"
      "http://127.0.0.1:${port}/cabinet/api/auth/bootstrap"
    )
  fi

  local u fail_local=0 fail_public=0
  info "Локальные проверки (бот на loopback):"
  for u in "${local_urls[@]}"; do
    if curl -fsS --max-time 10 "$u" >/dev/null 2>&1; then
      ok "$u"
    else
      err "FAIL $u"
      fail_local=1
    fi
  done

  if [[ "$cab" == "true" && -n "$public" ]]; then
    info "Публичные проверки (нужны DNS + reverse-proxy + SSL + порты 80/443):"
    for u in "${public%/}/cabinet/api/healthz" "${public%/}/cabinet/api/auth/bootstrap"; do
      if curl -fsS --max-time 10 "$u" >/dev/null 2>&1; then
        ok "$u"
      else
        warn "FAIL $u"
        fail_public=1
      fi
    done
    if [[ "$fail_public" -ne 0 ]]; then
      warn "Публичный кабинет недоступен — чаще всего: нет/кривой nginx server-block, SSL, DNS или закрыты 80/443."
      warn "Локально кабинет при этом может быть OK. Меню → 4) reverse-proxy / SSL."
    fi
  fi

  if [[ "$cab" == "true" && "$fail_local" -eq 0 ]]; then
    info "Фрагмент bootstrap (local):"
    curl -fsS --max-time 10 "http://127.0.0.1:${port}/cabinet/api/auth/bootstrap" 2>/dev/null | head -c 1000 || true
    printf '\n'
  fi

  if [[ "$fail_local" -ne 0 ]]; then
    err "Локальный smoke провален — бот на 127.0.0.1:${port} не отвечает"
    printf '\n'
    warn "Что сделать:"
    info "  1) Меню → 6) Управление ботом → 1) Статус — контейнер Up?"
    info "  2) Там же → 2) Запустить или 4) Restart (подхватит .env)"
    info "  3) Логи: меню 6 → 6) Логи  (или: docker logs remnawave-telegram-shop-bot --tail 80)"
    info "  4) Проверьте HEALTH_CHECK_PORT в .env (сейчас ждём :${port})"
    if [[ "$cab" == "true" ]]; then
      info "  5) Кабинет: меню 6 → 7) Проверка env / кабинет"
      info "     или меню 3) Только Web-кабинет / 4) reverse-proxy"
    fi
    return 1
  fi
  if [[ "$fail_public" -ne 0 ]]; then
    warn "Локальный smoke OK, публичный — нет (см. выше)."
    info "Дальше: меню → 4) reverse-proxy / SSL; DNS A-запись; порты 80/443."
    return 0
  fi
  ok "Smoke OK"
}

# ===================== BOT CONTROL =====================

bot_compose() {
  # bot_compose <compose args...>
  (cd "$REPO_ROOT" && "${COMPOSE_CMD[@]}" "$@")
}

show_bot_status() {
  local cname port st cab_h cab_c
  require_repo
  cname="$(bot_container_name)"
  port="$(get_env HEALTH_CHECK_PORT)"
  port="${port:-$DEFAULT_PORT}"
  header "Статус бота"
  info "Проект: $REPO_ROOT"
  info "Контейнер: $cname"
  st="$(docker inspect -f '{{.State.Status}} (healthy={{if .State.Health}}{{.State.Health.Status}}{{else}}n/a{{end}})' "$cname" 2>/dev/null || echo "не найден")"
  info "Docker: $st"
  cab_h="$(get_env CABINET_ENABLED)"
  cab_c="$(container_env_get "$cname" CABINET_ENABLED)"
  info "CABINET_ENABLED host=${cab_h:-<нет>} container=${cab_c:-<нет/не запущен>}"
  if curl -fsS --max-time 3 "http://127.0.0.1:${port}/healthcheck" >/dev/null 2>&1; then
    ok "healthcheck :${port} OK"
  else
    warn "healthcheck :${port} недоступен"
  fi
  if [[ "$cab_h" == "true" ]] || [[ "$cab_c" == "true" ]]; then
    if curl -fsS --max-time 3 "http://127.0.0.1:${port}/cabinet/api/healthz" >/dev/null 2>&1; then
      ok "cabinet healthz OK"
    else
      warn "cabinet healthz 404/недоступен (нужен recreate после смены .env?)"
    fi
  fi
  printf '\n'
  (cd "$REPO_ROOT" && "${COMPOSE_CMD[@]}" ps) || true
}

bot_start() {
  require_repo
  header "Запуск bot (+ db)"
  bot_compose up -d
  ok "docker compose up -d"
  sleep 2
  show_bot_status
}

bot_stop() {
  require_repo
  header "Остановка"
  info "1) Только bot (db оставить)"
  info "2) bot + db (compose stop)"
  local m=""
  while [[ ! "$m" =~ ^[12]$ ]]; do
    ask m "Вариант" "1"
    [[ "$m" =~ ^[12]$ ]] || warn "Введите 1 или 2"
  done
  case "$m" in
    2)
      bot_compose stop
      ok "Остановлены сервисы compose (bot/db)"
      ;;
    *)
      bot_compose stop bot 2>/dev/null || docker stop "$(bot_container_name)" >/dev/null 2>&1 || true
      ok "Остановлен bot"
      ;;
  esac
  show_bot_status
}

# Restart = stop + up --force-recreate: иначе compose может просто
# стартовать старый контейнер и НЕ подхватить новый .env.
bot_restart() {
  require_repo
  header "Restart bot (с перечитыванием .env)"
  info "compose stop bot → up -d --force-recreate (БД/volume не трогаем)"
  bot_compose stop bot 2>/dev/null || true
  bot_compose up -d --force-recreate --no-deps bot || bot_compose up -d --force-recreate bot
  ok "bot перезапущен с актуальным .env"
  sleep 3
  local port
  port="$(get_env HEALTH_CHECK_PORT)"
  port="${port:-$DEFAULT_PORT}"
  if [[ "$(get_env CABINET_ENABLED)" == "true" ]]; then
    ensure_bot_has_cabinet_routes "$port" || true
  fi
  show_bot_status
}

bot_pull_recreate() {
  require_repo
  header "Pull образа + recreate"
  local image
  pick_bot_image image
  if grep -qE 'image:\s*.*remnawave-telegram-shop' "$COMPOSE_FILE"; then
    backup_file "$COMPOSE_FILE"
    sed -i -E "s|image:\s*.*remnawave-telegram-shop[^[:space:]]*|image: ${image}|g" "$COMPOSE_FILE" || true
    ok "compose image → $image"
  fi
  bot_compose pull bot || bot_compose pull
  bot_compose up -d --force-recreate --no-deps bot || bot_compose up -d --force-recreate bot
  ok "Образ обновлён и bot пересоздан"
  sleep 3
  show_bot_status
}

bot_logs() {
  require_repo
  local cname n
  cname="$(bot_container_name)"
  header "Логи $cname"
  info "1) Последние 100 строк"
  info "2) Следить (tail -f), Ctrl+C — выход"
  ask n "Вариант" "1"
  case "$n" in
    2)
      info "Ctrl+C чтобы вернуться в меню"
      docker logs -f --tail 100 "$cname" || true
      ;;
    *)
      docker logs --tail 100 "$cname" 2>&1 || true
      ;;
  esac
}

bot_env_check() {
  require_repo
  local cname port key val
  cname="$(bot_container_name)"
  port="$(get_env HEALTH_CHECK_PORT)"
  port="${port:-$DEFAULT_PORT}"
  header "Проверка env / кабинет"
  info "Важные переменные контейнера (через docker inspect; секреты скрыты):"
  if ! docker inspect "$cname" >/dev/null 2>&1; then
    warn "Контейнер $cname не найден"
    return 1
  fi
  for key in CABINET_ENABLED CABINET_PUBLIC_URL HEALTH_CHECK_PORT REMNAWAVE_MODE SALES_MODE TELEGRAM_STARS_ENABLED YOOKASA_ENABLED; do
    val="$(container_env_get "$cname" "$key")"
    info "  ${key}=${val:-<нет>}"
  done
  val="$(container_env_get "$cname" CABINET_JWT_SECRET)"
  if [[ -n "$val" ]]; then
    info "  CABINET_JWT_SECRET=*** (${#val} символов)"
  else
    info "  CABINET_JWT_SECRET=<нет>"
  fi
  printf '\n'
  if [[ "$(get_env CABINET_ENABLED)" == "true" ]]; then
    ensure_bot_has_cabinet_routes "$port" || true
  else
    info "CABINET_ENABLED в .env не true — проверка cabinet healthz пропущена"
    if curl -fsS --max-time 5 "http://127.0.0.1:${port}/healthcheck" >/dev/null 2>&1; then
      ok "healthcheck OK"
    else
      warn "healthcheck недоступен"
    fi
  fi
}

manage_bot_menu() {
  require_repo
  while true; do
    printf '\n'
    printf '%s%s%s\n' "${C_GREEN}" "УПРАВЛЕНИЕ БОТОМ" "${C_RESET}"
    meta "Каталог: $REPO_ROOT"
    printf '\n'
    menu "1) Статус (ps / health / CABINET_ENABLED)"
    menu "2) Запустить (compose up -d)"
    menu "3) Остановить"
    menu "4) Restart (stop + up, подхватывает .env)"
    printf '\n'
    menu "5) Pull образа + recreate"
    menu "6) Логи"
    menu "7) Проверка env / кабинет"
    printf '\n'
    menu "0) Назад в главное меню"
    printf '\n'
    local m=""
    while [[ ! "$m" =~ ^[0-7]$ ]]; do
      ask m "Выберите действие (0-7)"
      [[ "$m" =~ ^[0-7]$ ]] || warn "Введите число от 0 до 7"
    done
    case "$m" in
      0) return 0 ;;
      1) show_bot_status ;;
      2) bot_start ;;
      3) bot_stop ;;
      4) bot_restart ;;
      5) bot_pull_recreate ;;
      6) bot_logs ;;
      7) bot_env_check ;;
    esac
  done
}

# ===================== MENU =====================

show_menu() {
  printf '\n'
  printf '%s%s%s\n' "${C_GREEN}" "MEOWS TELEGRAM SHOP — SETUP" "${C_RESET}"
  meta "Версия: ${SCRIPT_VERSION}"
  meta "Документация: documentation/cabinet/SETUP-GUIDE-RU.md"
  printf '\n'
  menu "1) Установить Telegram bot — ТОЛЬКО бот"
  menu "2) Установить Telegram bot + Web-кабинет"
  menu "3) Только Web-кабинет (бот уже установлен)"
  printf '\n'
  menu "4) Только reverse-proxy / SSL для кабинета"
  menu "5) Smoke-проверки"
  menu "6) Управление ботом"
  printf '\n'
  menu "0) Выход"
  printf '\n'
  printf '%s- Повторный запуск: %s%s%s\n' \
    "${C_YELLOW}" "${C_CYAN}" "cd /opt/remnawave-telegram-shop && ./scripts/meows-shop-setup.sh" "${C_RESET}"
  printf '\n'
}

run_choice() {
  local choice="$1"
  case "$choice" in
    1) setup_bot ;;
    2) setup_bot; setup_cabinet ;;
    3) setup_cabinet ;;
    4) setup_proxy ;;
    5) run_smoke; return $? ;;
    6) manage_bot_menu ;;
    0|q|Q) ok "Выход"; exit 0 ;;
    *) warn "Неизвестный пункт: $choice"; return 1 ;;
  esac
  return 0
}

main() {
  preflight
  # При one-liner сразу готовим репо (клон), чтобы меню работало из чистой VDS
  if ! resolve_repo_root; then
    info "Локального клона нет — будет git clone (compose + translations)."
    info "Образ бота: ${DEFAULT_IMAGE_REPO}:<latest|версия> (выбор при установке бота)."
    ensure_repo
  else
    init_paths
    cd "$REPO_ROOT"
    ok "Проект: $REPO_ROOT"
  fi

  if script_is_piped; then
    info "Запуск через curl/pipe. Повторно удобнее:"
    info "  cd ${REPO_ROOT} && ./scripts/meows-shop-setup.sh"
  fi

  local choice="${MEOWS_CHOICE:-}"
  if [[ -n "$choice" ]]; then
    info "MEOWS_CHOICE=${choice} — пропускаю меню"
    run_choice "$choice"
    exit $?
  fi

  while true; do
    show_menu
    choice=""
    while [[ ! "$choice" =~ ^[0-6]$ ]]; do
      ask choice "Выберите действие (0-6)"
      [[ "$choice" =~ ^[0-6]$ ]] || warn "Введите число от 0 до 6"
    done
    run_choice "$choice" || true
    # Подменю «Управление ботом» само возвращает в главное — не спрашиваем повторно
    if [[ "$choice" == "6" ]]; then
      continue
    fi
    printf '\n'
    ask_yn _cont "Вернуться в меню?" Y
    [[ "${_cont}" == "y" ]] || break
  done
}

main "$@"

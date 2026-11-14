#!/usr/bin/env bash
# Meows ← Bedolaga: интерактивная миграция клиентов, тарифов, рефералов и истории оплат (Linux + Docker).
#
# Запуск из клона репозитория:
#   ./scripts/meows-bedolaga-migrate.sh
#
# Под капотом: движок tools/migrate-bedolaga собирается и запускается в контейнере (Go на сервере не нужен).
# Переменные: MEOWS_MIGRATE_BIN (готовый linux-бинарь), MEOWS_NETWORK, MEOWS_GO_IMAGE.
#
set -euo pipefail

SCRIPT_VERSION="2.0.0"

if [[ -t 1 ]]; then
  C_RESET=$'\033[0m'
  C_GREEN=$'\033[1;32m'
  C_YELLOW=$'\033[1;33m'
  C_RED=$'\033[1;31m'
  C_CYAN=$'\033[1;36m'
  C_GRAY=$'\033[90m'
else
  C_RESET= C_GREEN= C_YELLOW= C_RED= C_CYAN= C_GRAY=
fi

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
  printf '\n%s%s%s\n' "${C_CYAN}" "$*" "${C_RESET}"
  printf '%s%s%s\n\n' "${C_GRAY}" "────────────────────────────────────────" "${C_RESET}"
}

need_cmd() { command -v "$1" >/dev/null 2>&1 || die "Нужна команда: $1"; }

ask() {
  local __var="$1" __prompt="$2" __default="${3-}" __reply="" __pfx
  __pfx="$(prompt_prefix "${__prompt}:")"
  if [[ -n "$__default" ]]; then
    if [[ -t 0 ]]; then read -e -i "$__default" -r -p "${__pfx} " __reply || true
    else read -r -p "${__pfx} " __reply || true; fi
    __reply="${__reply:-$__default}"
  else
    if [[ -t 0 ]]; then read -e -r -p "${__pfx} " __reply || true
    else read -r -p "${__pfx} " __reply || true; fi
  fi
  printf -v "$__var" '%s' "${__reply-}"
}

ask_yn() {
  local __var="$1" __prompt="$2" __def="${3:-N}" __reply="" __hint __pfx
  if [[ "$__def" =~ ^[Yy]$ ]]; then __hint="Y/n"; else __hint="y/N"; fi
  __pfx="$(prompt_prefix "${__prompt} (${__hint}):")"
  if [[ -t 0 ]]; then read -e -r -p "${__pfx} " __reply || true
  else read -r -p "${__pfx} " __reply || true; fi
  __reply="${__reply:-$__def}"
  case "$__reply" in
    [Yy]|[Yy][Ee][Ss]) printf -v "$__var" 'y' ;;
    *) printf -v "$__var" 'n' ;;
  esac
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "$REPO_ROOT"

# Движок мигратора запускается в контейнере в docker-сети бота: там сразу видны
# БД бота (db:5432), панель Remnawave и временный Postgres. Go на сервере не нужен.
NETWORK="${MEOWS_NETWORK:-remnawave-network}"
GO_IMAGE="${MEOWS_GO_IMAGE:-golang:1.27.4-alpine}"
TEMP_PG_NAME="${MEOWS_BEDOLAGA_PG_NAME:-meows-bedolaga-restore}"
TEMP_PG_PASS="${MEOWS_BEDOLAGA_PG_PASS:-migrator}"
TEMP_PG_DB="${MEOWS_BEDOLAGA_PG_DB:-bedolaga_restore}"
BIN_DIR="${REPO_ROOT}/.migrate-bin"
ENGINE_BIN="${BIN_DIR}/migrate-bedolaga"
CONFIG_PATH="${MEOWS_MIGRATE_CONFIG:-${REPO_ROOT}/migrate.yaml}"
REPORT_DIR="${MEOWS_MIGRATE_OUT:-${REPO_ROOT}/migrate-out}"
SHOP_DB_CONTAINER="${MEOWS_SHOP_DB_CONTAINER:-remnawave-telegram-shop-db}"
SOURCE_DSN=""
TARGET_DSN=""
RW_URL=""
RW_TOKEN=""
RW_MODE="local"
ENGINE_READY="n"

load_env_defaults() {
  local envf="${REPO_ROOT}/.env"
  [[ -f "$envf" ]] || return 0
  # shellcheck disable=SC1090
  set -a; source "$envf" 2>/dev/null || true; set +a
  TARGET_DSN="${DATABASE_URL:-}"
  RW_URL="${REMNAWAVE_URL:-${REMNAWAVE_BASE_URL:-}}"
  RW_TOKEN="${REMNAWAVE_TOKEN:-${REMNAWAVE_API_TOKEN:-}}"
  RW_MODE="${REMNAWAVE_MODE:-local}"
}

yaml_quote() {
  # Quote scalar for YAML so passwords with #/: don't break parsing.
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  printf '"%s"' "$s"
}

# Путь внутри контейнера движка: репозиторий смонтирован в /src.
container_path() {
  local p="$1"
  case "$p" in
    "$REPO_ROOT"/*) printf '/src/%s' "${p#"$REPO_ROOT"/}" ;;
    *) die "Путь должен лежать внутри ${REPO_ROOT}: $p" ;;
  esac
}

# Контейнер движка не видит 127.0.0.1 хоста — подменяем на имя контейнера БД бота.
net_dsn() {
  local d="$1"
  d="${d//@127.0.0.1:5432/@${SHOP_DB_CONTAINER}:5432}"
  d="${d//@localhost:5432/@${SHOP_DB_CONTAINER}:5432}"
  printf '%s' "$d"
}

write_config() {
  local rep
  rep="$(container_path "$REPORT_DIR")"
  cat >"$CONFIG_PATH" <<EOF
source:
  database_url: $(yaml_quote "${SOURCE_DSN}")
target:
  database_url: $(yaml_quote "$(net_dsn "${TARGET_DSN}")")
remnawave:
  base_url: $(yaml_quote "${RW_URL}")
  token: $(yaml_quote "${RW_TOKEN}")
  mode: $(yaml_quote "${RW_MODE}")
tariffs:
  mode: import_from_bedolaga
  mapping: {}
balance:
  enabled: true
  policy: user_mapped_tariff
  fallback_policy: cheapest_imported_1m
  apply_to_remnawave: true
customers:
  on_conflict: prefer_bedolaga
  set_legal_accepted: true
  skip_deleted: true
  skip_blocked: true
  import_cabinet: true
referrals:
  import_graph: true
history:
  import_payments: true
reporting:
  dir: ${rep}
EOF
  chmod 600 "$CONFIG_PATH" 2>/dev/null || true
  ok "Конфиг записан: ${CONFIG_PATH}"
}

ensure_network() {
  docker network inspect "$NETWORK" >/dev/null 2>&1 || docker network create "$NETWORK" >/dev/null
}

# Собирает движок один раз за запуск (кэш модулей Go — в docker volume).
ensure_engine() {
  [[ "$ENGINE_READY" == "y" ]] && return 0
  need_cmd docker
  ensure_network
  mkdir -p "$BIN_DIR"
  if [[ -n "${MEOWS_MIGRATE_BIN:-}" ]]; then
    [[ -x "$MEOWS_MIGRATE_BIN" ]] || die "MEOWS_MIGRATE_BIN не исполняемый файл: $MEOWS_MIGRATE_BIN"
    cp "$MEOWS_MIGRATE_BIN" "$ENGINE_BIN"
  else
    info "Собираю движок миграции (первый раз — 1–3 минуты, нужен интернет)..."
    docker run --rm \
      -v "${REPO_ROOT}:/src" -v meows-migrate-gocache:/go \
      -w /src -e CGO_ENABLED=0 -e GOFLAGS=-buildvcs=false \
      "$GO_IMAGE" go build -trimpath -o /src/.migrate-bin/migrate-bedolaga ./tools/migrate-bedolaga \
      || die "Не удалось собрать движок (проверьте интернет и доступ к Docker Hub)"
  fi
  ENGINE_READY="y"
  ok "Движок готов"
}

# engine_run [-v host:container ...] -- аргументы движка
engine_run() {
  local mounts=()
  while [[ "${1:-}" == "-v" ]]; do mounts+=(-v "$2"); shift 2; done
  ensure_engine
  docker run --rm --network "$NETWORK" \
    -v "${REPO_ROOT}:/src" ${mounts[@]+"${mounts[@]}"} -w /src \
    "$GO_IMAGE" /src/.migrate-bin/migrate-bedolaga "$@" </dev/null
}

run_engine() {
  local mode="$1" step="$2" flag=-dry-run
  [[ "$mode" == "apply" ]] && flag=-apply
  [[ -f "$CONFIG_PATH" ]] || die "Сначала пункт 1 (нет ${CONFIG_PATH})"
  engine_run -config "$(container_path "$CONFIG_PATH")" -step "$step" "$flag"
}

ensure_temp_pg() {
  need_cmd docker
  ensure_network
  if docker ps -a --format '{{.Names}}' | grep -qx "$TEMP_PG_NAME"; then
    info "Контейнер ${TEMP_PG_NAME} уже есть — стартуем"
    docker start "$TEMP_PG_NAME" >/dev/null || true
  else
    info "Поднимаю временный Postgres ${TEMP_PG_NAME}"
    docker run -d --name "$TEMP_PG_NAME" --network "$NETWORK" \
      -e POSTGRES_PASSWORD="$TEMP_PG_PASS" \
      -e POSTGRES_DB="$TEMP_PG_DB" \
      postgres:16 >/dev/null
  fi
  info "Жду готовности Postgres..."
  local i
  for i in $(seq 1 40); do
    if docker exec "$TEMP_PG_NAME" pg_isready -U postgres -d "$TEMP_PG_DB" >/dev/null 2>&1; then
      sleep 1
      ok "Postgres готов"
      SOURCE_DSN="postgres://postgres:${TEMP_PG_PASS}@${TEMP_PG_NAME}:5432/${TEMP_PG_DB}?sslmode=disable"
      return 0
    fi
    sleep 1
  done
  die "Postgres не поднялся за 40с"
}

# Бекап из админки Bedolaga («Создать бекап»): .tar.gz с database.json внутри.
load_admin_backup() {
  local f="$1" dir base
  [[ -f "$f" ]] || die "Файл не найден: $f"
  f="$(cd "$(dirname "$f")" && pwd)/$(basename "$f")"
  dir="$(dirname "$f")"; base="$(basename "$f")"
  ensure_temp_pg
  info "Разворачиваю бекап Bedolaga во временную базу (до минуты)..."
  engine_run -v "${dir}:/backup:ro" -load-backup "/backup/${base}" -load-dsn "$SOURCE_DSN" \
    || die "Не удалось развернуть бекап. Нужен файл backup_*.tar.gz из админки Bedolaga"
  ok "Бекап развёрнут"
}

restore_dump() {
  local dump_path="$1"
  [[ -f "$dump_path" ]] || die "Файл не найден: $dump_path"
  ensure_temp_pg
  info "Восстанавливаю дамп (может занять время)..."
  case "$dump_path" in
    *.gz)
      gunzip -c "$dump_path" | docker exec -i "$TEMP_PG_NAME" psql -U postgres -d "$TEMP_PG_DB" >/dev/null
      ;;
    *.sql)
      docker exec -i "$TEMP_PG_NAME" psql -U postgres -d "$TEMP_PG_DB" <"$dump_path" >/dev/null
      ;;
    *.dump|*.backup)
      docker exec -i "$TEMP_PG_NAME" pg_restore -U postgres -d "$TEMP_PG_DB" --no-owner --role=postgres <"$dump_path" \
        || warn "pg_restore вернул предупреждения (часто нормально)"
      ;;
    *)
      die "Неизвестный формат дампа (ожидаются .sql / .sql.gz / .dump)"
      ;;
  esac
  ok "Дамп восстановлен в ${TEMP_PG_NAME}"
}

cleanup_temp_pg() {
  if docker ps -a --format '{{.Names}}' | grep -qx "$TEMP_PG_NAME"; then
    ask_yn ans "Удалить временный контейнер ${TEMP_PG_NAME} (с копией данных Bedolaga)?" Y
    if [[ "$ans" == "y" ]]; then
      docker rm -f "$TEMP_PG_NAME" >/dev/null
      ok "Контейнер удалён"
    fi
  else
    meta "Временный Postgres не найден"
  fi
}

show_report_hint() {
  meta "Отчёты: ${REPORT_DIR}/"
  meta "  summary.json  customers.csv  tariffs.csv  balances.csv  problems.csv"
  if [[ -f "${REPORT_DIR}/summary.json" ]]; then
    info "summary:"
    cat "${REPORT_DIR}/summary.json" || true
    printf '\n'
  fi
}

# Копия БД бота перед записью — откат одной командой.
backup_shop_db() {
  local envf="${REPO_ROOT}/.env" user db out
  user="$( { grep -E '^POSTGRES_USER=' "$envf" 2>/dev/null || true; } | tail -n1 | cut -d= -f2- | tr -d '"\r')"
  db="$( { grep -E '^POSTGRES_DB=' "$envf" 2>/dev/null || true; } | tail -n1 | cut -d= -f2- | tr -d '"\r')"
  user="${user:-postgres}"; db="${db:-postgres}"
  mkdir -p "$REPORT_DIR"
  out="${REPORT_DIR}/shop-db-before-migrate-$(date +%Y%m%d-%H%M%S).sql.gz"
  if docker exec "$SHOP_DB_CONTAINER" pg_dump -U "$user" -d "$db" --no-owner 2>/dev/null | gzip >"$out" && [[ -s "$out" ]]; then
    chmod 600 "$out" 2>/dev/null || true
    ok "Копия БД бота перед миграцией: $out"
    meta "  Откат: gunzip -c $out | docker exec -i ${SHOP_DB_CONTAINER} psql -U ${user} -d ${db}"
  else
    rm -f "$out"
    warn "Не удалось сделать копию БД бота (контейнер ${SHOP_DB_CONTAINER} запущен?)"
    ask_yn cont "Продолжить без копии?" N
    [[ "$cont" == "y" ]] || die "Остановлено"
  fi
}

prepare_source() {
  header "Источник данных Bedolaga"
  menu "1) Бекап из админки Bedolaga (файл backup_*.tar.gz) — самый простой вариант"
  menu "2) pg_dump (.sql / .sql.gz / .dump)"
  menu "3) Уже есть DSN к Postgres со схемой Bedolaga (доступный из docker-сети ${NETWORK})"
  ask choice "Выбор" "1"
  case "$choice" in
    1)
      ask file "Путь к backup_*.tar.gz" ""
      load_admin_backup "$file"
      ;;
    2)
      ask dump "Путь к дампу" ""
      restore_dump "$dump"
      ;;
    3)
      ask SOURCE_DSN "Bedolaga DATABASE_URL" "${SOURCE_DSN}"
      ;;
    *) die "Неверный выбор" ;;
  esac
  [[ -n "$SOURCE_DSN" ]] || die "SOURCE_DSN пуст"

  header "БД нашего бота + Remnawave"
  compat_confirm
  ask TARGET_DSN "Наш DATABASE_URL" "${TARGET_DSN}"
  ask RW_URL "Remnawave URL (3.3.*-3.4.*)" "${RW_URL}"
  ask RW_TOKEN "Remnawave API token" "${RW_TOKEN}"
  ask RW_MODE "Remnawave mode (local/remote)" "${RW_MODE}"
  write_config
}

print_banner() {
  header "Meows ← Bedolaga migrate  v${SCRIPT_VERSION}"
  meta "Проект: ${REPO_ROOT}"
  meta "Нужны: Docker, интернет (один раз соберётся движок), запущенный бот Meows и панель Remnawave 3.3.*-3.4.*"
  warn "Перед apply остановите бота Bedolaga, чтобы в старую БД не падали новые оплаты."
  meta "Что переносится: тарифы, клиенты (Telegram и email/OAuth), срок подписки, баланс → дни, рефералы, история оплат."
  meta "Remnawave: только чтение + при необходимости удлинение срока; сквады, трафик и теги не меняются."
}

compat_confirm() {
  header "Совместимость Remnawave"
  info "Укажите URL/token панели Remnawave 3.3.*-3.4.*, с которой работает Meows."
  info "Источник Bedolaga может быть 3.x или 4.x — движок подстраивается под схему."
  ask_yn ans "Панель Remnawave для Meows — ветка 3.3.* или 3.4.* (не 2.8)?" Y
  if [[ "$ans" != "y" ]]; then
    warn "Бот 5.x умеет только Remnawave 3.x: в 3.0.0 у пользователя панели удалён uuid"
    warn "и убран GET /api/users/by-telegram-id/{id} — на панели 2.8 движок не найдёт клиентов."
    ask_yn ans2 "Всё равно продолжить настройку?" N
    [[ "$ans2" == "y" ]] || die "Остановлено: под Meows 5.x нужна панель Remnawave 3.3.*-3.4.*"
  fi
}

apply_step() {
  # apply_step <шаг> <вопрос>
  local step="$1" q="$2"
  [[ -f "$CONFIG_PATH" ]] || die "Сначала пункт 1 (нет ${CONFIG_PATH})"
  ask_yn ans "$q" N
  [[ "$ans" == "y" ]] || return 0
  run_engine apply "$step"
  show_report_hint
}

# Итог по шагам «всё сразу»: summary.json перезаписывается каждым шагом, поэтому берём копии.
print_totals() {
  info "Итог переноса:"
  local row step key label f n
  while IFS='|' read -r step key label; do
    f="${REPORT_DIR}/summary-${step}.json"
    [[ -f "$f" ]] || continue
    n="$(grep -o "\"${key}\": *[0-9]*" "$f" | grep -o '[0-9]*$' | head -n1 || true)"
    printf '  %s: %s\n' "$label" "${n:-?}"
  done <<'ROWS'
tariffs|tariffs_ok|Тарифов
customers|customers_ok|Клиентов
customers|customers_cabinet|  из них без Telegram (email / OAuth)
referrals|referrals_ok|Реферальных связей
history|payments_ok|Оплат в истории
ROWS
}

run_all() {
  [[ -f "$CONFIG_PATH" ]] || die "Сначала пункт 1 (нет ${CONFIG_PATH})"
  header "Пробный прогон (ничего не пишет)"
  run_engine dry-run all
  show_report_hint
  warn "Дальше перенос запишется в базу бота. Бот Bedolaga должен быть остановлен."
  ask_yn go "Переносить по-настоящему?" N
  [[ "$go" == "y" ]] || return 0
  backup_shop_db
  local st
  for st in tariffs customers balance referrals history; do
    header "Шаг: ${st}"
    run_engine apply "$st"
    cp "${REPORT_DIR}/summary.json" "${REPORT_DIR}/summary-${st}.json" 2>/dev/null || true
  done
  print_totals
  ok "Готово. Загляните в ${REPORT_DIR}/problems.csv — там клиенты, которым нужно внимание."
  cleanup_temp_pg
}

main_menu() {
  while true; do
    header "Меню"
    menu "1) Подготовить источник + записать migrate.yaml"
    menu "2) Перенести всё сразу (пробный прогон → копия БД → apply всех шагов)  ← рекомендуется"
    menu "3) Только пробный прогон (ничего не пишет)"
    menu "4) Apply по шагам: тарифы"
    menu "5) Apply по шагам: клиенты (Telegram + email/OAuth)"
    menu "6) Apply по шагам: срок/баланс (может удлинить срок в Remnawave)"
    menu "7) Apply по шагам: рефералы"
    menu "8) Apply по шагам: история оплат (клиенты не будут выглядеть «триалом»)"
    menu "9) Показать отчёты"
    menu "10) Удалить временный Postgres"
    menu "0) Выход"
    ask choice "Пункт" "1"
    case "$choice" in
      1) prepare_source ;;
      2) run_all ;;
      3) run_engine dry-run all; show_report_hint ;;
      4) apply_step tariffs "Применить импорт тарифов?" ;;
      5) apply_step customers "Применить импорт клиентов?" ;;
      6) warn "Этот шаг может УДЛИНИТЬ expireAt в Remnawave (сквады не трогает)."
         apply_step balance "Применить срок/баланс?" ;;
      7) apply_step referrals "Применить реферальный граф?" ;;
      8) apply_step history "Применить историю оплат?" ;;
      9) show_report_hint ;;
      10) cleanup_temp_pg ;;
      0) exit 0 ;;
      *) warn "Неизвестный пункт" ;;
    esac
  done
}

print_banner
load_env_defaults
main_menu

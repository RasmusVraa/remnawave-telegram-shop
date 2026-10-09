import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/** Читает значение cookie по имени. */
export function getCookie(name: string): string {
  const match = document.cookie.match(new RegExp(`(?:^|;\\s*)${name}=([^;]*)`))
  return match ? decodeURIComponent(match[1]) : ''
}

/**
 * Телефон или планшет по User-Agent.
 *
 * Нужен там, где решение зависит не от ширины экрана, а от того, есть ли у
 * системы то, чего нет на десктопе: настоящий share-шит, узкая клавиатура,
 * свой Telegram-клиент. Медиазапрос здесь не подходит — узкое окно на десктопе
 * не делает браузер мобильным.
 */
export function isMobileUserAgent(): boolean {
  if (typeof navigator === 'undefined') return false
  return /android|iphone|ipad|ipod|mobile/i.test(navigator.userAgent)
}

/** iPhone, iPad и iPod — любой браузер там WebKit. iPadOS 13+ представляется маком, поэтому одного userAgent мало. */
export function isIosWebKit(): boolean {
  if (typeof navigator === 'undefined') return false
  if (/iPad|iPhone|iPod/.test(navigator.userAgent)) return true
  return navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1
}

/** Генерирует UUID v4 для Idempotency-Key. */
export function newIdempotencyKey(): string {
  return crypto.randomUUID()
}

/** Маскирует email: user@example.com → u***@example.com */
export function maskEmail(email: string): string {
  const [local, domain] = email.split('@')
  if (!domain || local.length <= 1) return email
  return local[0] + '***@' + domain
}

/** Возвращает дни до даты ISO (отрицательное — уже прошло). */
export function daysUntil(iso: string): number {
  return Math.ceil((new Date(iso).getTime() - Date.now()) / 86_400_000)
}

/** Подпись над полосой трафика: «148.0 / 500 ГБ» или «Безлимит». */
export function formatTrafficUsageLabel(
  usedGb: number | null | undefined,
  limitGb: number | null | undefined,
  gigabytesLabel: string,
  unlimitedLabel: string,
): string {
  if (limitGb != null && limitGb > 0) {
    const used = Math.max(0, usedGb ?? 0).toFixed(1)
    return `${used} / ${limitGb} ${gigabytesLabel}`
  }
  return unlimitedLabel
}

/** Доля использованного трафика 0–100; null — безлимит или лимит не задан. */
export function trafficUsagePercent(
  usedGb: number | null | undefined,
  limitGb: number | null | undefined,
): number | null {
  if (limitGb == null || limitGb <= 0) return null
  return Math.min(100, Math.max(0, ((usedGb ?? 0) / limitGb) * 100))
}

/** Градиент заливки полосы трафика по порогам: >80% — оранжевый, >90% — красный. */
export function trafficBarFillClass(percent: number | null): string {
  if (percent != null && percent > 90) {
    return 'bg-gradient-to-r from-red-600 via-red-500 to-rose-600 dark:from-red-500 dark:via-red-400 dark:to-[#c70000]'
  }
  if (percent != null && percent > 70) {
    return 'bg-gradient-to-r from-amber-500 via-orange-500 to-orange-600 dark:from-amber-400 dark:via-orange-400 dark:to-amber-500'
  }
  return 'bg-gradient-to-r from-primary via-primary/90 to-primary/70'
}

/** Форматирует дату по локали. */
export function formatDate(iso: string, lang: string): string {
  return new Date(iso).toLocaleDateString(lang === 'ru' ? 'ru-RU' : 'en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

/** Формат DD.MM.YYYY HH:mm (локальное время браузера). */
export function formatDateTimeShort(iso: string): string {
  const d = new Date(iso)
  if (!Number.isFinite(d.getTime())) return '—'
  const dd = String(d.getDate()).padStart(2, '0')
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const yyyy = d.getFullYear()
  const hh = String(d.getHours()).padStart(2, '0')
  const min = String(d.getMinutes()).padStart(2, '0')
  return `${dd}.${mm}.${yyyy} ${hh}:${min}`
}

/**
 * Дата для таблиц истории: короткий год и время отдельной строкой.
 * `2026-08-28T22:32:00Z` → `{ date: '28.08.26', time: '22:32' }`.
 */
export function splitDateTimeShort(iso?: string): { date: string; time: string } | null {
  if (!iso) return null
  const d = new Date(iso)
  if (!Number.isFinite(d.getTime())) return null
  const dd = String(d.getDate()).padStart(2, '0')
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const yy = String(d.getFullYear() % 100).padStart(2, '0')
  const hh = String(d.getHours()).padStart(2, '0')
  const min = String(d.getMinutes()).padStart(2, '0')
  return { date: `${dd}.${mm}.${yy}`, time: `${hh}:${min}` }
}

/**
 * Форматирует цену в рублях.
 * Реализация — в `@/lib/format` (локаль берётся из i18n); здесь реэкспорт,
 * чтобы не переписывать существующие импорты из `@/lib/utils`.
 */
export { formatRub } from '@/lib/format'

/** initData из Telegram Mini App (пустая строка в обычном браузере). */
export function getTelegramInitData(): string {
  const raw = window.Telegram?.WebApp?.initData
  if (typeof raw === 'string' && raw.trim().length > 0) {
    return raw.trim()
  }
  // Fallback for first Mini App open when SDK object is not ready yet.
  const fromHash = readUrlParamFromHashOrSearch('tgWebAppData')
  return fromHash.trim()
}

/**
 * start_param из Mini App (как deep-link бота). Для рефералки обычно ref_<tg>.
 * Не подписан отдельно — доверяем только в связке с проверкой initData на бэкенде;
 * referral_code дублируется в POST явно.
 */
export function getTelegramMiniAppStartParam(): string {
  const u = window.Telegram?.WebApp?.initDataUnsafe
  const sp = u && typeof u === 'object' && u !== null && 'start_param' in u ? (u as { start_param?: unknown }).start_param : undefined
  if (typeof sp === 'string' && sp.trim().length > 0) {
    return sp.trim()
  }
  return readUrlParamFromHashOrSearch('tgWebAppStartParam').trim()
}

function readUrlParamFromHashOrSearch(name: string): string {
  if (typeof window === 'undefined') return ''
  const fromSearch = new URLSearchParams(window.location.search).get(name)
  if (fromSearch) return fromSearch

  const hash = window.location.hash.startsWith('#') ? window.location.hash.slice(1) : window.location.hash
  if (!hash) return ''
  const fromHash = new URLSearchParams(hash).get(name)
  return fromHash ?? ''
}

/**
 * Источник, из которого пришёл посетитель, из query-параметров.
 *
 * У регистрации одно поле «откуда пришёл», и через него идут обе программы:
 * `?ref=` — реферальная ссылка (telegram_id пригласившего), `?p=` —
 * партнёрская. Партнёрский код отправляем с префиксом `p_`, иначе бэкенд не
 * отличит его от реферального идентификатора.
 */
export function acquisitionCodeFromQuery(params: URLSearchParams): string {
  const ref = (params.get('ref') ?? '').trim()
  if (ref) return ref
  const partner = (params.get('p') ?? '').trim()
  return partner ? `p_${partner.replace(/^p_/i, '')}` : ''
}

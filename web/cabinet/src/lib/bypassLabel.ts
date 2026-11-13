import { i18n } from '@/i18n'

/** Название по умолчанию, которое сервер отдаёт, пока админ не задал своё (TRAFFIC_BYPASS_LABEL). */
const SERVER_DEFAULT_LABEL = 'обход'

/** Аббревиатуру («LTE», «VIP») не трогаем: у неё вторая буква тоже заглавная. */
function isAcronym(s: string): boolean {
  const second = s.charAt(1)
  return second !== '' && second !== second.toLowerCase()
}

function rawLabel(label?: string | null): string {
  const l = (label ?? '').trim()
  if (!l) return ''
  // Серверное «обход» по умолчанию в английском интерфейсе заменяем переводом.
  if (l.toLowerCase() === SERVER_DEFAULT_LABEL && !String(i18n.language ?? '').startsWith('ru')) return ''
  return l
}

/** Признак, что админ не задал своё название — для текстов, где есть готовая формулировка. */
export function isDefaultBypassLabel(label?: string | null): boolean {
  return (label ?? '').trim().toLowerCase() === SERVER_DEFAULT_LABEL
}

/**
 * Как называть серверы обхода в заголовке, на вкладке, в начале строки — с заглавной.
 * Сервер по умолчанию отдаёт «обход» со строчной, потому что в боте название стоит
 * внутри фразы; админ может задать любое написание.
 */
export function bypassLabel(label?: string | null): string {
  const l = rawLabel(label) || i18n.t('trafficSplit.bypassFallback')
  return l.charAt(0).toUpperCase() + l.slice(1)
}

/** То же название внутри фразы — со строчной: «Безлимит + 20 ГБ обход». */
export function bypassLabelInline(label?: string | null): string {
  const l = rawLabel(label) || i18n.t('trafficSplit.bypassFallback')
  if (isAcronym(l)) return l
  return l.charAt(0).toLowerCase() + l.slice(1)
}

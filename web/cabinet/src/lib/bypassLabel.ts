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

/**
 * Признак, что админ не задал своё название — для текстов, где есть готовая формулировка.
 * Пустое название — тоже «не задал»: иначе в текст подставилось бы «Обход» без склонения.
 */
export function isDefaultBypassLabel(label?: string | null): boolean {
  const l = (label ?? '').trim().toLowerCase()
  return l === '' || l === SERVER_DEFAULT_LABEL
}

/**
 * «+25 ГБ на серверах Обхода» для страницы оформления. Название от админа не
 * склоняется, поэтому слово берём в кавычки («Резерв»), аббревиатуру — как есть (LTE).
 */
export function bypassServersTitle(gb: string, label?: string | null): string {
  const l = rawLabel(label)
  if (isDefaultBypassLabel(l)) return i18n.t('checkout.bypassTitleDefault', { gb })
  return isAcronym(l)
    ? i18n.t('checkout.bypassTitleAcronym', { gb, label: l })
    : i18n.t('checkout.bypassTitleCustom', { gb, label: l })
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

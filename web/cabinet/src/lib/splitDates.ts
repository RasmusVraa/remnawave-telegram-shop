/*
 * Границы расчётного периода трафика — полночь UTC (1-е число). Показывать их в часовом поясе браузера нельзя:
 * западнее UTC «1 декабря 00:00» превращается в «30 ноября».
 */

const locale = (lang: string) => (lang === 'ru' ? 'ru-RU' : 'en-US')

/** «1 декабря» в UTC. */
export function formatUtcDayMonth(iso: string | null | undefined, lang: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (!Number.isFinite(d.getTime())) return ''
  return d.toLocaleDateString(locale(lang), { day: 'numeric', month: 'long', timeZone: 'UTC' })
}

/** Название месяца, который заканчивается в момент iso (reset_at — начало следующего). */
export function utcPeriodMonthName(resetIso: string | null | undefined, lang: string): string {
  if (!resetIso) return ''
  const d = new Date(new Date(resetIso).getTime() - 86_400_000)
  if (!Number.isFinite(d.getTime())) return ''
  return d.toLocaleDateString(locale(lang), { month: 'long', timeZone: 'UTC' })
}

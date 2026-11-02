import {
  AlertTriangle,
  Ban,
  Briefcase,
  CalendarClock,
  CreditCard,
  Dices,
  Gift,
  Handshake,
  Headset,
  House,
  Link2,
  Megaphone,
  MonitorSmartphone,
  Package,
  Plug,
  Receipt,
  ShoppingCart,
  Star,
  Ticket,
  User,
  UserPlus,
  Users,
  type LucideIcon,
} from 'lucide-react'

import type { NotificationDTO } from '@/lib/api'

/**
 * Категория карточки: подпись, значок и цвет. Берётся из вида уведомления —
 * админу заполнять нечего. Важное — своя категория, какого бы вида оно ни было.
 * tone — цвет подписи и значка, tint — подложка значка; обе пары и для
 * светлой темы, и для тёмной.
 */
export type NotificationCategory = { labelKey: string; icon: LucideIcon; tone: string; tint: string }

const IMPORTANT: NotificationCategory = {
  labelKey: 'notifications.kinds.important',
  icon: AlertTriangle,
  tone: 'text-red-600 dark:text-red-400',
  tint: 'bg-red-500/15',
}

const CATEGORIES: Record<string, NotificationCategory> = {
  broadcast: { labelKey: 'notifications.kinds.broadcast', icon: Megaphone, tone: 'text-primary', tint: 'bg-primary/15' },
  payment: { labelKey: 'notifications.kinds.payment', icon: CreditCard, tone: 'text-emerald-600 dark:text-emerald-400', tint: 'bg-emerald-500/15' },
  expiring: { labelKey: 'notifications.kinds.expiring', icon: CalendarClock, tone: 'text-amber-600 dark:text-amber-400', tint: 'bg-amber-500/15' },
  referral: { labelKey: 'notifications.kinds.referral', icon: Gift, tone: 'text-violet-600 dark:text-violet-400', tint: 'bg-violet-500/15' },
  support: { labelKey: 'notifications.kinds.support', icon: Headset, tone: 'text-blue-600 dark:text-blue-400', tint: 'bg-blue-500/15' },
  partner: { labelKey: 'notifications.kinds.partner', icon: Handshake, tone: 'text-indigo-600 dark:text-indigo-400', tint: 'bg-indigo-500/15' },
  devices: { labelKey: 'notifications.kinds.devices', icon: MonitorSmartphone, tone: 'text-teal-600 dark:text-teal-400', tint: 'bg-teal-500/15' },
  loyalty: { labelKey: 'notifications.kinds.loyalty', icon: Star, tone: 'text-yellow-600 dark:text-yellow-400', tint: 'bg-yellow-500/15' },
  torrent: { labelKey: 'notifications.kinds.torrent', icon: Ban, tone: 'text-orange-600 dark:text-orange-400', tint: 'bg-orange-500/15' },
}

export function notificationCategory(item: Pick<NotificationDTO, 'kind' | 'important'>): NotificationCategory {
  if (item.important) return IMPORTANT
  return CATEGORIES[item.kind] ?? CATEGORIES.broadcast
}

/**
 * Кнопка-переход: раздел, значок и подпись. Подпись — та же, что админ видит
 * в форме рассылки и что уходит в Telegram («🔌 Мой VPN»): свой набор подписей
 * разошёлся бы с ботом. Эмодзи из неё убирается — вместо него значок
 * интерфейса, как у остальных кнопок кабинета.
 */
export type NotificationButton = { route: string; icon: LucideIcon; labelKey: string }

export const NOTIFICATION_BUTTONS: Record<string, NotificationButton> = {
  buy: { route: '/tariffs', icon: ShoppingCart, labelKey: 'admin.broadcast.buttons.buy' },
  connect: { route: '/connections', icon: Plug, labelKey: 'admin.broadcast.buttons.connect' },
  promo: { route: '/promocodes', icon: Ticket, labelKey: 'admin.broadcast.buttons.promo' },
  share_referral: { route: '/referral', icon: UserPlus, labelKey: 'admin.broadcast.buttons.shareReferral' },
  dashboard: { route: '/dashboard', icon: House, labelKey: 'admin.broadcast.links.dashboard' },
  tariffs: { route: '/tariffs', icon: Package, labelKey: 'admin.broadcast.links.tariffs' },
  connections: { route: '/connections', icon: Link2, labelKey: 'admin.broadcast.links.connections' },
  accounts: { route: '/accounts', icon: Users, labelKey: 'admin.broadcast.links.accounts' },
  profile: { route: '/profile', icon: User, labelKey: 'admin.broadcast.links.profile' },
  payments: { route: '/payments', icon: Receipt, labelKey: 'admin.broadcast.links.payments' },
  promocodes: { route: '/promocodes', icon: Ticket, labelKey: 'admin.broadcast.links.promocodes' },
  referral: { route: '/referral', icon: Gift, labelKey: 'admin.broadcast.links.referral' },
  partner: { route: '/partner', icon: Briefcase, labelKey: 'admin.broadcast.links.partner' },
  loyalty: { route: '/loyalty', icon: Star, labelKey: 'admin.broadcast.links.loyalty' },
  fortune: { route: '/fortune', icon: Dices, labelKey: 'admin.broadcast.links.fortune' },
  support: { route: '/support', icon: Headset, labelKey: 'admin.broadcast.links.support' },
}

/** Эмодзи, флаги и пробелы в начале строки: «⚠️ Важно» → «Важно». */
const LEADING_EMOJI = /^(?:[\p{Extended_Pictographic}\p{Emoji_Presentation}\p{Regional_Indicator}\u{FE0F}\u{200D}\u{20E3}]|\s)+/u

export function stripLeadingEmoji(text: string): string {
  return text.replace(LEADING_EMOJI, '')
}

/** Первая строка рассылки длиннее этого — уже текст, а не заголовок. */
const TITLE_MAX = 100

function plainText(html: string): string {
  // DOMParser даёт инертный документ: ни скриптов, ни загрузки картинок.
  const doc = new DOMParser().parseFromString(html.replace(/<br\s*\/?>/gi, ' '), 'text/html')
  return (doc.body.textContent ?? '').replace(/\s+/g, ' ').trim()
}

/** Теги строки закрыты там же, где открыты: делить по ней разметку безопасно. */
function tagsBalanced(html: string): boolean {
  const open = html.match(/<[a-z][\w-]*[^>]*>/gi)?.length ?? 0
  const close = html.match(/<\/[a-z][\w-]*>/gi)?.length ?? 0
  return open === close
}

/**
 * Заголовок и текст карточки. У личных событий заголовок есть, у рассылок —
 * нет: им становится короткая первая строка, если за ней есть что-то ещё.
 * Эмодзи в начале заголовка убирается — у карточки уже есть значок. Так же
 * берёт заголовок и плашка важного на главной.
 */
export function splitTitle(item: Pick<NotificationDTO, 'title' | 'body'>): { title: string; body: string } {
  if (item.title.trim()) return { title: stripLeadingEmoji(item.title.trim()), body: item.body }
  const body = item.body.trim()
  const nl = body.indexOf('\n')
  if (nl <= 0) return { title: '', body }
  const first = body.slice(0, nl)
  const rest = body.slice(nl + 1).replace(/^\s+/, '')
  const title = stripLeadingEmoji(plainText(first))
  if (!title || !rest || title.length > TITLE_MAX || !tagsBalanced(first)) return { title: '', body }
  return { title, body: rest }
}

function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()
}

/**
 * Когда пришло: «Сегодня, 19:19», «Вчера, 19:19», дальше «21.10.26 19:19».
 * Дата у каждой карточки своя, поэтому заголовков дней в ленте нет.
 */
export function formatWhen(iso: string, lang: string, today: string, yesterday: string): string {
  const d = new Date(iso)
  const time = d.toLocaleTimeString(lang, { hour: '2-digit', minute: '2-digit' })
  const now = new Date()
  if (sameDay(d, now)) return `${today}, ${time}`
  if (sameDay(d, new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1))) return `${yesterday}, ${time}`
  return `${d.toLocaleDateString(lang, { day: '2-digit', month: '2-digit', year: '2-digit' })} ${time}`
}

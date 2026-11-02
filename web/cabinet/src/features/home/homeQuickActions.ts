import {
  FileText,
  Handshake,
  Link2,
  MessageCircle,
  Newspaper,
  Star,
  TicketPercent,
  Users,
  type LucideIcon,
} from 'lucide-react'

import type { AuthBootstrapResponse } from '@/lib/api'
import { FortuneWheelIcon } from '@/features/fortune/FortuneWheelIcon'

/*
 * Быстрые действия главной на местах 3 и 4 (CABINET_HOME_QUICK_ACTIONS).
 * Места 1 и 2 зависят от состояния подписки и не настраиваются.
 * Список id дублируется в internal/config/home_quick_actions.go.
 */

export const HOME_QUICK_ACTION_IDS = [
  'promo',
  'invite',
  'partner',
  'fortune',
  'support',
  'info',
  'news',
  'feedback',
  'sublink',
] as const

export type HomeQuickActionId = (typeof HOME_QUICK_ACTION_IDS)[number]

export const DEFAULT_HOME_QUICK_ACTIONS: HomeQuickActionId[] = ['promo', 'invite']

export interface HomeQuickActionMeta {
  icon: LucideIcon
  /** Ключ подписи кнопки на главной. */
  labelKey: string
  to?: string
  /** Внешняя ссылка из bootstrap (новости и отзывы ведут в Telegram). */
  href?: (b: AuthBootstrapResponse | undefined) => string | undefined
  /** Кнопка не ведёт на страницу, а копирует ссылку подписки (см. HomeParts). */
  copiesSubscriptionLink?: boolean
  /** false — раздел выключен, кнопку на главной показывать нельзя. */
  available?: (b: AuthBootstrapResponse | undefined) => boolean
}

const siteLink = (key: string) => (b: AuthBootstrapResponse | undefined) => b?.site_links?.[key]?.trim() || undefined

export const HOME_QUICK_ACTIONS: Record<HomeQuickActionId, HomeQuickActionMeta> = {
  promo: { icon: TicketPercent, labelKey: 'home.qaPromo', to: '/promocodes' },
  invite: { icon: Users, labelKey: 'home.qaInvite', to: '/referral' },
  partner: {
    icon: Handshake,
    labelKey: 'home.qaPartner',
    to: '/partner',
    available: (b) => b?.partner_nav_visible !== false,
  },
  fortune: {
    icon: FortuneWheelIcon,
    labelKey: 'home.qaFortune',
    to: '/fortune',
    available: (b) => b?.fortune_nav_visible !== false,
  },
  support: { icon: MessageCircle, labelKey: 'home.qaSupport', to: '/support' },
  info: { icon: FileText, labelKey: 'home.qaInfo', to: '/support#cabinet-info' },
  news: { icon: Newspaper, labelKey: 'home.qaNews', href: siteLink('channel'), available: (b) => !!siteLink('channel')(b) },
  feedback: {
    icon: Star,
    labelKey: 'home.qaFeedback',
    href: siteLink('feedback'),
    available: (b) => !!siteLink('feedback')(b),
  },
  sublink: { icon: Link2, labelKey: 'home.qaSubLink', copiesSubscriptionLink: true },
}

export function isHomeQuickActionId(v: string): v is HomeQuickActionId {
  return (HOME_QUICK_ACTION_IDS as readonly string[]).includes(v)
}

export function homeQuickActionAvailable(id: HomeQuickActionId, b: AuthBootstrapResponse | undefined): boolean {
  return HOME_QUICK_ACTIONS[id].available?.(b) ?? true
}

/** Разбор значения настройки «promo,invite»; битое значение — по умолчанию. */
export function parseHomeQuickActions(value: string | undefined): HomeQuickActionId[] {
  const parts = (value ?? '')
    .split(',')
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean)
  if (parts.length !== DEFAULT_HOME_QUICK_ACTIONS.length) return [...DEFAULT_HOME_QUICK_ACTIONS]
  if (!parts.every(isHomeQuickActionId) || parts[0] === parts[1]) return [...DEFAULT_HOME_QUICK_ACTIONS]
  return parts as HomeQuickActionId[]
}

/**
 * Что реально стоит на местах 3 и 4. Выключенный раздел (партнёрка, колесо,
 * новости без ссылки) уступает своё место кнопке по умолчанию — пустого
 * места и кнопки в «раздел недоступен» быть не должно. Соседнее место
 * при этом не сдвигается.
 */
export function resolveHomeQuickActions(b: AuthBootstrapResponse | undefined): HomeQuickActionId[] {
  const slots: (HomeQuickActionId | null)[] = parseHomeQuickActions(b?.home_quick_actions?.join(',')).map((id) =>
    homeQuickActionAvailable(id, b) ? id : null,
  )
  return slots.map((id, i) => {
    if (id) return id
    const fallback = DEFAULT_HOME_QUICK_ACTIONS.find((d) => !slots.includes(d)) ?? DEFAULT_HOME_QUICK_ACTIONS[i]
    slots[i] = fallback
    return fallback
  })
}

import type { TFunction } from 'i18next'
import type { LucideIcon } from 'lucide-react'
import {
  ArrowUpRight,
  CalendarClock,
  CalendarX,
  CircleCheck,
  CircleX,
  Download,
  FerrisWheel,
  Gauge,
  Gem,
  Handshake,
  KeyRound,
  Link2,
  LogIn,
  Megaphone,
  MessageCircle,
  MessageSquareReply,
  MessageSquareText,
  MousePointerClick,
  RefreshCw,
  RotateCcw,
  Server,
  ServerOff,
  Shield,
  ShieldAlert,
  Smartphone,
  SmartphoneNfc,
  Sparkles,
  Terminal,
  TicketPercent,
  Trash2,
  Users,
  UserPlus,
  Wallet,
} from 'lucide-react'

import type { AdminEventDTO } from '@/lib/types/admin'

/** Пункты фильтра ленты — в порядке показа. Совпадают с adminevents.Categories. */
export const EVENT_CATEGORIES = ['money', 'users', 'subs', 'promo', 'support', 'nodes', 'admin'] as const
export type EventCategory = (typeof EVENT_CATEGORIES)[number]

/** Цвет точки категории в фильтре. */
export const CATEGORY_DOT: Record<EventCategory, string> = {
  money: 'bg-emerald-500',
  users: 'bg-sky-500',
  subs: 'bg-violet-500',
  promo: 'bg-amber-500',
  support: 'bg-pink-500',
  nodes: 'bg-rose-500',
  admin: 'bg-slate-400',
}

/** Плашка значка: фон и цвет по категории, красная — для сбоев. */
const TONE = {
  money: 'bg-emerald-500/[0.12] text-emerald-500 dark:text-emerald-400',
  users: 'bg-sky-500/[0.12] text-sky-500 dark:text-sky-400',
  subs: 'bg-violet-500/[0.12] text-violet-500 dark:text-violet-400',
  promo: 'bg-amber-500/[0.12] text-amber-600 dark:text-amber-400',
  support: 'bg-pink-500/[0.12] text-pink-500 dark:text-pink-400',
  nodes: 'bg-emerald-500/[0.12] text-emerald-500 dark:text-emerald-400',
  admin: 'bg-slate-500/[0.12] text-slate-600 dark:text-slate-300',
  bad: 'bg-rose-500/[0.12] text-rose-500 dark:text-rose-400',
} as const

export interface EventView {
  icon: LucideIcon
  tone: string
  title: string
  sub?: string
  /** Сумма справа: plus — пришли деньги, minus — не дошли. */
  amount?: { text: string; tone: 'plus' | 'minus' | 'plain' }
  /** Метка источника справа, когда суммы нет. */
  tag?: string
  /** Нажатие в боте и прочие мелочи: строка тише остальных. */
  quiet: boolean
  /** Ссылка у строки (кроме карточки клиента): «Список» у сводки истёкших. */
  link?: { to: string; label: string }
}

type Payload = Record<string, unknown>

const str = (p: Payload, k: string): string => {
  const v = p[k]
  return typeof v === 'string' ? v : typeof v === 'number' ? String(v) : ''
}
const num = (p: Payload, k: string): number => {
  const v = p[k]
  return typeof v === 'number' ? v : typeof v === 'string' && v.trim() !== '' ? Number(v) || 0 : 0
}

/** Сумма в валюте платежа: рубли — «₽», звёзды — «⭐», остальное — кодом. */
export function formatMoney(amount: number, currency: string, locale: string): string {
  const cur = (currency || 'RUB').toUpperCase()
  const n = new Intl.NumberFormat(locale, { maximumFractionDigits: cur === 'RUB' ? 0 : 2 }).format(amount)
  if (cur === 'RUB') return `${n} ₽`
  if (cur === 'XTR' || cur === 'STARS') return `${n} ⭐`
  return `${n} ${cur}`
}

const PROVIDER_LABEL: Record<string, string> = {
  yookasa: 'ЮKassa',
  yookassa: 'ЮKassa',
  platega: 'Platega',
  heleket: 'Heleket',
  crypto: 'CryptoPay',
  cryptopay: 'CryptoPay',
  telegram: 'Telegram Stars',
  tribute: 'Tribute',
}

export function providerLabel(p: string): string {
  return PROVIDER_LABEL[p.toLowerCase()] ?? p
}

const LOGIN_METHOD: Record<string, string> = {
  email: 'email',
  telegram: 'Telegram',
  google: 'Google',
  yandex: 'Яндекс',
  vk: 'VK',
}

/** Понятное название кнопки бота по её служебным данным. */
function botButtonLabel(data: string, t: TFunction): string {
  const key = data.split(/[?:]/)[0].replace(/_\d+$/, '').replace(/_$/, '')
  const known = t(`admin.monitoring.botButton.${key}`, { defaultValue: '' })
  if (known) return known
  return data.length > 32 ? `${data.slice(0, 32)}…` : data
}

/** Действие админа: «users/:id/extend» → «Продление подписки». */
function adminActionLabel(action: string, method: string, t: TFunction): string {
  const norm = action.replace(/\//g, '.').replace(/:id/g, 'id')
  const withMethod = t(`admin.monitoring.adminAction.${method}.${norm}`, { defaultValue: '' })
  if (withMethod) return withMethod
  const plain = t(`admin.monitoring.adminAction.any.${norm}`, { defaultValue: '' })
  return plain || action
}

function fortuneReward(type: string, value: number, t: TFunction): string {
  const m = /^(days|discount)_(\d+)$/.exec(type)
  if (m) return t(`admin.monitoring.fortune.${m[1]}`, { count: Number(m[2]) })
  if (type === 'xp' || type === 'micro') return t('admin.monitoring.fortune.xp', { count: value })
  if (type === 'free_spin') return t('admin.monitoring.fortune.freeSpin', { count: value })
  return type
}

function promoEffect(p: Payload, t: TFunction): string {
  const parts: string[] = []
  if (num(p, 'days')) parts.push(t('admin.monitoring.promo.days', { count: num(p, 'days') }))
  if (num(p, 'trial_days')) parts.push(t('admin.monitoring.promo.trial', { count: num(p, 'trial_days') }))
  if (num(p, 'devices')) parts.push(t('admin.monitoring.promo.devices', { count: num(p, 'devices') }))
  if (num(p, 'discount')) parts.push(t('admin.monitoring.promo.discount', { count: num(p, 'discount') }))
  return parts.join(' · ')
}

function minutesText(m: number, t: TFunction): string {
  if (m < 1) return t('admin.monitoring.lessMinute')
  if (m < 60) return t('admin.monitoring.minutes', { count: m })
  if (m < 60 * 48) return t('admin.monitoring.hours', { count: Math.round(m / 60) })
  return t('admin.monitoring.days', { count: Math.round(m / 1440) })
}

/** Подпись по типу события. Неизвестный тип показывается как есть: новое событие не пропадёт из ленты. */
export function buildEventView(e: AdminEventDTO, t: TFunction, locale: string): EventView {
  const p = (e.payload ?? {}) as Payload
  const T = (k: string, o?: Record<string, unknown>) => t(`admin.monitoring.ev.${k}`, o ?? {})
  const sourceTag = e.source === 'bot' ? T('srcBot') : e.source === 'cabinet' ? T('srcCabinet') : undefined
  const money = (sign: 'plus' | 'minus'): EventView['amount'] => {
    const amount = num(p, 'amount')
    if (!amount) return undefined
    const text = formatMoney(amount, str(p, 'currency'), locale)
    return { text: sign === 'plus' ? `+${text}` : text, tone: sign }
  }
  const payDetails = () =>
    [
      num(p, 'months') ? t('admin.monitoring.monthsShort', { count: num(p, 'months') }) : '',
      str(p, 'provider') ? providerLabel(str(p, 'provider')) : '',
      num(p, 'discount') ? t('admin.monitoring.discount', { count: num(p, 'discount') }) : '',
    ]
      .filter(Boolean)
      .join(' · ')
  const base = { quiet: e.level >= 2 }

  switch (e.type) {
    case 'user.registered':
      return { ...base, icon: UserPlus, tone: TONE.users, title: T('registered'), tag: sourceTag }
    case 'user.login':
      return {
        ...base, icon: LogIn, tone: TONE.users, title: T('login'),
        sub: str(p, 'method') ? T('via', { method: LOGIN_METHOD[str(p, 'method')] ?? str(p, 'method') }) : undefined,
      }
    case 'account.linked':
      return { ...base, icon: Link2, tone: TONE.users, title: T('linked', { provider: LOGIN_METHOD[str(p, 'provider')] ?? str(p, 'provider') }) }
    case 'account.merged':
      return { ...base, icon: Users, tone: TONE.users, title: T('merged') }
    case 'account.deleted':
      return { ...base, icon: Trash2, tone: TONE.bad, title: T('deleted'), sub: num(p, 'customer') ? `#${num(p, 'customer')}` : undefined }
    case 'account.email_verified':
      return { ...base, icon: CircleCheck, tone: TONE.users, title: T('emailVerified') }
    case 'account.password_reset':
      return { ...base, icon: KeyRound, tone: TONE.users, title: T('passwordReset') }

    case 'trial.activated':
      return { ...base, icon: Sparkles, tone: TONE.subs, title: T('trial'), sub: num(p, 'days') ? t('admin.monitoring.daysShort', { count: num(p, 'days') }) : undefined, tag: sourceTag }
    case 'payment.subscription_new':
      return { ...base, icon: Wallet, tone: TONE.money, title: str(p, 'tariff') ? T('buyTariff', { tariff: str(p, 'tariff') }) : T('buy'), sub: payDetails(), amount: money('plus') }
    case 'payment.subscription_renew':
      return { ...base, icon: RefreshCw, tone: TONE.money, title: str(p, 'tariff') ? T('renewTariff', { tariff: str(p, 'tariff') }) : T('renew'), sub: payDetails(), amount: money('plus') }
    case 'payment.tariff_upgrade':
      return { ...base, icon: ArrowUpRight, tone: TONE.money, title: p.downgrade ? T('downgrade', { tariff: str(p, 'tariff') }) : T('upgrade', { tariff: str(p, 'tariff') }), sub: payDetails(), amount: money('plus') }
    case 'payment.extra_hwid':
      return { ...base, icon: Smartphone, tone: TONE.money, title: T('extraHwid', { count: num(p, 'devices') || 1 }), sub: payDetails(), amount: money('plus') }
    case 'payment.extra_traffic': {
      const gb = num(p, 'traffic_bytes') / 1024 ** 3
      return {
        ...base, icon: Gauge, tone: TONE.money,
        title: T('extraTraffic', { gb: new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(gb) }),
        sub: payDetails(), amount: money('plus'),
      }
    }
    case 'payment.failed':
      return {
        // Брошенный счёт — не авария, красным только разбор Heleket.
        ...base, icon: CircleX,
        tone: str(p, 'reason') === 'heleket_needs_attention' ? TONE.bad : TONE.admin,
        title: str(p, 'reason') === 'heleket_needs_attention' ? T('heleketReview') : T('unpaid'),
        sub: [str(p, 'provider') ? providerLabel(str(p, 'provider')) : '', str(p, 'reason') ? T(`reason.${str(p, 'reason')}`, { defaultValue: str(p, 'reason') }) : '']
          .filter(Boolean).join(' · '),
        amount: money('minus'),
      }
    case 'subscription.expired':
      return { ...base, icon: CalendarX, tone: TONE.subs, title: T('expired') }
    case 'subscription.expired_digest':
      return {
        ...base, icon: CalendarClock, tone: TONE.subs,
        title: T('expiredDigest', { count: num(p, 'expired') }),
        sub: T('notRenewed', { count: num(p, 'not_renewed') }),
        link: { to: '/admin/users?scope=inactive', label: T('list') },
      }
    case 'subscription.link_reset':
      return { ...base, icon: RotateCcw, tone: TONE.subs, title: T('linkReset'), tag: sourceTag }
    case 'device.removed':
      return { ...base, icon: SmartphoneNfc, tone: TONE.subs, title: T('deviceRemoved'), tag: sourceTag }
    case 'torrent.report':
      return { ...base, icon: ShieldAlert, tone: TONE.bad, title: T('torrent'), sub: [str(p, 'node'), num(p, 'minutes') ? minutesText(num(p, 'minutes'), t) : ''].filter(Boolean).join(' · ') }

    case 'promo.activated':
      return { ...base, icon: TicketPercent, tone: TONE.promo, title: T('promo', { code: str(p, 'code') }), sub: promoEffect(p, t) || undefined, tag: sourceTag }
    case 'promo.invalid':
      return { ...base, icon: TicketPercent, tone: TONE.bad, title: T('promoInvalid', { code: str(p, 'code') }) }
    case 'fortune.spin':
      return { ...base, icon: FerrisWheel, tone: TONE.promo, title: T('fortune'), sub: T('won', { reward: fortuneReward(str(p, 'reward_type'), num(p, 'reward_value'), t) }) }
    case 'loyalty.level_up':
      return { ...base, icon: Gem, tone: TONE.promo, title: T('loyalty', { level: num(p, 'level') }), sub: num(p, 'discount') ? t('admin.monitoring.discount', { count: num(p, 'discount') }) : undefined }
    case 'referral.bonus':
      return { ...base, icon: Users, tone: TONE.promo, title: T('referral', { count: num(p, 'days') }), sub: p.to_referrer ? T('forInvite') : T('forJoin') }
    case 'partner.applied':
      return { ...base, icon: Handshake, tone: TONE.promo, title: T('partnerApplied'), sub: p.auto_approved ? T('autoApproved') : undefined }
    case 'partner.payout_requested':
      return { ...base, icon: Download, tone: TONE.promo, title: T('payoutRequested'), amount: num(p, 'amount') ? { text: formatMoney(num(p, 'amount'), 'RUB', locale), tone: 'plain' } : undefined }

    case 'support.message':
      return { ...base, icon: MessageCircle, tone: TONE.support, title: T('supportMessage'), sub: str(p, 'preview') ? `«${str(p, 'preview')}»` : num(p, 'images') ? T('images', { count: num(p, 'images') }) : undefined }
    case 'support.reply':
      return { ...base, icon: MessageSquareReply, tone: TONE.support, title: T('supportReply'), sub: [str(p, 'author'), str(p, 'preview') ? `«${str(p, 'preview')}»` : ''].filter(Boolean).join(': ') || undefined }
    case 'support.closed':
      return { ...base, icon: CircleCheck, tone: TONE.support, title: T('supportClosed') }
    case 'bot.message':
      return { ...base, icon: MessageSquareText, tone: TONE.support, title: T('botMessage'), sub: str(p, 'preview') ? `«${str(p, 'preview')}»` : undefined }
    case 'bot.command':
      return { ...base, quiet: true, icon: Terminal, tone: TONE.admin, title: T('command', { command: str(p, 'command') }) }
    case 'bot.button':
      return { ...base, quiet: true, icon: MousePointerClick, tone: TONE.admin, title: T('button', { label: botButtonLabel(str(p, 'data'), t) }) }

    case 'node.down': {
      const probe = Boolean(p.probe)
      const reason = str(p, 'reason')
      const sub = probe
        ? reason === 'ru' && num(p, 'total')
          ? T('fromRussia', { ok: num(p, 'ok'), total: num(p, 'total') })
          : T(`probe.${reason}`, { defaultValue: T('probe.any') })
        : T('panelLost')
      return { ...base, icon: ServerOff, tone: TONE.bad, title: T('nodeDown', { name: str(p, 'name') }), sub }
    }
    case 'node.up':
      return {
        ...base, icon: Server, tone: TONE.nodes, title: T('nodeUp', { name: str(p, 'name') }),
        sub: num(p, 'down_minutes') ? T('downFor', { time: minutesText(num(p, 'down_minutes'), t) }) : undefined,
      }

    case 'system.broadcast':
      return {
        ...base, icon: Megaphone, tone: TONE.admin,
        title: T('broadcast', { sent: num(p, 'sent'), total: num(p, 'total') }),
        sub: [num(p, 'blocked') ? T('blocked', { count: num(p, 'blocked') }) : '', num(p, 'failed') ? T('failedSends', { count: num(p, 'failed') }) : '']
          .filter(Boolean).join(' · ') || undefined,
      }
    case 'system.sync':
      return { ...base, icon: RefreshCw, tone: TONE.admin, title: T('sync'), sub: T('syncDetails', { created: num(p, 'created'), updated: num(p, 'updated'), deleted: num(p, 'deleted') }) }
    case 'admin.action': {
      const params = (p.params ?? {}) as Payload
      const details = Object.entries(params)
        .slice(0, 3)
        .map(([k, v]) => `${k}: ${String(v)}`)
        .join(' · ')
      return {
        ...base, icon: Shield, tone: TONE.admin,
        title: adminActionLabel(str(p, 'action'), str(p, 'method'), t),
        sub: details || undefined,
      }
    }
    default:
      return { ...base, icon: Shield, tone: TONE.admin, title: e.type }
  }
}

/** Время строки: «14:32». */
export function formatTime(iso: string, locale: string): string {
  return new Date(iso).toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' })
}

/** Подпись дня над группой строк: «Сегодня», «Вчера», «4 сентября». */
export function dayLabel(iso: string, locale: string, t: TFunction): string {
  const d = new Date(iso)
  const today = new Date()
  const start = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime()
  const diff = Math.round((start(today) - start(d)) / 86_400_000)
  if (diff === 0) return t('admin.monitoring.today')
  if (diff === 1) return t('admin.monitoring.yesterday')
  return d.toLocaleDateString(locale, {
    day: 'numeric',
    month: 'long',
    ...(d.getFullYear() !== today.getFullYear() ? { year: 'numeric' } : {}),
  })
}

export function dayKey(iso: string): string {
  const d = new Date(iso)
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`
}

/** «12 мин», «3 ч», «2 дн» — сколько длится инцидент. */
export function sinceText(iso: string | undefined, t: TFunction, now = Date.now()): string {
  if (!iso) return ''
  return minutesText(Math.max(0, Math.floor((now - new Date(iso).getTime()) / 60_000)), t)
}

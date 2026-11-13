import {
  EyeOff,
  Globe2,
  Headphones,
  KeyRound,
  MonitorSmartphone,
  ShieldCheck,
  UserPlus,
  Wifi,
  Zap,
  type LucideIcon,
} from 'lucide-react'

/**
 * Наполнение лендинга: списки секций.
 *
 * Тексты живут в i18n (`landing.*` в src/i18n/{ru,en}.json) — здесь только
 * ключи, иконки и акцентный цвет. Чтобы отредактировать копирайт, править
 * нужно i18n, а не этот файл.
 */

/** Совпадает с --lp-* в landing.css. */
export type LandingAccent = 'cyan' | 'violet' | 'emerald' | 'amber'

export function accentVar(accent: LandingAccent): string {
  return `var(--lp-${accent})`
}

export interface LandingFeature {
  id: string
  icon: LucideIcon
  accent: LandingAccent
}

export const LANDING_FEATURES: LandingFeature[] = [
  { id: 'protocols', icon: ShieldCheck, accent: 'cyan' },
  { id: 'speed', icon: Zap, accent: 'amber' },
  { id: 'locations', icon: Globe2, accent: 'violet' },
  { id: 'noLogs', icon: EyeOff, accent: 'emerald' },
  { id: 'devices', icon: MonitorSmartphone, accent: 'cyan' },
  { id: 'support', icon: Headphones, accent: 'violet' },
]

export interface LandingStep {
  id: string
  icon: LucideIcon
  accent: LandingAccent
}

export const LANDING_STEPS: LandingStep[] = [
  { id: 'signup', icon: UserPlus, accent: 'cyan' },
  { id: 'plan', icon: KeyRound, accent: 'violet' },
  { id: 'connect', icon: Wifi, accent: 'emerald' },
]

/** Порядок пунктов FAQ на странице. */
export const LANDING_FAQ_IDS = [
  'whatIs',
  'devices',
  'apps',
  'payment',
  'limits',
  'refund',
] as const

/** Пункты меню. href абсолютный: страница живёт и на корне домена, и внутри /cabinet. */
export const LANDING_NAV = [
  { id: 'how', href: '/#how' },
  { id: 'tariffs', href: '/#tariffs' },
  { id: 'faq', href: '/#faq' },
  { id: 'status', href: '/status' },
] as const

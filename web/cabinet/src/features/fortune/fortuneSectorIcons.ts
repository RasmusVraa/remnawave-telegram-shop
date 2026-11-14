import {
  BadgePercent,
  CalendarPlus,
  Gem,
  Gift,
  Sparkles,
  Ticket,
  type LucideIcon,
} from 'lucide-react'

/** Иконки призов колеса (и модалка, и витрина). */
export const FORTUNE_SECTOR_ICONS: Record<string, LucideIcon> = {
  micro: Sparkles,
  xp: Gem,
  discount_3: BadgePercent,
  discount_5: BadgePercent,
  days_3: CalendarPlus,
  days_5: CalendarPlus,
  days_7: CalendarPlus,
  days_15: Gift,
  days_30: Gift,
  days_180: Gift,
  free_spin: Ticket,
}

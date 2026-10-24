import { Heart, MessageCircle, type LucideIcon } from 'lucide-react'

import type { DecorThemeId } from './decorThemes'
import { useCabinetDecorTheme } from './useCabinetDecorTheme'

type NavIconProps = {
  className?: string
  strokeWidth?: number
}

/** Снеговик для new_year — читается даже на 18–20px. */
export function DecorSnowmanIcon({ className, strokeWidth = 1.75 }: NavIconProps) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <circle cx="12" cy="6.5" r="3" />
      <circle cx="12" cy="13.5" r="4" />
      <circle cx="12" cy="20.5" r="2.75" />
      <circle cx="10.5" cy="5.5" r="0.45" fill="currentColor" stroke="none" />
      <circle cx="13.5" cy="5.5" r="0.45" fill="currentColor" stroke="none" />
      <path d="M10 7.5h4" />
      <path d="M9 12.5h6" />
      <path d="M7.5 14l-2.25 1.25" />
      <path d="M16.5 14l2.25 1.25" />
    </svg>
  )
}

/** Ёлка для new_year вместо «дома». */
export function DecorTreeIcon({ className, strokeWidth = 1.75 }: NavIconProps) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M12 2.5l1 2h-2z" fill="currentColor" />
      <path d="M12 5L8 10h2.5L7 15h3L6 20h12l-4-5h3l-3.5-5H16z" />
      <path d="M12 20v2" />
    </svg>
  )
}

export function resolveDecorNavIcon(
  theme: DecorThemeId,
  to: string,
  defaultIcon: LucideIcon,
): LucideIcon | typeof DecorSnowmanIcon | typeof DecorTreeIcon {
  if (theme === 'valentine' && to === '/support') return Heart
  if (theme === 'new_year' && to === '/profile') return DecorSnowmanIcon
  if (theme === 'new_year' && to === '/dashboard') return DecorTreeIcon
  return defaultIcon
}

type DecorNavIconProps = NavIconProps & {
  to: string
  defaultIcon: LucideIcon
  theme: DecorThemeId
}

export function DecorNavIcon({ to, defaultIcon: DefaultIcon, theme, className, strokeWidth }: DecorNavIconProps) {
  const Icon = resolveDecorNavIcon(theme, to, DefaultIcon)
  return <Icon className={className} strokeWidth={strokeWidth} />
}

/** Иконка поддержки: сердце в valentine, иначе MessageCircle. */
export function DecorSupportIcon({ className, strokeWidth = 1.75 }: NavIconProps) {
  const theme = useCabinetDecorTheme()
  if (theme === 'valentine') {
    return <Heart className={className} strokeWidth={strokeWidth} aria-hidden />
  }
  return <MessageCircle className={className} strokeWidth={strokeWidth} aria-hidden />
}

import type { CSSProperties } from 'react'

import type { DecorThemeId } from './decorThemes'

/** Только акцентные цвета — без частиц, сцен и кастомных иконок. */
export const DECOR_COLOR_ONLY_THEMES = [
  'green',
  'pink',
  'orange',
  'yellow',
  'violet',
  'slate',
  'carbon',
] as const

export type DecorColorOnlyThemeId = (typeof DECOR_COLOR_ONLY_THEMES)[number]

export function isDecorColorOnlyTheme(theme: DecorThemeId): boolean {
  return (DECOR_COLOR_ONLY_THEMES as readonly string[]).includes(theme)
}

/** Цвет подписи опции в админке (акцент темы). */
export const DECOR_THEME_ADMIN_LABEL_COLOR: Partial<Record<DecorThemeId, string>> = {
  green: 'hsl(142 52% 38%)',
  pink: 'hsl(338 78% 52%)',
  orange: 'hsl(28 95% 48%)',
  yellow: 'hsl(38 92% 46%)',
  violet: 'hsl(262 70% 52%)',
  slate: 'hsl(215 16% 46%)',
  carbon: 'hsl(217 80% 55%)',
  neon: 'hsl(186 100% 42%)',
  new_year: 'hsl(198 85% 52%)',
  new_year_2: 'hsl(198 85% 52%)',
  summer: 'hsl(38 92% 46%)',
  halloween: 'hsl(28 95% 48%)',
  halloween_2: 'hsl(28 95% 48%)',
  valentine: 'hsl(338 78% 52%)',
  spring: 'hsl(142 52% 38%)',
  black_friday: 'hsl(45 95% 46%)',
  aurora: 'hsl(168 75% 48%)',
  nebula: 'hsl(198 89% 50%)',
  ocean: 'hsl(210 85% 48%)',
  cyber: 'hsl(140 90% 42%)',
  sunset: 'hsl(18 92% 54%)',
  lavender: 'hsl(270 45% 58%)',
  wine: 'hsl(346 70% 40%)',
}

export function decorThemeOptionLabelStyle(themeId: string): CSSProperties | undefined {
  const color = DECOR_THEME_ADMIN_LABEL_COLOR[themeId as DecorThemeId]
  return color ? { color } : undefined
}

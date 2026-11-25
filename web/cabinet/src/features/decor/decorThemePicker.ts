import type { DecorThemeId } from './decorThemes'

/**
 * Темы для окна выбора — по группам, как в макете (.cursor/work-in-progress/theme-picker).
 * Все темы кроме `off` должны быть ровно в одной группе.
 */
export const DECOR_PICKER_GROUPS: { id: 'base' | 'color' | 'seasonal'; themes: DecorThemeId[] }[] = [
  { id: 'base', themes: ['green', 'slate', 'carbon', 'violet', 'pink', 'orange', 'yellow'] },
  {
    id: 'color',
    themes: ['lime', 'emerald', 'cyber', 'neon', 'ocean', 'aurora', 'nebula', 'lavender', 'wine', 'sunset'],
  },
  {
    id: 'seasonal',
    themes: [
      'spring',
      'summer',
      'valentine',
      'halloween',
      'halloween_2',
      'new_year',
      'new_year_2',
      'black_friday',
    ],
  },
]

/** Акцент темы для мини-превью плитки и кружка в строке настроек (тёмная тема). */
export const DECOR_PICKER_ACCENT: Record<DecorThemeId, string> = {
  off: 'hsl(215 14% 52%)',
  green: 'hsl(142 55% 46%)',
  spring: 'hsl(142 55% 46%)',
  cyber: 'hsl(140 90% 46%)',
  lime: 'hsl(96 68% 52%)',
  emerald: 'hsl(158 57% 51%)',
  neon: 'hsl(186 100% 50%)',
  ocean: 'hsl(205 85% 52%)',
  new_year: 'hsl(198 85% 52%)',
  new_year_2: 'hsl(198 85% 52%)',
  slate: 'hsl(215 20% 62%)',
  carbon: 'hsl(217 90% 60%)',
  aurora: 'hsl(168 75% 48%)',
  nebula: 'hsl(198 89% 50%)',
  violet: 'hsl(262 72% 62%)',
  lavender: 'hsl(270 48% 68%)',
  pink: 'hsl(338 78% 58%)',
  valentine: 'hsl(338 78% 58%)',
  wine: 'hsl(346 66% 52%)',
  sunset: 'hsl(18 92% 56%)',
  orange: 'hsl(28 95% 52%)',
  halloween: 'hsl(28 95% 52%)',
  halloween_2: 'hsl(28 95% 52%)',
  yellow: 'hsl(42 95% 52%)',
  summer: 'hsl(42 95% 52%)',
  black_friday: 'hsl(45 95% 50%)',
}

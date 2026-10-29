/** Декоративные темы кабинета — порядок = порядок в админ-селекте (по цветовым группам). */
export const DECOR_THEME_IDS = [
  'off',
  // greens
  'green',
  'spring',
  'cyber',
  // cyan / blue
  'neon',
  'ocean',
  'new_year',
  'new_year_2',
  'slate',
  'carbon',
  // teal–violet bridge + purple
  'aurora',
  'nebula',
  'violet',
  'lavender',
  // pink / red
  'pink',
  'valentine',
  'wine',
  // warm
  'sunset',
  'orange',
  'halloween',
  'halloween_2',
  // yellow / gold
  'yellow',
  'summer',
  'black_friday',
] as const

export type DecorThemeId = (typeof DECOR_THEME_IDS)[number]

export type DecorEffectKind =
  | 'snow'
  | 'sunrays'
  | 'pumpkins'
  | 'hearts'
  | 'petals'
  | 'sparks'
  | 'aurora'
  | 'bubbles'
  | 'matrix'
  | 'embers'
  | 'dots'
  | null

export interface DecorThemeDef {
  id: DecorThemeId
  effect: DecorEffectKind
}

export const DECOR_THEMES: Record<DecorThemeId, DecorThemeDef> = {
  off: { id: 'off', effect: null },
  green: { id: 'green', effect: null },
  spring: { id: 'spring', effect: 'petals' },
  cyber: { id: 'cyber', effect: 'matrix' },
  neon: { id: 'neon', effect: null },
  ocean: { id: 'ocean', effect: 'bubbles' },
  new_year: { id: 'new_year', effect: null },
  // Сцена со своим снегом — HolidayAtmosphere, слой частиц не нужен.
  new_year_2: { id: 'new_year_2', effect: null },
  slate: { id: 'slate', effect: null },
  carbon: { id: 'carbon', effect: null },
  aurora: { id: 'aurora', effect: 'aurora' },
  nebula: { id: 'nebula', effect: null },
  violet: { id: 'violet', effect: null },
  lavender: { id: 'lavender', effect: 'dots' },
  pink: { id: 'pink', effect: null },
  valentine: { id: 'valentine', effect: 'hearts' },
  wine: { id: 'wine', effect: null },
  sunset: { id: 'sunset', effect: 'embers' },
  orange: { id: 'orange', effect: null },
  halloween: { id: 'halloween', effect: null },
  halloween_2: { id: 'halloween_2', effect: null },
  yellow: { id: 'yellow', effect: null },
  summer: { id: 'summer', effect: 'sunrays' },
  black_friday: { id: 'black_friday', effect: 'sparks' },
}

export function normalizeDecorTheme(value: string | undefined | null): DecorThemeId {
  if (!value) return 'off'
  const v = value.trim().toLowerCase()
  if (v in DECOR_THEMES) return v as DecorThemeId
  return 'off'
}

/** Темы, у которых нет светлого варианта: пока они включены, кабинет только тёмный. */
const DARK_ONLY_DECOR_THEMES: ReadonlySet<DecorThemeId> = new Set(['halloween_2', 'new_year_2'])

export function isDarkOnlyDecorTheme(theme: DecorThemeId): boolean {
  return DARK_ONLY_DECOR_THEMES.has(theme)
}

export function decorEffectForTheme(theme: DecorThemeId): DecorEffectKind {
  return DECOR_THEMES[theme].effect
}

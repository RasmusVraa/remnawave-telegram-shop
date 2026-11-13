/*
 * Цвет серверов обхода в раздельном трафике (--split-bypass*).
 *
 * По умолчанию фиолетовый. Но в фиолетовых темах основной цвет сам фиолетовый, и обход
 * сливается с обычными серверами, поэтому там берётся бирюзовый. Тема определяется по
 * оттенку --primary, а не по имени темы: тем много, а оттенок один на всех.
 *
 * Модуль подключается побочным эффектом из компонентов, которые рисуют обход, и следит
 * за сменой темы (класс dark, декор-тема, цвет акцента) сам.
 */

type Palette = { base: string; deep: string; alt: string; soft: string }

const VIOLET: Palette = { base: '262 83% 68%', deep: '262 83% 62%', alt: '280 80% 62%', soft: '262 83% 78%' }
const TEAL: Palette = { base: '168 70% 48%', deep: '168 70% 40%', alt: '186 72% 42%', soft: '168 70% 62%' }

/** Насколько близко к фиолетовому (240–300°) основной цвет темы. */
function isVioletish(hue: number): boolean {
  return hue >= 235 && hue <= 305
}

function apply() {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  const raw = getComputedStyle(root).getPropertyValue('--primary').trim()
  const hue = parseFloat(raw.split(/\s+/)[0])
  const p = Number.isFinite(hue) && isVioletish(hue) ? TEAL : VIOLET
  if (root.dataset.splitBypass === p.base) return
  root.dataset.splitBypass = p.base
  root.style.setProperty('--split-bypass', p.base)
  root.style.setProperty('--split-bypass-deep', p.deep)
  root.style.setProperty('--split-bypass-alt', p.alt)
  root.style.setProperty('--split-bypass-soft', p.soft)
}

if (typeof document !== 'undefined') {
  apply()
  new MutationObserver(apply).observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class', 'data-cabinet-decor', 'data-theme', 'style'],
  })
}

export {}

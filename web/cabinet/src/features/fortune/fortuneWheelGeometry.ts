/**
 * Смещение центра чипа от центра квадрата колеса (для `left` / `top` в %).
 * Угол — как в CSS `rotate()`: 0° вправо, по часовой стрелке; `-90°` — вверх.
 * Радиус — доля половины стороны (как у `cos`/`sin` при единичной окружности), в процентах от **полной** ширины/высоты квадрата: для круга вписанного в квадрат расстояние до обода = 50%.
 */
export function fortuneChipOffsetPercent(angleCssDeg: number, radiusPct: number): { x: number; y: number } {
  const rad = (angleCssDeg * Math.PI) / 180
  return {
    x: radiusPct * Math.cos(rad),
    y: radiusPct * Math.sin(rad),
  }
}

/**
 * Угол для `cos`/`sin` и `rotate` чипа (CSS: 0° вправо, по часовой): биссектриса сектора в стандартной тригонометрии.
 * Совпадает с прежним `-90 + (i+0.5)*step` при том же визуальном положении на круге.
 */
export function sectorCenterDeg(sectorIndex: number, sectorCount: number): number {
  const step = 360 / Math.max(sectorCount, 1)
  return -90 + (sectorIndex + 0.5) * step
}

/**
 * Следующий суммарный `rotate(deg)` колеса: указатель совпадает с центром выигравшего сектора.
 * `currentRotation` — накопленный угол без нормализации (как в state React).
 */
/**
 * Индекс сектора, чей участок сейчас у верхней стрелки, при суммарном повороте `rotationDeg` (как в state).
 */
export function sectorIndexUnderPointer(rotationDeg: number, sectorCount: number): number {
  const step = 360 / Math.max(sectorCount, 1)
  const Ln = ((-rotationDeg % 360) + 360) % 360
  for (let i = 0; i < sectorCount; i++) {
    const start = i * step
    const end = (i + 1) * step
    if (Ln >= start && Ln < end) return i
  }
  return 0
}

export function nextSpinRotation(params: {
  currentRotation: number
  sectorIndex: number
  sectorCount: number
  fullSpins?: number
}): number {
  const { currentRotation, sectorIndex, sectorCount, fullSpins = 5 } = params
  const step = 360 / Math.max(sectorCount, 1)
  const bisectorCw = (sectorIndex + 0.5) * step
  const rem = ((-bisectorCw - currentRotation) % 360 + 360) % 360
  const jitter = (Math.random() - 0.5) * Math.min(step * 0.22, 18)
  return currentRotation + fullSpins * 360 + rem + jitter
}

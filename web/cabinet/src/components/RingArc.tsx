import { useId } from 'react'

/**
 * Дуга кольца-показателя в SVG с viewBox 0 0 100 100 — общая для плиток «Дней», «Трафика»
 * и «Устройств» (StatRing) и вложенных колец раздельного трафика (NestedStatRing).
 *
 * Оба конца дуги скруглены, а на конце — «кончик» с мягкой тенью по ходу кольца, как у
 * колец Apple Watch: на 100% кончик ложится поверх начала кольца и читается, а не
 * сливается в сплошной круг. Тень обрезана полосой кольца и на фон плитки не выходит.
 *
 * Безлимитные кольца сюда не ходят: у них нет конца, а значит, ни кончика, ни тени.
 */

/** Толщина кольца в пикселях — одна для всех плиток, чтобы ряд выглядел ровно. */
export const RING_THICKNESS_PX = 7

const point = (r: number, deg: number): [number, number] => {
  const a = (deg * Math.PI) / 180
  return [50 + r * Math.sin(a), 50 - r * Math.cos(a)]
}

/** Отрезок окружности по часовой стрелке от a1 до a2 (градусы от верхней точки). */
const arcPath = (r: number, a1: number, a2: number) => {
  const [x1, y1] = point(r, a1)
  const [x2, y2] = point(r, a2)
  return `M${x1} ${y1} A${r} ${r} 0 ${a2 - a1 > 180 ? 1 : 0} 1 ${x2} ${y2}`
}

export function RingArc({
  r,
  w,
  value,
  color,
  trackColor = color,
  trackOpacity = 0.18,
}: {
  r: number
  w: number
  /** 0–100. */
  value: number
  color: string
  trackColor?: string
  trackOpacity?: number
}) {
  const id = useId().replace(/[^a-zA-Z0-9_-]/g, '')
  const pct = Math.min(100, Math.max(0, value))
  const c = 2 * Math.PI * r
  const end = pct * 3.6
  const t = (end * Math.PI) / 180
  const [ex, ey] = point(r, end)
  return (
    <>
      <circle cx="50" cy="50" r={r} fill="none" style={{ stroke: trackColor }} strokeOpacity={trackOpacity} strokeWidth={w} />
      {pct > 0 && (
        <>
          <defs>
            {/* Область фильтра — весь рисунок: по габаритам самой фигуры размытие обрезается прямыми краями. */}
            <filter id={`${id}-blur`} filterUnits="userSpaceOnUse" x="0" y="0" width="100" height="100">
              <feGaussianBlur stdDeviation={w * 0.3} />
            </filter>
            <mask id={`${id}-band`} maskUnits="userSpaceOnUse" x="0" y="0" width="100" height="100">
              <circle cx="50" cy="50" r={r} fill="none" stroke="#fff" strokeWidth={w} />
            </mask>
          </defs>
          <circle
            cx="50"
            cy="50"
            r={r}
            fill="none"
            style={{ stroke: color }}
            strokeWidth={w}
            strokeLinecap="round"
            strokeDasharray={c}
            strokeDashoffset={c * (1 - pct / 100)}
            transform="rotate(-90 50 50)"
          />
          {/* Тень — размытый кружок под круглым кончиком, чуть впереди: расходится от кончика вперёд и к краям полосы. */}
          <g mask={`url(#${id}-band)`}>
            <circle
              cx={ex + Math.cos(t) * w * 0.28}
              cy={ey + Math.sin(t) * w * 0.28}
              r={w / 2}
              fill="#000"
              className="ring-tip-shadow"
              filter={`url(#${id}-blur)`}
            />
          </g>
          {/* Кончик поверх тени: тень видна только впереди него. */}
          <path
            d={arcPath(r, Math.max(0, end - 40), end)}
            fill="none"
            style={{ stroke: color }}
            strokeWidth={w}
            strokeLinecap="round"
          />
        </>
      )}
    </>
  )
}

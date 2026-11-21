import { useEffect, useRef } from 'react'

import { wheelSectorRimSolid } from '@/features/fortune/fortunePrizeVisuals'

type FortuneConfettiProps = {
  /** Номер выигрыша: растёт на единицу при каждой остановке колеса, 0 — салюта ещё не было. */
  burstId: number
  prize: string | null
}

type Piece = {
  x: number
  y: number
  vx: number
  vy: number
  r: number
  rot: number
  vr: number
  life: number
  t: number
  color: string
  rect: boolean
}

/** Сколько конфетти: чем ценнее приз, тем пышнее салют. */
function pieceCount(prize: string | null): number {
  if (prize === 'days_180' || prize === 'days_30') return 260
  if (prize === 'days_15') return 170
  if (prize === 'micro') return 50
  return 90
}

const PALETTE = ['#ffffff', '#ffd24a', '#4aa8f0', '#f78fd3', '#5be08f']
/** Скорости и гравитация заданы для канвы шириной 1116 px (колесо 900 px + поля), дальше масштабируются. */
const REF_WIDTH = 1116

/**
 * Салют на остановке колеса: из центра вверх веером летит конфетти, падает под гравитацией и гаснет.
 * Канва шире колеса на 12% с каждой стороны, чтобы частицы не обрезались по ободу.
 */
export function FortuneConfetti({ burstId, prize }: FortuneConfettiProps) {
  const ref = useRef<HTMLCanvasElement>(null)
  const piecesRef = useRef<Piece[]>([])
  const rafRef = useRef(0)
  // Приз читаем из ref: смена приза на следующей крутке не должна запускать новый салют и обрывать текущий.
  const prizeRef = useRef(prize)
  prizeRef.current = prize

  useEffect(() => {
    const canvas = ref.current
    if (!canvas || burstId === 0) return undefined
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return undefined
    const ctx = canvas.getContext('2d')
    if (!ctx) return undefined

    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    const w = canvas.clientWidth
    const h = canvas.clientHeight
    canvas.width = Math.ceil(w * dpr)
    canvas.height = Math.ceil(h * dpr)
    // Масштаб считаем от целых пикселей канвы, а не от dpr: при дробной плотности (125%, 150%) иначе крайний столбец и строка
    // стираются не целиком, и на правом и нижнем краях остаются тонкие «следы» конфетти.
    const sx = canvas.width / w
    const sy = canvas.height / h
    const clear = () => {
      ctx.setTransform(1, 0, 0, 1, 0, 0)
      ctx.clearRect(0, 0, canvas.width, canvas.height)
    }
    const k = w / REF_WIDTH

    const prizeNow = prizeRef.current
    const colors = [prizeNow ? wheelSectorRimSolid(prizeNow, 0) : '#ffd24a', ...PALETTE]
    const n = pieceCount(prizeNow)
    for (let i = 0; i < n; i++) {
      const a = ((-90 + (Math.random() - 0.5) * 150) * Math.PI) / 180
      const v = (500 + Math.random() * 700) * k
      piecesRef.current.push({
        x: w / 2,
        y: h * 0.46,
        vx: Math.cos(a) * v,
        vy: Math.sin(a) * v,
        r: (3 + Math.random() * 5) * Math.max(0.7, k),
        rot: Math.random() * 6,
        vr: (Math.random() - 0.5) * 14,
        life: 1.8 + Math.random() * 1.2,
        t: 0,
        color: colors[i % colors.length],
        rect: Math.random() > 0.4,
      })
    }

    let last = performance.now()
    const frame = (now: number) => {
      const dt = Math.min(0.05, (now - last) / 1000)
      last = now
      clear()
      ctx.setTransform(sx, 0, 0, sy, 0, 0)
      piecesRef.current = piecesRef.current.filter((p) => (p.t += dt) < p.life)
      for (const p of piecesRef.current) {
        p.vy += 1500 * k * dt
        p.vx *= 0.992
        p.x += p.vx * dt
        p.y += p.vy * dt
        p.rot += p.vr * dt
        ctx.save()
        ctx.translate(p.x, p.y)
        ctx.rotate(p.rot)
        ctx.globalAlpha = Math.min(1, (p.life - p.t) * 1.5)
        ctx.fillStyle = p.color
        if (p.rect) {
          ctx.fillRect(-p.r, -p.r * 0.5, p.r * 2, p.r)
        } else {
          ctx.beginPath()
          ctx.arc(0, 0, p.r * 0.6, 0, Math.PI * 2)
          ctx.fill()
        }
        ctx.restore()
      }
      if (piecesRef.current.length > 0) {
        rafRef.current = requestAnimationFrame(frame)
      } else {
        clear()
      }
    }
    cancelAnimationFrame(rafRef.current)
    rafRef.current = requestAnimationFrame(frame)
    return () => {
      // Оборвали на полпути (уход со страницы, новый салют): чистим канву и частицы, иначе кадр так и замёрзнет.
      cancelAnimationFrame(rafRef.current)
      piecesRef.current = []
      clear()
    }
  }, [burstId])

  return (
    <canvas
      ref={ref}
      className="pointer-events-none absolute -left-[12%] -top-[12%] z-[35] h-[124%] w-[124%]"
      aria-hidden
    />
  )
}

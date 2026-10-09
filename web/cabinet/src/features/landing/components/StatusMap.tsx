import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { geoNaturalEarth1, geoPath } from 'd3-geo'
import { feature } from 'topojson-client'
import type { FeatureCollection } from 'geojson'
import world from 'world-atlas/countries-110m.json'

import { COUNTRY_CENTROIDS, countryCentroid } from '../countryCentroids'

export interface StatusProbeHit {
  city?: string
  network?: string
  country?: string
  lat?: number
  lon?: number
  ping_ms?: number
  ok?: boolean
  kind?: string
  answer?: string
}

export interface StatusProbe {
  world_ping_ms?: number
  world_ok: number
  world_total: number
  russia_ok: number
  russia_total: number
  history?: Array<number | null>
  measured_at?: string
  hits?: StatusProbeHit[]
}

export interface StatusMapNode {
  name: string
  country?: string
  note?: string
  whitelist?: boolean
  state: string
  probe?: StatusProbe
}

type DotState = 'up' | 'partial' | 'down'

const WIDTH = 960
const HEIGHT = 520

function worst(a: DotState, b: DotState): DotState {
  const rank = { down: 0, partial: 1, up: 2 }
  return rank[a] <= rank[b] ? a : b
}

function dotState(node: StatusMapNode): DotState {
  const panel: DotState = node.state === 'up' || node.state === 'partial' || node.state === 'down' ? node.state : 'down'
  const probe = node.probe
  if (!probe || (probe.world_total === 0 && probe.russia_total === 0)) return panel
  const worldFail = probe.world_total > 0 && probe.world_ok === 0
  const russiaFail = probe.russia_total > 0 && probe.russia_ok === 0
  const worldPart = probe.world_ok > 0 && probe.world_ok < probe.world_total
  const russiaPart = probe.russia_ok > 0 && probe.russia_ok < probe.russia_total
  if (worldFail && russiaFail) return 'down'
  if (worldFail || russiaFail || worldPart || russiaPart) return 'partial'
  return panel === 'down' ? 'down' : 'up'
}

const FILL = { up: '#3dd68c', partial: '#f5b942', down: '#f07178' }

interface View {
  x: number
  y: number
  k: number
}

interface Frame {
  minX: number
  maxX: number
  minY: number
  maxY: number
  minK: number
}

/** Граница самой карты. Дальше неё — пустое поле, туда не отдаляемся и не уезжаем. */
function projectFrame(pathOf: (object: { type: 'Sphere' }) => [[number, number], [number, number]] | null): Frame {
  const bounds = pathOf({ type: 'Sphere' })
  if (!bounds) return { minX: 0, maxX: WIDTH, minY: 0, maxY: HEIGHT, minK: 1 }
  return { minX: bounds[0][0], maxX: bounds[1][0], minY: bounds[0][1], maxY: bounds[1][1], minK: 1 }
}

function clampView(next: View, frame: Frame): View {
  const k = Math.min(8, Math.max(1, next.k))
  const spanX = (frame.maxX - frame.minX) * k
  const spanY = (frame.maxY - frame.minY) * k
  const x =
    spanX <= WIDTH
      ? WIDTH / 2 - ((frame.minX + frame.maxX) / 2) * k
      : Math.min(-frame.minX * k, Math.max(WIDTH - frame.maxX * k, next.x))
  const y =
    spanY <= HEIGHT
      ? HEIGHT / 2 - ((frame.minY + frame.maxY) / 2) * k
      : Math.min(-frame.minY * k, Math.max(HEIGHT - frame.maxY * k, next.y))
  return { k, x, y }
}

/** Стартовый кадр: Европа у левого края, Япония у правого. Отдаление по-прежнему показывает весь мир. */
function openingView(project: (coord: [number, number]) => [number, number] | null, frame: Frame): View {
  const left = project([-18, 46])
  const right = project([148, 36])
  if (!left || !right || right[0] <= left[0]) return clampView({ k: 1, x: 0, y: 0 }, frame)
  const k = (0.94 * WIDTH) / (right[0] - left[0])
  return clampView(
    {
      k,
      x: 0.02 * WIDTH - left[0] * k,
      y: 0.5 * HEIGHT - left[1] * k,
    },
    frame,
  )
}

interface Anchor {
  code: string
  label: string
  ax: number
  ay: number
}

interface LabelBox extends Anchor {
  x: number
  y: number
  w: number
  h: number
  flag: boolean
  kneeX: number
  kneeY: number
  shelfX: number
}

interface Pt {
  x: number
  y: number
}

const LABEL_H = 16

function labelWidth(label: string) {
  return label.length * 7.2 + 22
}

function labelsOverlap(
  a: { x: number; y: number; w: number; h: number },
  b: { x: number; y: number; w: number; h: number },
) {
  const pad = 8
  return a.x < b.x + b.w + pad && a.x + a.w + pad > b.x && a.y < b.y + b.h + pad && a.y + a.h + pad > b.y
}

function coversDot(box: { x: number; y: number; w: number; h: number }, x: number, y: number) {
  return x >= box.x - 8 && x <= box.x + box.w + 8 && y >= box.y - 8 && y <= box.y + box.h + 10
}

function orient(a: Pt, b: Pt, c: Pt) {
  return (b.x - a.x) * (c.y - a.y) - (b.y - a.y) * (c.x - a.x)
}

function segmentsCross(a: Pt, b: Pt, c: Pt, d: Pt) {
  return orient(a, b, c) * orient(a, b, d) < 0 && orient(c, d, a) * orient(c, d, b) < 0
}

function pointSegDist(px: number, py: number, a: Pt, b: Pt) {
  const dx = b.x - a.x
  const dy = b.y - a.y
  const len2 = dx * dx + dy * dy || 1
  const t = Math.max(0, Math.min(1, ((px - a.x) * dx + (py - a.y) * dy) / len2))
  return Math.hypot(px - (a.x + t * dx), py - (a.y + t * dy))
}

/** Полка под надписью и наклонная под 45° к точке. side < 0 — подпись слева. */
function buildCallout(anchor: Anchor, side: -1 | 1, dy: number): LabelBox | null {
  const w = labelWidth(anchor.label)
  const h = LABEL_H
  const lift = 18
  const kneeX = anchor.ax + side * lift
  const kneeY = anchor.ay - lift + dy
  const x = side < 0 ? kneeX - w : kneeX
  const y = kneeY - h - 2
  if (x < 4 || y < 4 || x + w > WIDTH - 4 || y + h > HEIGHT - 6) return null
  if (y < 34 && x + w > WIDTH - 440) return null
  return { ...anchor, x, y, w, h, flag: true, kneeX, kneeY, shelfX: side < 0 ? x : x + w }
}

function leaderEnds(box: LabelBox) {
  const dx = box.ax - box.kneeX
  const dy = box.ay - box.kneeY
  const len = Math.hypot(dx, dy) || 1
  const gap = 8
  return {
    knee: { x: box.kneeX, y: box.kneeY },
    tip: { x: box.ax - (dx / len) * gap, y: box.ay - (dy / len) * gap },
    shelf: { x: box.shelfX, y: box.kneeY },
  }
}

function leaderPath(box: LabelBox) {
  const { knee, tip, shelf } = leaderEnds(box)
  if (Math.hypot(tip.x - knee.x, tip.y - knee.y) < 6) return null
  return `M ${shelf.x} ${shelf.y} L ${knee.x} ${knee.y} L ${tip.x} ${tip.y}`
}

function segmentHitsBox(a: Pt, b: Pt, box: LabelBox) {
  const rect = { x: box.x - 2, y: box.y - 2, w: box.w + 4, h: box.h + 4 }
  const inside = (p: Pt) => p.x >= rect.x && p.x <= rect.x + rect.w && p.y >= rect.y && p.y <= rect.y + rect.h
  if (inside(a) || inside(b)) return true
  const corners = [
    { x: rect.x, y: rect.y },
    { x: rect.x + rect.w, y: rect.y },
    { x: rect.x + rect.w, y: rect.y + rect.h },
    { x: rect.x, y: rect.y + rect.h },
  ]
  return corners.some((corner, i) => segmentsCross(a, b, corner, corners[(i + 1) % 4]))
}

function calloutFits(box: LabelBox, taken: LabelBox[], dots: Anchor[]) {
  if (taken.some((other) => labelsOverlap(box, other))) return false
  if (dots.some((dot) => dot.code !== box.code && coversDot(box, dot.ax, dot.ay))) return false
  const { knee, tip, shelf } = leaderEnds(box)
  for (const dot of dots) {
    if (dot.code === box.code) continue
    if (pointSegDist(dot.ax, dot.ay, knee, tip) < 8) return false
  }
  for (const other of taken) {
    const prev = leaderEnds(other)
    if (segmentsCross(knee, tip, prev.knee, prev.tip)) return false
    if (segmentsCross(knee, tip, prev.knee, prev.shelf)) return false
    if (segmentsCross(knee, shelf, prev.knee, prev.tip)) return false
    if (segmentHitsBox(knee, tip, other)) return false
  }
  return true
}

function sidesFor(anchor: Anchor, anchors: Anchor[]): Array<-1 | 1> {
  let vx = 0
  for (const other of anchors) {
    if (other.code === anchor.code) continue
    const dx = anchor.ax - other.ax
    const dist = Math.hypot(dx, other.ay - anchor.ay)
    if (dist < 1 || dist > 90) continue
    vx += dx / dist
  }
  const preferred: -1 | 1 = vx === 0 ? (anchor.ax > WIDTH * 0.72 ? -1 : 1) : vx >= 0 ? 1 : -1
  return [preferred, preferred === 1 ? -1 : 1]
}

function placeAnchors(anchors: Anchor[]): LabelBox[] {
  const crowd = (anchor: Anchor) =>
    anchors.reduce((n, other) => n + (other !== anchor && Math.hypot(anchor.ax - other.ax, anchor.ay - other.ay) < 48 ? 1 : 0), 0)
  const order = [...anchors].sort((a, b) => crowd(a) - crowd(b) || a.ay - b.ay || a.code.localeCompare(b.code))
  const boxes: LabelBox[] = []
  const shifts = [0, -24, -48, 24, -72, 48, -96, 72]
  for (const anchor of order) {
    let chosen: LabelBox | null = null
    const sides = sidesFor(anchor, anchors)
    for (const dy of shifts) {
      for (const side of sides) {
        const box = buildCallout(anchor, side, dy)
        if (!box || !calloutFits(box, boxes, anchors)) continue
        chosen = box
        break
      }
      if (chosen) break
    }
    if (!chosen) {
      for (const side of sidesFor(anchor, anchors)) {
        const box = buildCallout(anchor, side, 0)
        if (box) {
          chosen = box
          break
        }
      }
    }
    if (chosen) boxes.push(chosen)
  }
  return boxes
}

function inView(point: [number, number], view: View): boolean {
  const x = point[0] * view.k + view.x
  const y = point[1] * view.k + view.y
  return x >= -12 && x <= WIDTH + 12 && y >= -12 && y <= HEIGHT + 12
}

/** Дуга вдоль хорды. Изгиб ограничен длиной, соседние линии гнутся в разные стороны. */
function arc(from: [number, number], to: [number, number], bend: number): string {
  const dx = to[0] - from[0]
  const dy = to[1] - from[1]
  const len = Math.hypot(dx, dy) || 1
  const off = Math.min(26, len * 0.14) * bend
  const cx = (from[0] + to[0]) / 2 + (-dy / len) * off
  const cy = (from[1] + to[1]) / 2 + (dx / len) * off
  return `M ${from[0]} ${from[1]} Q ${cx} ${cy} ${to[0]} ${to[1]}`
}

const RUSSIA: [number, number] = COUNTRY_CENTROIDS.RU ?? [37.6, 55.75]

/** Карта локаций: её можно двигать и приближать. Линии идут из одной точки в России. */
export function StatusMap({ nodes }: { nodes: StatusMapNode[] }) {
  const { t, i18n } = useTranslation()
  const svgRef = useRef<SVGSVGElement>(null)
  const pointers = useRef(new Map<number, { x: number; y: number }>())
  const pinch = useRef<{ dist: number; view: View } | null>(null)

  const countries = useMemo(() => {
    const topo = world as unknown as { objects: { countries: object } }
    return feature(topo as never, topo.objects.countries as never) as unknown as FeatureCollection
  }, [])

  const projection = useMemo(
    () =>
      geoNaturalEarth1().fitExtent(
        [
          [12, 16],
          [WIDTH - 12, HEIGHT - 16],
        ],
        { type: 'Sphere' },
      ),
    [],
  )

  const frame = useMemo(
    () => projectFrame((object) => geoPath(projection).bounds(object)),
    [projection],
  )

  const initial = useMemo<View>(() => openingView(projection, frame), [projection, frame])

  const [view, setView] = useState<View>(initial)
  const viewRef = useRef(view)
  viewRef.current = view

  useEffect(() => {
    const svg = svgRef.current
    if (!svg) return
    const onWheel = (event: WheelEvent) => {
      event.preventDefault()
      const rect = svg.getBoundingClientRect()
      const px = ((event.clientX - rect.left) / rect.width) * WIDTH
      const py = ((event.clientY - rect.top) / rect.height) * HEIGHT
      setView((prev) => {
        const nextK = prev.k * (event.deltaY < 0 ? 1.12 : 0.9)
        const ratio = nextK / prev.k
        return clampView({ k: nextK, x: px - (px - prev.x) * ratio, y: py - (py - prev.y) * ratio }, frame)
      })
    }
    svg.addEventListener('wheel', onWheel, { passive: false })
    return () => svg.removeEventListener('wheel', onWheel)
  }, [frame])

  const placed = useMemo(() => {
    const byCountry = new Map<string, { code: string; state: DotState }>()
    for (const node of nodes) {
      const code = (node.country ?? '').toUpperCase()
      if (!countryCentroid(code)) continue
      const prev = byCountry.get(code)
      const state = dotState(node)
      if (!prev) byCountry.set(code, { code, state })
      else byCountry.set(code, { code, state: worst(prev.state, state) })
    }
    return [...byCountry.values()]
  }, [nodes])

  const links = useMemo(
    () => placed.filter((item) => item.code !== 'RU').map((item) => item.code),
    [placed],
  )

  const path = geoPath(projection)

  const regionName = (code: string) => {
    try {
      return new Intl.DisplayNames([i18n.language], { type: 'region' }).of(code) ?? code
    } catch {
      return code
    }
  }

  const labelBoxes = useMemo(() => {
    const anchors: Array<{ code: string; label: string; ax: number; ay: number }> = []
    const ru = projection(RUSSIA)
    if (ru && inView(ru, view)) {
      anchors.push({
        code: 'RU',
        label: regionName('RU'),
        ax: ru[0] * view.k + view.x,
        ay: ru[1] * view.k + view.y,
      })
    }
    for (const item of placed) {
      if (item.code === 'RU') continue
      const raw = projection(countryCentroid(item.code)!)
      if (!raw || !inView(raw, view)) continue
      anchors.push({
        code: item.code,
        label: regionName(item.code),
        ax: raw[0] * view.k + view.x,
        ay: raw[1] * view.k + view.y,
      })
    }
    return placeAnchors(anchors)
  }, [placed, projection, view, i18n.language])

  function toLocal(event: React.PointerEvent) {
    const rect = svgRef.current?.getBoundingClientRect()
    if (!rect) return { x: 0, y: 0 }
    return {
      x: ((event.clientX - rect.left) / rect.width) * WIDTH,
      y: ((event.clientY - rect.top) / rect.height) * HEIGHT,
    }
  }

  function onPointerDown(event: React.PointerEvent<SVGSVGElement>) {
    svgRef.current?.setPointerCapture(event.pointerId)
    pointers.current.set(event.pointerId, toLocal(event))
    pinch.current = null
  }

  function onPointerMove(event: React.PointerEvent<SVGSVGElement>) {
    const prevPoint = pointers.current.get(event.pointerId)
    if (!prevPoint) return
    const point = toLocal(event)
    pointers.current.set(event.pointerId, point)
    const pts = [...pointers.current.values()]
    if (pts.length >= 2) {
      const dist = Math.hypot(pts[0].x - pts[1].x, pts[0].y - pts[1].y)
      if (!pinch.current) pinch.current = { dist, view: viewRef.current }
      const midX = (pts[0].x + pts[1].x) / 2
      const midY = (pts[0].y + pts[1].y) / 2
      const base = pinch.current
      const nextK = base.view.k * (dist / (base.dist || 1))
      const ratio = nextK / base.view.k
      setView(clampView({ k: nextK, x: midX - (midX - base.view.x) * ratio, y: midY - (midY - base.view.y) * ratio }, frame))
      return
    }
    pinch.current = null
    setView((current) =>
      clampView(
        {
          ...current,
          x: current.x + point.x - prevPoint.x,
          y: current.y + point.y - prevPoint.y,
        },
        frame,
      ),
    )
  }

  function onPointerUp(event: React.PointerEvent<SVGSVGElement>) {
    pointers.current.delete(event.pointerId)
    pinch.current = null
  }

  return (
    <div className="landing-status-map mt-8">
      <div className="landing-status-map__stage">
      <svg
        ref={svgRef}
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        role="img"
        aria-label={t('landing.status.mapLabel')}
        className="landing-status-map__canvas"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
      >
        <rect width={WIDTH} height={HEIGHT} fill="#0c1428" />
        <g transform={`translate(${view.x} ${view.y}) scale(${view.k})`}>
          {countries.features.map((shape, i) => (
            <path key={i} d={path(shape) ?? ''} className="landing-status-map__land" />
          ))}
          {links.map((code, index) => {
            const from = projection(RUSSIA)
            const to = projection(countryCentroid(code)!)
            if (!from || !to) return null
            return (
              <path
                key={code}
                className="landing-status-map__line"
                d={arc(from, to, index % 2 === 0 ? 1 : -1)}
              />
            )
          })}
          {projection(RUSSIA) && inView(projection(RUSSIA)!, view) && (
            <>
              <circle
                className="landing-status-map__pulse"
                cx={projection(RUSSIA)![0]}
                cy={projection(RUSSIA)![1]}
                r={8 / view.k}
                fill="none"
                stroke="#7eb6ff"
                strokeWidth={1.4 / view.k}
              />
              <rect
                x={projection(RUSSIA)![0] - 5 / view.k}
                y={projection(RUSSIA)![1] - 5 / view.k}
                width={10 / view.k}
                height={10 / view.k}
                rx={2 / view.k}
                fill="#7eb6ff"
                stroke="#04101f"
                strokeWidth={1.4 / view.k}
              />
            </>
          )}
          {placed.map((item, index) => {
            const point = projection(countryCentroid(item.code)!)
            if (!point || !inView(point, view)) return null
            return (
              <g key={item.code}>
                <circle
                  className="landing-status-map__pulse"
                  cx={point[0]}
                  cy={point[1]}
                  r={7 / view.k}
                  fill="none"
                  stroke={FILL[item.state]}
                  strokeWidth={1.5 / view.k}
                  style={{ animationDelay: `${(index % 6) * 0.35}s` }}
                />
                <circle
                  className="landing-status-map__dot"
                  cx={point[0]}
                  cy={point[1]}
                  r={6 / view.k}
                  fill={FILL[item.state]}
                  stroke="#04101f"
                  strokeWidth={1.6 / view.k}
                  style={{ animationDelay: `${(index % 6) * 0.08}s` }}
                />
              </g>
            )
          })}
        </g>
        <g className="landing-status-map__leaders" pointerEvents="none">
          {labelBoxes.map((item) => {
            const d = leaderPath(item)
            if (!d) return null
            return <path key={item.code} d={d} />
          })}
        </g>
      </svg>
      <div className="pointer-events-none absolute inset-0">
        {labelBoxes.map((item) => (
          <span
            key={item.code}
            className="landing-status-map__tag absolute flex items-center gap-1 whitespace-nowrap text-[12px] font-semibold text-white"
            style={{
              left: `${(item.x / WIDTH) * 100}%`,
              top: `${(item.y / HEIGHT) * 100}%`,
              textShadow: '0 1px 2px #04101f, 0 0 6px #04101f',
            }}
          >
            {item.flag && (
              <img
                src={`${import.meta.env.BASE_URL}flags/${item.code.toLowerCase()}.svg`}
                alt=""
                className="h-2.5 w-3.5 rounded-[2px]"
              />
            )}
            {item.label}
          </span>
        ))}
      </div>
      </div>

      <ul className="absolute right-3 top-3 hidden flex-wrap justify-end gap-x-3 gap-y-1 text-xs text-slate-200 sm:flex sm:max-w-[28rem]">
        <li className="inline-flex items-center gap-1.5">
          <span className="landing-status-dot landing-status-dot--up" />
          {t('landing.status.mapLegendUp')}
        </li>
        <li className="inline-flex items-center gap-1.5">
          <span className="landing-status-dot landing-status-dot--partial" />
          {t('landing.status.mapLegendPartial')}
        </li>
        <li className="inline-flex items-center gap-1.5">
          <span className="landing-status-dot landing-status-dot--down" />
          {t('landing.status.mapLegendDown')}
        </li>
        <li className="inline-flex items-center gap-1.5">
          <span className="landing-status-dot landing-status-dot--ru" />
          {t('landing.status.mapLegendRu')}
        </li>
      </ul>
    </div>
  )
}

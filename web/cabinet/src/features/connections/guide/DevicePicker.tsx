import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import { createPortal } from 'react-dom'
import { X } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { AppTile, PlatformIcon, platformLabel } from './glyphs'
import type { GuideText } from './guideText'
import type { AppGuide, PlatformKey } from './types'

type Props = {
  open: boolean
  onClose: () => void
  text: GuideText
  platforms: PlatformKey[]
  platform: PlatformKey
  onPlatform: (p: PlatformKey) => void
  apps: AppGuide[]
  appId: string
  onApp: (id: string) => void
}

/** Длительность выезда/ухода — совпадает с transition в connections-guide.css. */
const ANIM_MS = 340

/**
 * Окно «Изменить»: устройство и приложение. На телефоне — шторка, выезжает
 * снизу и уходит вниз (крестиком, тапом по фону или свайпом за полоску или
 * заголовок). На ПК — модалка по центру с плавным появлением.
 *
 * Через портал: страница обёрнута в PageReveal с transform, и fixed внутри
 * него считался бы от карточки, а не от экрана.
 */
export function DevicePicker({ open, onClose, text, platforms, platform, onPlatform, apps, appId, onApp }: Props) {
  // mounted держит разметку на время анимации ухода, shown включает сам переход.
  const [mounted, setMounted] = useState(open)
  const [shown, setShown] = useState(false)
  const sheetRef = useRef<HTMLDivElement>(null)
  const backdropRef = useRef<HTMLDivElement>(null)
  const drag = useRef<{ startY: number; lastY: number; lastT: number; v: number } | null>(null)

  useEffect(() => {
    if (open) {
      setMounted(true)
      // Открыли снова, пока шторка ещё уезжала после свайпа, — снимаем её позу.
      if (sheetRef.current) sheetRef.current.style.transform = ''
      if (backdropRef.current) backdropRef.current.style.opacity = ''
      // Два кадра: сначала браузер рисует шторку за краем экрана, потом едет вверх.
      let inner = 0
      const outer = requestAnimationFrame(() => {
        inner = requestAnimationFrame(() => setShown(true))
      })
      return () => {
        cancelAnimationFrame(outer)
        cancelAnimationFrame(inner)
      }
    }
    setShown(false)
    const t = window.setTimeout(() => setMounted(false), ANIM_MS)
    return () => window.clearTimeout(t)
  }, [open])

  useEffect(() => {
    if (!open) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    document.addEventListener('keydown', onKey)
    return () => {
      document.body.style.overflow = prevOverflow
      document.removeEventListener('keydown', onKey)
    }
  }, [open, onClose])

  /* ── Свайп вниз (только шторка на телефоне) ─────────────────────── */

  function onDragStart(e: ReactPointerEvent<HTMLDivElement>) {
    if (window.matchMedia('(min-width: 768px)').matches) return
    if ((e.target as HTMLElement).closest('button')) return
    drag.current = { startY: e.clientY, lastY: e.clientY, lastT: e.timeStamp, v: 0 }
    sheetRef.current?.classList.add('cg-sheet--drag')
    e.currentTarget.setPointerCapture(e.pointerId)
  }

  function onDragMove(e: ReactPointerEvent<HTMLDivElement>) {
    const d = drag.current
    const sheet = sheetRef.current
    if (!d || !sheet) return
    const dy = Math.max(0, e.clientY - d.startY)
    d.v = (e.clientY - d.lastY) / Math.max(1, e.timeStamp - d.lastT)
    d.lastY = e.clientY
    d.lastT = e.timeStamp
    sheet.style.transform = `translateY(${dy}px)`
    if (backdropRef.current) backdropRef.current.style.opacity = String(Math.max(0, 1 - dy / sheet.offsetHeight))
  }

  function onDragEnd(e: ReactPointerEvent<HTMLDivElement>) {
    const d = drag.current
    const sheet = sheetRef.current
    if (!d || !sheet) return
    drag.current = null
    const dy = Math.max(0, e.clientY - d.startY)
    sheet.classList.remove('cg-sheet--drag')
    // Утянули дальше трети высоты или резко смахнули — закрываем, иначе шторка
    // возвращается на место. Переход доезжает от текущего положения пальца.
    if (dy > sheet.offsetHeight / 3 || d.v > 0.6) {
      // Сразу отправляем вниз от текущего положения, не дожидаясь перерисовки:
      // иначе на кадр шторка дёрнулась бы вверх и только потом уехала.
      sheet.style.transform = 'translateY(105%)'
      if (backdropRef.current) backdropRef.current.style.opacity = '0'
      onClose()
      return
    }
    sheet.style.transform = ''
    if (backdropRef.current) backdropRef.current.style.opacity = ''
  }

  const dragHandlers = {
    onPointerDown: onDragStart,
    onPointerMove: onDragMove,
    onPointerUp: onDragEnd,
    onPointerCancel: onDragEnd,
  }

  if (!mounted) return null

  return createPortal(
    <div className="fixed inset-0 z-[130] flex items-end justify-center md:items-center md:p-4" role="presentation">
      <div
        ref={backdropRef}
        className="cg-backdrop absolute inset-0 bg-black/55 backdrop-blur-[2px]"
        data-shown={shown}
        onClick={onClose}
        aria-hidden
      />
      <div
        ref={sheetRef}
        role="dialog"
        aria-modal="true"
        aria-label={text.pickerTitle}
        data-shown={shown}
        className={cn(
          'cg-sheet relative flex max-h-[90dvh] w-full flex-col gap-3 overflow-y-auto border border-border bg-card px-4 pb-6 text-card-foreground shadow-2xl',
          'rounded-t-3xl md:max-w-[460px] md:rounded-3xl md:pb-4 md:pt-4',
          'dark:border-white/10 dark:bg-[#141d2e]',
        )}
      >
        {/* Полоска и заголовок — за них шторку тянут вниз */}
        <div className="-mx-4 flex cursor-grab touch-none justify-center pb-1 pt-2.5 md:hidden" {...dragHandlers}>
          <span className="h-1.5 w-10 rounded-full bg-foreground/25" aria-hidden />
        </div>
        <div className="flex touch-none items-center justify-between gap-2 md:touch-auto" {...dragHandlers}>
          <h3 className="text-[17px] font-bold">{text.pickerTitle}</h3>
          <button
            type="button"
            onClick={onClose}
            aria-label={text.close}
            className="grid size-8 place-items-center rounded-lg text-muted-foreground hover:bg-foreground/[0.06] hover:text-foreground"
          >
            <X size={16} />
          </button>
        </div>

        <div className="grid grid-cols-3 gap-2">
          {platforms.map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => onPlatform(p)}
              aria-pressed={p === platform}
              className={cn(
                'flex flex-col items-center gap-1.5 rounded-2xl border px-1.5 py-3 text-[13px] font-semibold transition-colors',
                p === platform
                  ? 'border-primary bg-primary/[0.12] text-primary'
                  : 'border-border bg-foreground/[0.04] text-foreground hover:border-primary/50 dark:border-white/10',
              )}
            >
              <PlatformIcon platform={p} size={24} />
              <span className="truncate">{platformLabel[p] || p}</span>
            </button>
          ))}
        </div>

        <p className="mt-1 text-xs font-bold uppercase tracking-wide text-muted-foreground">{text.pickerApp}</p>
        <div className="flex flex-col gap-2">
          {apps.map((app) => (
            <button
              key={app.id}
              type="button"
              onClick={() => onApp(app.id)}
              aria-pressed={app.id === appId}
              className={cn(
                'flex items-center gap-3 rounded-2xl border px-3 py-2.5 text-left transition-colors',
                app.id === appId
                  ? 'border-primary bg-primary/10'
                  : 'border-border bg-foreground/[0.04] hover:border-primary/50 dark:border-white/10',
              )}
            >
              <AppTile app={app} className="size-[34px] text-[11px]" />
              <b className="min-w-0 truncate text-[15px] font-semibold">{app.name}</b>
              {app.isFeatured ? (
                <span className="ml-auto whitespace-nowrap rounded-full bg-amber-400/15 px-2 py-0.5 text-[11px] font-bold text-amber-700 dark:text-amber-300">
                  ★ {text.recommended}
                </span>
              ) : null}
            </button>
          ))}
        </div>

        <Button type="button" className="mt-1 h-11 w-full" onClick={onClose}>
          {text.pickerDone}
        </Button>
      </div>
    </div>,
    document.body,
  )
}

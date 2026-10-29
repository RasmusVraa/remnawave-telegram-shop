import { useEffect, useRef, type PointerEvent as ReactPointerEvent } from 'react'

/** Сколько едет шторка вниз после отпускания, прежде чем окно закроется. */
const DISMISS_MS = 220

type Options = {
  open: boolean
  onDismiss: () => void
  /**
   * С какой ширины окно перестаёт быть шторкой и становится диалогом по
   * центру — там свайп не нужен. У большинства окон это sm, у части — md.
   */
  desktopQuery?: string
  /** Временный запрет: например, пока отправляется форма. */
  disabled?: boolean
}

/**
 * Свайп вниз закрывает шторку на телефоне — как системные листы iOS и Android.
 *
 * Тянут за полоску-ручку и заголовок: на них вешаются `handlers` (и класс
 * `touch-none`, чтобы браузер не забирал жест под прокрутку). Тело окна не
 * трогаем — там свой скролл, и жест конфликтовал бы с ним.
 *
 * Пока палец на экране, шторка идёт за ним без transition. Отпустили дальше
 * трети высоты или резко смахнули — шторка доезжает вниз, и только потом
 * вызывается onDismiss: многие окна снимаются с экрана мгновенно, и без этой
 * паузы исчезали бы из середины жеста.
 */
export function useSheetDragDismiss<
  S extends HTMLElement = HTMLDivElement,
  B extends HTMLElement = HTMLElement,
>({ open, onDismiss, desktopQuery = '(min-width: 640px)', disabled }: Options) {
  const sheetRef = useRef<S>(null)
  /** Необязательный отдельный фон: он гаснет по мере того, как шторку тянут. */
  const backdropRef = useRef<B>(null)
  const drag = useRef<{ startY: number; lastY: number; lastT: number; v: number } | null>(null)
  const timer = useRef<number | undefined>(undefined)
  const onDismissRef = useRef(onDismiss)
  onDismissRef.current = onDismiss

  // Окно открыли снова, пока оно ещё не размонтировалось после свайпа, —
  // снимаем позу, оставшуюся от жеста.
  useEffect(() => {
    if (!open) return
    window.clearTimeout(timer.current)
    resetInline(sheetRef.current, backdropRef.current)
  }, [open])

  useEffect(() => () => window.clearTimeout(timer.current), [])

  function onPointerDown(e: ReactPointerEvent<HTMLElement>) {
    if (disabled || window.matchMedia(desktopQuery).matches) return
    if ((e.target as HTMLElement).closest('button, a, input, textarea, select, [role="button"]')) return
    const sheet = sheetRef.current
    if (!sheet) return
    drag.current = { startY: e.clientY, lastY: e.clientY, lastT: e.timeStamp, v: 0 }
    sheet.style.transition = 'none'
    if (backdropRef.current) backdropRef.current.style.transition = 'none'
    e.currentTarget.setPointerCapture(e.pointerId)
  }

  function onPointerMove(e: ReactPointerEvent<HTMLElement>) {
    const d = drag.current
    const sheet = sheetRef.current
    if (!d || !sheet) return
    const dy = Math.max(0, e.clientY - d.startY)
    d.v = (e.clientY - d.lastY) / Math.max(1, e.timeStamp - d.lastT)
    d.lastY = e.clientY
    d.lastT = e.timeStamp
    sheet.style.transform = `translateY(${dy}px)`
    if (backdropRef.current) {
      backdropRef.current.style.opacity = String(Math.max(0, 1 - dy / sheet.offsetHeight))
    }
  }

  function onPointerUp(e: ReactPointerEvent<HTMLElement>) {
    const d = drag.current
    const sheet = sheetRef.current
    if (!d || !sheet) return
    drag.current = null
    const dy = Math.max(0, e.clientY - d.startY)
    const ease = `${DISMISS_MS}ms cubic-bezier(0.2, 0.8, 0.2, 1)`
    sheet.style.transition = `transform ${ease}`
    if (backdropRef.current) backdropRef.current.style.transition = `opacity ${ease}`

    if (dy > sheet.offsetHeight / 3 || d.v > 0.6) {
      sheet.style.transform = 'translateY(105%)'
      if (backdropRef.current) backdropRef.current.style.opacity = '0'
      timer.current = window.setTimeout(() => onDismissRef.current(), DISMISS_MS)
      return
    }
    // Не дотянули — возвращаем на место, а затем отдаём управление классам
    // окна, чтобы его собственная анимация закрытия работала как раньше.
    sheet.style.transform = 'translateY(0px)'
    if (backdropRef.current) backdropRef.current.style.opacity = ''
    timer.current = window.setTimeout(() => resetInline(sheetRef.current, backdropRef.current), DISMISS_MS)
  }

  return {
    sheetRef,
    backdropRef,
    handlers: {
      onPointerDown,
      onPointerMove,
      onPointerUp,
      onPointerCancel: onPointerUp,
    },
  }
}

function resetInline(sheet: HTMLElement | null, backdrop: HTMLElement | null) {
  if (sheet) {
    sheet.style.transform = ''
    sheet.style.transition = ''
  }
  if (backdrop) {
    backdrop.style.opacity = ''
    backdrop.style.transition = ''
  }
}

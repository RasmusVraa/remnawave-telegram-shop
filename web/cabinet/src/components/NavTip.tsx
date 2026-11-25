import { type FocusEvent, type MouseEvent, type ReactNode, useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'

interface Tip {
  label: string
  x: number
  y: number
}

const SHOW_DELAY_MS = 200

/**
 * Подсказка под иконкой шапки: рисуется порталом поверх страницы, поэтому
 * не двигает соседние иконки и не обрезается overflow у <nav>.
 */
export function useNavTip(): {
  bind: (label: string) => {
    onMouseEnter: (e: MouseEvent<HTMLElement>) => void
    onMouseLeave: () => void
    onFocus: (e: FocusEvent<HTMLElement>) => void
    onBlur: () => void
    onClick: () => void
  }
  node: ReactNode
} {
  const [tip, setTip] = useState<Tip | null>(null)
  const timer = useRef<number | undefined>(undefined)

  const hide = useCallback(() => {
    window.clearTimeout(timer.current)
    setTip(null)
  }, [])

  const show = useCallback((label: string, el: HTMLElement) => {
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => {
      const r = el.getBoundingClientRect()
      setTip({ label, x: r.left + r.width / 2, y: r.bottom })
    }, SHOW_DELAY_MS)
  }, [])

  useEffect(() => () => window.clearTimeout(timer.current), [])

  const bind = useCallback(
    (label: string) => ({
      onMouseEnter: (e: MouseEvent<HTMLElement>) => show(label, e.currentTarget),
      onMouseLeave: hide,
      onFocus: (e: FocusEvent<HTMLElement>) => {
        // Подсказку по фокусу — только при навигации с клавиатуры.
        if (e.currentTarget.matches(':focus-visible')) show(label, e.currentTarget)
      },
      onBlur: hide,
      onClick: hide,
    }),
    [show, hide],
  )

  const node = tip
    ? createPortal(
        <div
          role="tooltip"
          className="pointer-events-none fixed z-[60] -translate-x-1/2"
          style={{ left: tip.x, top: tip.y + 8 }}
        >
          <div className="relative animate-nav-tip-in whitespace-nowrap rounded-lg border border-border bg-background px-2.5 py-1 text-xs font-medium text-foreground shadow-lg">
            <span
              aria-hidden
              className="absolute -top-[5px] left-1/2 size-2 -translate-x-1/2 rotate-45 border-l border-t border-border bg-background"
            />
            {tip.label}
          </div>
        </div>,
        document.body,
      )
    : null

  return { bind, node }
}

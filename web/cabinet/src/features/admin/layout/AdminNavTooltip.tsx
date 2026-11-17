import { useCallback, useState, type FocusEvent, type MouseEvent } from 'react'
import { createPortal } from 'react-dom'

interface TipState {
  label: string
  hint?: string
  top: number
  left: number
}

type TipTarget = MouseEvent<HTMLElement> | FocusEvent<HTMLElement>

/**
 * Подпись справа от иконки свёрнутого меню. Через портал: меню прокручивается
 * (`overflow-y-auto`) и обрезало бы всё, что торчит за его край.
 */
export function useAdminNavTooltip(enabled: boolean) {
  const [tip, setTip] = useState<TipState | null>(null)

  const hide = useCallback(() => setTip(null), [])

  const bind = useCallback(
    (label: string, hint?: string) => {
      if (!enabled) return {}
      const show = (e: TipTarget) => {
        const r = e.currentTarget.getBoundingClientRect()
        setTip({ label, hint, top: r.top + r.height / 2, left: r.right + 12 })
      }
      return { onMouseEnter: show, onMouseLeave: hide, onFocus: show, onBlur: hide }
    },
    [enabled, hide],
  )

  const node =
    enabled && tip
      ? createPortal(
          <div
            role="tooltip"
            style={{ position: 'fixed', top: tip.top, left: tip.left, zIndex: 9999 }}
            className="pointer-events-none -translate-y-1/2 whitespace-nowrap rounded-lg border border-border/60 bg-card px-2.5 py-1.5 text-sm font-medium text-foreground shadow-xl"
          >
            {tip.label}
            {tip.hint && <span className="ml-2 text-xs font-normal text-muted-foreground">{tip.hint}</span>}
          </div>,
          document.body,
        )
      : null

  return { bind, hide, node }
}

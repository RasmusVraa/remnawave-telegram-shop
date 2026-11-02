import { type ReactNode, useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { X } from 'lucide-react'

import { useSheetDragDismiss } from '@/hooks/useSheetDragDismiss'
import { cn } from '@/lib/utils'

/** Длительность появления и ухода; из неё же считается задержка размонтирования. */
const ANIM_MS = 250

/**
 * Шторка снизу на телефоне и модалка по центру на sm+. Высота — по содержимому,
 * не больше экрана минус шапка; длинное содержимое прокручивается внутри.
 *
 * Тот же приём, что у ConnectInviteModal: mounted держит разметку, пока идёт
 * уход, visible переключает классы перехода.
 */
export function ResponsiveSheet({
  open,
  onClose,
  title,
  subtitle,
  actions,
  children,
  className,
}: {
  open: boolean
  onClose: () => void
  title: ReactNode
  subtitle?: ReactNode
  /** Кнопки в шапке слева от крестика. */
  actions?: ReactNode
  children: ReactNode
  className?: string
}) {
  const { t } = useTranslation()
  const [mounted, setMounted] = useState(open)
  const [visible, setVisible] = useState(false)
  const { sheetRef, handlers: dragHandlers } = useSheetDragDismiss({ open, onDismiss: onClose })

  useEffect(() => {
    if (open) {
      setMounted(true)
      // Два кадра: в первом разметка только вставляется, и если сразу
      // переключить классы, браузер посчитает стили один раз — уже конечные —
      // и выезда снизу не будет.
      let inner = 0
      const outer = requestAnimationFrame(() => {
        inner = requestAnimationFrame(() => setVisible(true))
      })
      return () => {
        cancelAnimationFrame(outer)
        cancelAnimationFrame(inner)
      }
    }
    setVisible(false)
    const id = window.setTimeout(() => setMounted(false), ANIM_MS + 30)
    return () => window.clearTimeout(id)
  }, [open])

  useEffect(() => {
    if (!open) return
    function onEsc(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onEsc)
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onEsc)
      document.body.style.overflow = prev
    }
  }, [open, onClose])

  if (typeof document === 'undefined' || !mounted) return null

  return createPortal(
    <div
      className={cn(
        'fixed inset-0 z-[2000] flex items-end justify-center bg-black/60 backdrop-blur-sm transition-opacity duration-[250ms] ease-out sm:items-start sm:px-4 sm:pt-[7.5rem]',
        visible ? 'opacity-100' : 'opacity-0',
      )}
      onClick={onClose}
    >
      <div
        ref={sheetRef}
        role="dialog"
        aria-modal="true"
        aria-label={typeof title === 'string' ? title : undefined}
        onClick={(e) => e.stopPropagation()}
        className={cn(
          'home-sheet flex max-h-[calc(100dvh-3.5rem-var(--cabinet-tg-safe-top))] w-full flex-col overflow-hidden border border-border bg-background shadow-[0_-20px_50px_rgba(0,0,0,0.5)] transition-[opacity,transform] duration-[250ms] ease-out',
          'rounded-t-[28px] sm:max-h-[calc(100dvh-9rem)] sm:max-w-[540px] sm:rounded-[26px] sm:shadow-[0_30px_80px_rgba(0,0,0,0.6)]',
          visible
            ? 'translate-y-0 opacity-100 sm:scale-100'
            : 'translate-y-full opacity-0 sm:translate-y-0 sm:scale-[0.97]',
          className,
        )}
      >
        <div className="flex shrink-0 cursor-grab touch-none justify-center pb-0.5 pt-2.5 sm:hidden" {...dragHandlers}>
          <span className="h-[5px] w-10 rounded-full bg-muted-foreground/40" aria-hidden />
        </div>
        <div
          className="flex shrink-0 touch-none items-center gap-2 px-4 pb-3.5 pt-2.5 sm:touch-auto sm:px-5 sm:pt-3.5"
          {...dragHandlers}
        >
          <div className="min-w-0 flex-1">
            <p className="font-heading text-[22px] font-extrabold leading-tight">{title}</p>
            {subtitle && <p className="mt-0.5 text-[13px] text-muted-foreground">{subtitle}</p>}
          </div>
          {actions}
          <button
            type="button"
            onClick={onClose}
            aria-label={t('common.close')}
            className="inline-flex size-9 shrink-0 items-center justify-center rounded-[10px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <X size={17} />
          </button>
        </div>
        <div className="flex min-h-0 flex-1 flex-col gap-3.5 overflow-y-auto overscroll-contain px-4 pb-[max(1.5rem,calc(1rem+var(--cabinet-sheet-safe-bottom)))] sm:px-5 sm:pb-5">
          {children}
        </div>
      </div>
    </div>,
    document.body,
  )
}

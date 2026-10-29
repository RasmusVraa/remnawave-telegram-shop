import { useEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { AlertTriangle, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useSheetDragDismiss } from '@/hooks/useSheetDragDismiss'
import { cn } from '@/lib/utils'

const ANIM_MS = 250

type ConfirmModalProps = {
  open: boolean
  title: string
  /** Пояснение под заголовком: чем именно закончится действие. */
  description?: ReactNode
  confirmLabel: string
  cancelLabel?: string
  /** danger — красная кнопка подтверждения и красный бейдж иконки. */
  tone?: 'danger' | 'default'
  icon?: LucideIcon
  /** Заголовок на одной линии с иконкой, пояснение — отдельным абзацем под ними. */
  inlineTitle?: boolean
  loading?: boolean
  onConfirm: () => void
  onClose: () => void
}

/**
 * Общая модалка подтверждения кабинета: портал, фейд оверлея, бейдж с иконкой
 * и пара кнопок в один ряд — подтверждение слева, отмена справа. На телефоне
 * это шторка снизу со свайпом вниз, как остальные окна кабинета; с sm — диалог
 * по центру. Заменяет разрозненные inline-диалоги (выход, удаление устройства,
 * отвязка провайдера, уменьшение доп. устройств).
 */
export function ConfirmModal({
  open,
  title,
  description,
  confirmLabel,
  cancelLabel,
  tone = 'danger',
  icon: Icon = AlertTriangle,
  inlineTitle = false,
  loading = false,
  onConfirm,
  onClose,
}: ConfirmModalProps) {
  const { t } = useTranslation()
  const [mounted, setMounted] = useState(open)
  const [visible, setVisible] = useState(false)
  const confirmRef = useRef<HTMLButtonElement>(null)

  const { sheetRef, handlers: dragHandlers } = useSheetDragDismiss({
    open,
    onDismiss: onClose,
    disabled: loading,
  })

  useEffect(() => {
    if (open) {
      setMounted(true)
      // Кадр задержки: без него переход стартует с уже конечных значений.
      const raf = requestAnimationFrame(() => setVisible(true))
      return () => cancelAnimationFrame(raf)
    }
    setVisible(false)
    // По таймеру, а не по transitionend: при «уменьшить движение» событие не
    // приходит, и окно осталось бы в DOM.
    const id = window.setTimeout(() => setMounted(false), ANIM_MS + 30)
    return () => window.clearTimeout(id)
  }, [open])

  useEffect(() => {
    if (!open) return undefined
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape' && !loading) onClose()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [open, loading, onClose])

  useEffect(() => {
    if (visible) confirmRef.current?.focus()
  }, [visible])

  if (!mounted || typeof document === 'undefined') return null

  const danger = tone === 'danger'

  return createPortal(
    <div
      role="presentation"
      className={cn(
        'fixed inset-0 z-[2000] flex items-end justify-center bg-black/60 backdrop-blur-sm transition-opacity duration-[250ms] ease-out sm:items-center sm:p-4',
        visible ? 'opacity-100' : 'pointer-events-none opacity-0',
      )}
      onClick={() => !loading && onClose()}
    >
      <div
        ref={sheetRef}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className={cn(
          'max-h-[88dvh] w-full max-w-md overflow-y-auto rounded-t-2xl border border-border bg-background/95 p-5 pb-[max(1.25rem,calc(0.75rem+var(--cabinet-sheet-safe-bottom)))] shadow-[0_24px_60px_-24px_rgb(0_0_0_/_0.55)] backdrop-blur-sm',
          'transition-[opacity,transform] duration-[250ms] ease-out sm:max-h-[90vh] sm:max-w-sm sm:rounded-2xl sm:pb-5',
          visible
            ? 'translate-y-0 opacity-100 sm:scale-100'
            : 'translate-y-full opacity-0 sm:translate-y-1 sm:scale-[0.97]',
        )}
        onClick={(e) => e.stopPropagation()}
      >
        <div
          className="-mx-5 -mt-5 mb-1 flex cursor-grab touch-none justify-center pb-2 pt-3 sm:hidden"
          {...dragHandlers}
        >
          <span className="h-1 w-10 rounded-full bg-muted-foreground/30" aria-hidden />
        </div>

        <div
          className={cn(
            'flex touch-none gap-3 sm:touch-auto',
            description && !inlineTitle ? 'items-start' : 'items-center',
          )}
          {...dragHandlers}
        >
          <div
            className={cn(
              'flex h-10 w-10 shrink-0 items-center justify-center rounded-xl',
              danger
                ? 'bg-destructive/15 text-destructive'
                : 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
            )}
          >
            <Icon className="h-5 w-5" aria-hidden />
          </div>
          <div className="min-w-0 flex-1 space-y-1.5">
            <h2 className="text-base font-semibold leading-snug text-foreground">{title}</h2>
            {description && !inlineTitle ? (
              <div className="text-sm leading-relaxed text-muted-foreground">{description}</div>
            ) : null}
          </div>
        </div>
        {description && inlineTitle ? (
          <div className="mt-3 text-sm leading-relaxed text-muted-foreground">{description}</div>
        ) : null}

        {/* Подтверждение слева, отмена справа — одинаковая ширина, без переноса на мобиле. */}
        <div className="mt-5 flex items-center gap-2">
          <Button
            ref={confirmRef}
            type="button"
            variant={danger ? 'destructive' : 'default'}
            className="flex-1"
            loading={loading}
            disabled={loading}
            onClick={onConfirm}
          >
            {confirmLabel}
          </Button>
          <Button
            type="button"
            variant="outline"
            className="flex-1"
            disabled={loading}
            onClick={onClose}
          >
            {cancelLabel ?? t('common.cancel')}
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  )
}

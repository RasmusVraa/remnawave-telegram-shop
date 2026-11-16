import { type ReactNode, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { X, type LucideIcon } from 'lucide-react'

import { useSheetDragDismiss } from '@/hooks/useSheetDragDismiss'
import { cn } from '@/lib/utils'
import { rwIconToneClassNames, type RwIconTone } from '../utils/rwStatusStyles'
import {
  adminSectionIconAccentClassNames,
  type AdminSectionIconAccent,
} from '../utils/adminSectionIconAccents'

/**
 * Шкала ширин окна. Три ступени вместо произвольных `sm:max-w-*` у каждого
 * вызова: раньше на одном экране жили окна в трёх разных ширинах, и переход
 * между ними читался как переход в другой раздел.
 *
 *  - `sm` — подтверждение или короткий список действий;
 *  - `md` — одно-два поля: трафик, срок, серверы, описание;
 *  - `lg` — внутри окна список: устройства.
 *
 * Без `size` окно остаётся прежней ширины (448 px) — так старые вызовы не
 * меняют вид сами по себе; новым лучше указывать ступень явно.
 */
/** Появление и уход окна; из него же — задержка размонтирования (как у ResponsiveSheet). */
const ANIM_MS = 220

const SIZE_CLASS = {
  sm: 'sm:max-w-sm',
  md: 'sm:max-w-lg',
  lg: 'sm:max-w-2xl',
} as const

export type AdminModalSize = keyof typeof SIZE_CLASS

interface AdminModalProps {
  open: boolean
  onClose: () => void
  title: string
  /** Подпись под заголовком: контекст окна, а не инструкция. */
  description?: string
  children: ReactNode
  footer?: ReactNode
  className?: string
  size?: AdminModalSize
  panelClassName?: string
  bodyClassName?: string
  icon?: LucideIcon
  iconTone?: RwIconTone
  iconAccent?: AdminSectionIconAccent
  /** Кнопка в шапке напротив заголовка (например, фильтр). */
  headerAction?: ReactNode
  /** Без крестика: окно закрывается тапом вне его. Только вместе с headerAction. */
  hideClose?: boolean
  /** Своя разметка вместо текста заголовка (title остаётся для доступности). */
  titleSlot?: ReactNode
  /** В окне несохранённые изменения: свайп вниз не уносит шторку, а только вызывает onClose. */
  dismissGuarded?: boolean
}

export function AdminModal({
  open,
  onClose,
  title,
  description,
  children,
  footer,
  className,
  size,
  panelClassName,
  bodyClassName,
  icon: Icon,
  iconTone = 'default',
  iconAccent,
  headerAction,
  hideClose,
  titleSlot,
  dismissGuarded,
}: AdminModalProps) {
  const { t } = useTranslation()
  const { sheetRef, handlers: dragHandlers } = useSheetDragDismiss({ open, onDismiss: onClose, snapBack: dismissGuarded })

  // На телефоне шторка выезжает снизу, на ПК окно проявляется из центра; уходит
  // так же. mounted держит разметку на время ухода, visible переключает классы.
  const [mounted, setMounted] = useState(open)
  const [visible, setVisible] = useState(false)
  useEffect(() => {
    if (open) {
      setMounted(true)
      // Два кадра: в первом разметка только вставляется — переключи классы сразу,
      // и браузер увидит лишь конечное состояние, без выезда.
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

  const iconStyles =
    iconTone !== 'default'
      ? rwIconToneClassNames(iconTone)
      : iconAccent
        ? adminSectionIconAccentClassNames(iconAccent)
        : rwIconToneClassNames('default')

  useEffect(() => {
    if (!open) return
    function onEsc(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    document.addEventListener('keydown', onEsc)
    return () => {
      document.body.style.overflow = prevOverflow
      document.removeEventListener('keydown', onEsc)
    }
  }, [open, onClose])

  // Пока окно уезжает, родитель обычно уже сбросил данные (промокод, партнёр,
  // редактируемая нода) — показываем то, что было на экране в момент закрытия,
  // а не пустое тело или скелетон.
  const live = { title, description, children, footer, headerAction, titleSlot, Icon }
  const frozenRef = useRef(live)
  if (open) frozenRef.current = live
  const view = open ? live : frozenRef.current

  if (!mounted) return null

  return createPortal(
    <div
      className={cn(
        'fixed inset-0 z-[200] flex items-end justify-center bg-black/50 backdrop-blur-sm',
        'sm:items-center sm:p-4',
        'transition-opacity duration-[220ms] ease-out motion-reduce:transition-none',
        visible ? 'opacity-100' : 'opacity-0',
        !open && 'pointer-events-none',
        className,
      )}
      onClick={open ? onClose : undefined}
    >
        <div
          ref={sheetRef}
          onClick={(e) => e.stopPropagation()}
          className={cn(
            'cabinet-elevated-card admin-modal-panel flex max-h-[min(92dvh,100%)] w-full flex-col overflow-hidden',
            // Ниже sm любое окно — шторка снизу: на телефоне так до кнопок
            // ближе, чем до центра экрана.
            'rounded-b-none rounded-t-2xl sm:rounded-xl',
            size ? SIZE_CLASS[size] : 'sm:max-w-md',
            'transition-[opacity,transform] duration-[220ms] ease-out motion-reduce:transition-none',
            visible
              ? 'translate-y-0 opacity-100 sm:scale-100'
              : 'translate-y-full opacity-0 sm:translate-y-0 sm:scale-[0.97]',
            panelClassName,
          )}
        >
          {/* Ручка шторки: за неё и за заголовок окно тянут вниз, чтобы закрыть. */}
          <div className="flex shrink-0 cursor-grab touch-none justify-center pb-1 pt-2.5 sm:hidden" {...dragHandlers}>
            <span className="h-1 w-10 rounded-full bg-muted-foreground/30" aria-hidden />
          </div>
          <div
            className="flex shrink-0 touch-none items-center justify-between gap-3 border-b border-border/70 px-5 pb-4 pt-2 sm:touch-auto sm:pt-4"
            {...dragHandlers}
          >
            <div className="flex min-w-0 items-center gap-3">
              {view.Icon && (
                <div
                  className={cn(
                    'flex size-9 shrink-0 items-center justify-center rounded-lg',
                    iconStyles.boxClassName,
                  )}
                >
                  <view.Icon className={cn('size-4', iconStyles.iconClassName)} />
                </div>
              )}
              <div className="min-w-0">
                {view.titleSlot ?? <h3 className="truncate text-lg font-semibold leading-tight">{view.title}</h3>}
                {view.description && (
                  <p className="mt-0.5 truncate text-xs text-muted-foreground">{view.description}</p>
                )}
              </div>
            </div>
            {view.headerAction && (
              <div
                className="shrink-0"
                onPointerDown={(e) => e.stopPropagation()}
                onTouchStart={(e) => e.stopPropagation()}
              >
                {view.headerAction}
              </div>
            )}
            {!(hideClose && view.headerAction) && (
              <button
                type="button"
                onClick={onClose}
                className="rounded-btn p-1 hover:bg-accent"
                aria-label={t('common.close')}
              >
                <X className="size-4" />
              </button>
            )}
          </div>
          {/* Нижний отступ с учётом кнопок навигации Android несёт последний
              блок: футер, а без него — тело окна. */}
          <div
            className={cn(
              'min-h-0 flex-1 overflow-y-auto p-5',
              !view.footer && 'pb-[max(1.25rem,calc(0.75rem+var(--cabinet-sheet-safe-bottom)))] sm:pb-5',
              bodyClassName,
            )}
          >
            {view.children}
          </div>
          {view.footer && (
            <div className="shrink-0 border-t border-border/70 bg-muted/25 px-5 py-4 pb-[max(1rem,calc(0.75rem+var(--cabinet-sheet-safe-bottom)))] dark:bg-secondary/25 sm:pb-4">
              {view.footer}
            </div>
          )}
        </div>
    </div>,
    document.body,
  )
}

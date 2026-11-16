import { useEffect, useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { Check, Copy } from 'lucide-react'

import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { cn } from '@/lib/utils'
import type { AdminPromoCodeDTO } from '@/lib/types/admin'
import { resolvePromoDisplayStatus, type PromoDisplayStatus } from '../utils/promoStatus'
import {
  describeExpiry,
  describeUsage,
  EXPIRY_TONE_CLASS,
  promoTypeIcon,
  promoTypeTone,
  USAGE_BAR_CLASS,
} from './promoPresentation'

/** Плитка с иконкой типа: 34 — таблица, 30 — карточка телефона. */
export function PromoTypeTile({ type, size = 34, className }: { type: string; size?: 30 | 34 | 36; className?: string }) {
  const Icon = promoTypeIcon(type)
  return (
    <span
      className={cn('inline-flex shrink-0 items-center justify-center', promoTypeTone(type).tile, className)}
      style={{ width: size, height: size, borderRadius: size >= 34 ? 10 : 9 }}
      aria-hidden
    >
      <Icon className="size-4" />
    </span>
  )
}

const STATUS_CHIP: Record<PromoDisplayStatus, { cls: string; key: string }> = {
  active: { cls: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400', key: 'admin.promos.statusActive' },
  exhausted: { cls: 'bg-amber-500/15 text-amber-700 dark:text-amber-400', key: 'admin.promos.statusExhausted' },
  expired: { cls: 'bg-amber-500/15 text-amber-700 dark:text-amber-400', key: 'admin.promos.statusExpired' },
  inactive: { cls: 'bg-muted text-muted-foreground', key: 'admin.promos.statusOff' },
}

export function PromoStatusChip({ promo, className }: { promo: AdminPromoCodeDTO; className?: string }) {
  const { t } = useTranslation()
  const s = STATUS_CHIP[resolvePromoDisplayStatus(promo)]
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-semibold',
        s.cls,
        className,
      )}
    >
      <span className="size-1.5 rounded-full bg-current" aria-hidden />
      {t(s.key)}
    </span>
  )
}

export function PromoUsageBar({ promo, className }: { promo: AdminPromoCodeDTO; className?: string }) {
  const u = describeUsage(promo)
  return (
    <div className={cn('h-1.5 overflow-hidden rounded-full bg-muted-foreground/20', className)} aria-hidden>
      {u.max != null && (
        <div className={cn('h-full rounded-full transition-[width]', USAGE_BAR_CLASS[u.tone])} style={{ width: `${u.pct}%` }} />
      )}
    </div>
  )
}

/** «**2** из 6» или «**12** активаций». */
export function PromoUsageText({ promo, className }: { promo: AdminPromoCodeDTO; className?: string }) {
  const { t } = useTranslation()
  const u = describeUsage(promo)
  return (
    <span className={cn('whitespace-nowrap text-[13px] text-muted-foreground tabular-nums', className)}>
      <b className="font-semibold text-foreground">{u.used}</b>{' '}
      {u.max != null ? t('admin.promos.used.ofMax', { max: u.max }) : t('admin.promos.used.activations', { count: u.used })}
    </span>
  )
}

export function PromoExpiryText({ validUntil, className }: { validUntil?: string | null; className?: string }) {
  const { t } = useTranslation()
  const e = describeExpiry(validUntil, t)
  return <span className={cn('whitespace-nowrap text-[13px] tabular-nums', EXPIRY_TONE_CLASS[e.tone], className)}>{e.text}</span>
}

/** Кнопка копирования кода: иконка → галочка на 2 с. Внутри кликабельной строки — не всплывает. */
export function CopyCodeButton({ code, className }: { code: string; className?: string }) {
  const { t } = useTranslation()
  const { state, copy } = useCopyToClipboard()
  const label = state === 'done' ? t('admin.promos.copy.done') : state === 'failed' ? t('admin.promos.copy.failed') : t('admin.promos.copy.action')
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      onClick={(e) => {
        e.stopPropagation()
        void copy(code)
      }}
      className={cn(
        'inline-flex size-7 shrink-0 items-center justify-center rounded-[8px] text-muted-foreground transition-colors hover:bg-primary/10 hover:text-foreground',
        state === 'done' && 'text-emerald-600 dark:text-emerald-400',
        className,
      )}
    >
      {state === 'done' ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
    </button>
  )
}

/** Таблица — только когда ширина самого списка ≥ 960 px (рядом может стоять боковое меню). */
export const PROMO_TABLE_MIN_WIDTH_PX = 960

export function usePromoListLayout(ref: RefObject<HTMLElement | null>): { layout: 'table' | 'cards'; twoColumns: boolean } {
  const [width, setWidth] = useState(0)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const apply = () => setWidth(el.clientWidth)
    apply()
    const ro = new ResizeObserver(apply)
    ro.observe(el)
    return () => ro.disconnect()
  }, [ref])
  return { layout: width >= PROMO_TABLE_MIN_WIDTH_PX ? 'table' : 'cards', twoColumns: width >= 640 }
}

/**
 * Всплывающий список у кнопки: портал с position: fixed, как у HeaderFilter —
 * таблица лежит в контейнере с overflow, и обычный блок обрезался бы.
 */
export function PromoPopover({
  anchorRef,
  open,
  onClose,
  align = 'left',
  width = 250,
  children,
  label,
}: {
  anchorRef: RefObject<HTMLElement | null>
  open: boolean
  onClose: () => void
  align?: 'left' | 'right'
  width?: number
  children: ReactNode
  label: string
}) {
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null)
  const listRef = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    if (!open) {
      setPos(null)
      return
    }
    const place = () => {
      const rect = anchorRef.current?.getBoundingClientRect()
      if (!rect) return
      const raw = align === 'right' ? rect.right - width : rect.left
      const left = Math.max(8, Math.min(raw, window.innerWidth - width - 8))
      const listH = listRef.current?.offsetHeight ?? 0
      const below = rect.bottom + 6
      const top = listH && below + listH > window.innerHeight - 8 ? Math.max(8, rect.top - 6 - listH) : below
      setPos({ left, top })
    }
    place()
    // Второй проход — когда список уже отрисован и известна его высота.
    const raf = requestAnimationFrame(place)
    window.addEventListener('resize', place)
    window.addEventListener('scroll', place, true)
    return () => {
      cancelAnimationFrame(raf)
      window.removeEventListener('resize', place)
      window.removeEventListener('scroll', place, true)
    }
  }, [open, align, width, anchorRef])

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      const target = e.target as Node
      if (anchorRef.current?.contains(target) || listRef.current?.contains(target)) return
      onClose()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, onClose, anchorRef])

  if (!open) return null
  return createPortal(
    <div
      ref={listRef}
      role="menu"
      aria-label={label}
      onClick={(e) => e.stopPropagation()}
      style={{ position: 'fixed', left: pos?.left ?? -9999, top: pos?.top ?? -9999, width, zIndex: 9999 }}
      className="rounded-[14px] border border-border bg-card p-1.5 text-sm text-foreground shadow-xl"
    >
      {children}
    </div>,
    document.body,
  )
}

export function PromoPopoverItem({
  icon,
  children,
  onClick,
  selected,
  danger,
  trailing,
}: {
  icon?: ReactNode
  children: ReactNode
  onClick: () => void
  selected?: boolean
  danger?: boolean
  trailing?: ReactNode
}) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onClick}
      className={cn(
        'flex min-h-9 w-full items-center gap-2.5 rounded-[9px] px-2.5 py-2 text-left transition-colors hover:bg-primary/10',
        selected && 'bg-primary/10',
        danger && 'text-destructive hover:bg-destructive/10',
      )}
    >
      {icon !== undefined && <span className="inline-flex size-4 shrink-0 items-center justify-center">{icon}</span>}
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {trailing}
    </button>
  )
}

import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Check, ChevronRight, CircleCheck, TriangleAlert, X } from 'lucide-react'

import { ResponsiveSheet } from '@/components/ResponsiveSheet'
import { useIsMobile } from '@/hooks/useIsMobile'
import { cn } from '@/lib/utils'
import { sinceText } from './eventView'
import { useAttentionDismissals, type AttentionEntry } from './attention'

const GROUPS: { severity: AttentionEntry['severity']; titleKey: string }[] = [
  { severity: 'crit', titleKey: 'admin.attention.groupCrit' },
  { severity: 'warn', titleKey: 'admin.attention.groupWarn' },
]

/**
 * «Требует внимания» одной кнопкой в шапке страницы: счётчик и цвет по худшему
 * пункту, по нажатию — список. На телефоне список — шторка снизу, на ПК —
 * выпадашка под кнопкой. Прочитанное скрывается и возвращается само, только если
 * стало хуже (см. isDismissed).
 *
 * Пока гореть нечему — кнопки нет вовсе; всё прочитано — кнопка серая.
 */
export function AttentionMenu({
  entries,
  ready,
  owns,
}: {
  entries: AttentionEntry[]
  /** Список пунктов страницы уже загружен. */
  ready: boolean
  /** Какие ключи считает эта страница (для очистки устаревших отметок). */
  owns?: (id: string) => boolean
}) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const { visible, hidden, pending, dismiss, dismissAll, restoreAll } = useAttentionDismissals(
    entries,
    ready,
    owns,
  )

  // Выпадашка закрывается кликом мимо и Esc; шторка делает это сама.
  useEffect(() => {
    if (!open || isMobile) return
    const onDown = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, isMobile])

  // Гореть больше нечему — окно закрываем, а не оставляем пустым.
  useEffect(() => {
    if (open && entries.length === 0) setOpen(false)
  }, [open, entries.length])

  if (entries.length === 0) return null

  const calm = visible.length === 0
  const crit = visible.some((e) => e.severity === 'crit')
  const tone = calm
    ? 'border-border bg-card text-muted-foreground hover:bg-secondary/80'
    : crit
      ? 'border-rose-500/50 bg-card bg-gradient-to-r from-rose-500/[0.14] to-rose-500/[0.14] text-rose-500 hover:border-rose-500'
      : 'border-amber-500/50 bg-card bg-gradient-to-r from-amber-500/[0.14] to-amber-500/[0.14] text-amber-600 hover:border-amber-500 dark:text-amber-400'

  const label = calm ? t('admin.attention.allRead') : t('admin.attention.title')

  const button = (
    <button
      type="button"
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-label={calm ? label : `${label}: ${visible.length}`}
      title={label}
      onClick={() => setOpen((v) => !v)}
      className={cn(
        'relative inline-flex size-10 shrink-0 items-center justify-center rounded-xl border transition-colors sm:w-auto sm:gap-2 sm:px-3.5',
        tone,
        open && 'ring-2 ring-ring/30',
      )}
    >
      {calm ? (
        <CircleCheck className="size-[18px] sm:size-4" aria-hidden />
      ) : (
        <TriangleAlert className="size-[18px] sm:size-4" aria-hidden />
      )}
      <span className="hidden text-sm font-medium sm:inline">{label}</span>
      {!calm && (
        <span
          className={cn(
            'absolute -right-1.5 -top-1.5 flex h-[18px] min-w-[18px] items-center justify-center rounded-full border-2 border-background px-1 text-[11px] font-bold leading-none text-white tabular-nums sm:static sm:h-5 sm:min-w-5 sm:border-0 sm:px-1.5 sm:text-xs',
            crit ? 'bg-rose-500' : 'bg-amber-500',
          )}
        >
          {visible.length}
        </span>
      )}
    </button>
  )

  const list = calm ? (
    <p className="py-8 text-center text-sm text-muted-foreground">{t('admin.attention.empty')}</p>
  ) : (
    GROUPS.map(({ severity, titleKey }) => {
      const group = visible.filter((e) => e.severity === severity)
      if (group.length === 0) return null
      return (
        <section key={severity}>
          <h3 className="px-1 pb-1 pt-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
            {t(titleKey)}
          </h3>
          <ul className="divide-y divide-border/50">
            {group.map((e) => (
              <AttentionRow key={e.id} entry={e} onGo={() => setOpen(false)} onDismiss={() => dismiss(e)} />
            ))}
          </ul>
        </section>
      )
    })
  )

  const hasFooter = hidden > 0 || !calm
  const footer = (
    <div className="flex items-center gap-2">
      {hidden > 0 && (
        <button
          type="button"
          onClick={restoreAll}
          className="min-h-10 flex-1 rounded-xl px-2 text-left text-[13px] text-muted-foreground transition-colors hover:text-foreground sm:flex-none"
        >
          {t('admin.attention.showHidden', { count: hidden })}
        </button>
      )}
      {!calm && (
        <button
          type="button"
          onClick={dismissAll}
          disabled={pending}
          className="ml-auto inline-flex min-h-10 flex-1 items-center justify-center gap-1.5 rounded-xl border border-border bg-secondary/60 px-3.5 text-sm font-medium transition-colors hover:bg-secondary disabled:opacity-60 sm:flex-none"
        >
          <Check className="size-4" aria-hidden />
          {t('admin.attention.dismissAll')}
        </button>
      )}
    </div>
  )

  return (
    <div className="relative" ref={rootRef}>
      {button}
      {isMobile ? (
        <ResponsiveSheet
          open={open}
          onClose={() => setOpen(false)}
          title={t('admin.attention.title')}
          footer={hasFooter ? footer : undefined}
        >
          <div>{list}</div>
        </ResponsiveSheet>
      ) : (
        open && (
          <div
            role="dialog"
            aria-label={t('admin.attention.title')}
            className="absolute right-0 top-12 z-30 flex max-h-[min(70vh,32rem)] w-[26rem] max-w-[calc(100vw-2rem)] flex-col overflow-hidden rounded-2xl border border-border bg-card shadow-[0_18px_50px_rgba(0,0,0,0.5)]"
          >
            <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 pb-2 pt-1.5">{list}</div>
            {hasFooter && <div className="shrink-0 border-t border-border/60 px-3 py-2">{footer}</div>}
          </div>
        )
      )}
    </div>
  )
}

function AttentionRow({
  entry,
  onGo,
  onDismiss,
}: {
  entry: AttentionEntry
  onGo: () => void
  onDismiss: () => void
}) {
  const { t } = useTranslation()
  const dot = entry.severity === 'crit' ? 'bg-rose-500' : 'bg-amber-500'
  const body = (
    <>
      <span className={cn('size-2 shrink-0 rounded-full', dot)} aria-hidden />
      <span className="min-w-0 flex-1">
        <span className="block text-sm leading-snug">{entry.label}</span>
        {entry.since && (
          <span className="block text-xs tabular-nums text-muted-foreground">
            {sinceText(entry.since, t)}
          </span>
        )}
      </span>
      {entry.to && <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden />}
    </>
  )
  const cls = 'flex min-h-12 min-w-0 flex-1 items-center gap-3 rounded-lg py-2 pl-1 pr-1 sm:min-h-11'
  return (
    <li className="flex items-center gap-1">
      {entry.to ? (
        <Link to={entry.to} onClick={onGo} className={cn(cls, 'hover:bg-secondary/50')}>
          {body}
        </Link>
      ) : (
        <div className={cls}>{body}</div>
      )}
      <button
        type="button"
        onClick={onDismiss}
        aria-label={t('admin.attention.dismiss')}
        title={t('admin.attention.dismiss')}
        className="inline-flex size-10 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground sm:size-8"
      >
        <X className="size-4" aria-hidden />
      </button>
    </li>
  )
}

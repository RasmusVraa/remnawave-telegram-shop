import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, Globe, X } from 'lucide-react'

import type { AdminCheckResult, AdminCheckState, AdminRowChecksDTO } from '@/lib/api'
import { cn } from '@/lib/utils'

const TONE: Record<AdminCheckState, string> = {
  ok: 'border-emerald-500/35 bg-emerald-500/10 text-emerald-500',
  partial: 'border-amber-500/40 bg-amber-500/10 text-amber-500',
  bad: 'border-rose-500/40 bg-rose-500/10 text-rose-500',
  none: 'border-border/60 text-muted-foreground opacity-50',
}

const MARK_BG: Record<AdminCheckState, string> = {
  ok: 'bg-emerald-500',
  partial: 'bg-amber-500',
  bad: 'bg-rose-500',
  none: 'bg-muted-foreground',
}

/** Два значка у строки сервера: «мир» и «РФ». Подсказка по наведению и по нажатию. */
export function AdminStatusRowChecks({
  checks,
  enabled,
  showRegistry = false,
  className,
}: {
  checks?: AdminRowChecksDTO
  enabled: boolean
  /** Третий значок «РКН». У строк обхода его нет: там адрес — хост из белого списка, не сервер. */
  showRegistry?: boolean
  className?: string
}) {
  const { t } = useTranslation()
  const none: AdminCheckResult = { state: 'none', ok: 0, total: 0 }
  const world = enabled && checks ? checks.world : none
  const russia = enabled && checks ? checks.russia : none
  const registry = enabled && checks?.registry ? checks.registry : { state: 'none' as const }
  const registryResult: AdminCheckResult = { state: registry.state, ok: 0, total: 0 }
  return (
    <div className={cn('flex shrink-0 gap-1', className)} role="group" aria-label={t('admin.statusPage.chkAria')}>
      <CheckBadge
        result={world}
        title={t('admin.statusPage.chkWorld')}
        hint={t('admin.statusPage.chkWorldHint')}
        icon={<Globe className="size-4" />}
      />
      <CheckBadge
        result={russia}
        title={checks?.operators ? t('admin.statusPage.chkOps') : t('admin.statusPage.chkRu')}
        hint={checks?.operators ? t('admin.statusPage.chkOpsHint') : t('admin.statusPage.chkRuHint')}
        icon={<span className="text-[10px] font-extrabold leading-none">РФ</span>}
      />
      {showRegistry && (
        <CheckBadge
          result={registryResult}
          title={t('admin.statusPage.chkRkn')}
          hint={t(registry.state === 'bad' ? (registry.kind === 'cdn' ? 'admin.statusPage.chkRknCdnHint' : 'admin.statusPage.chkRknHint') : 'admin.statusPage.chkRknOkHint')}
          verdict={t(`admin.statusPage.chkRkn${registry.state[0].toUpperCase()}${registry.state.slice(1)}`)}
          icon={<span className="text-[10px] font-extrabold leading-none">РКН</span>}
        />
      )}
    </div>
  )
}

function CheckBadge({
  result,
  title,
  hint,
  icon,
  verdict: verdictText,
}: {
  result: AdminCheckResult
  title: string
  hint: string
  icon: React.ReactNode
  /** Свой вердикт вместо «отвечает / не отвечает»: у «РКН» смысл другой. */
  verdict?: string
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const close = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('pointerdown', close)
    return () => document.removeEventListener('pointerdown', close)
  }, [open])

  const verdict = verdictText ?? t(`admin.statusPage.chk${result.state[0].toUpperCase()}${result.state.slice(1)}`)

  return (
    <div ref={ref} className="group relative" onMouseLeave={() => setOpen(false)}>
      <button
        type="button"
        aria-label={`${title}: ${verdict}`}
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className={cn(
          'relative flex size-[30px] items-center justify-center rounded-[9px] border transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
          TONE[result.state],
        )}
      >
        {icon}
        <span
          className={cn(
            'absolute -bottom-[3px] -right-[3px] flex size-[13px] items-center justify-center rounded-full border-2 border-background text-background',
            MARK_BG[result.state],
          )}
        >
          {result.state === 'ok' && <Check className="size-2" strokeWidth={4} />}
          {result.state === 'bad' && <X className="size-2" strokeWidth={4} />}
          {result.state === 'partial' && <span className="text-[8px] font-black leading-none">!</span>}
        </span>
      </button>
      <div
        role="tooltip"
        className={cn(
          'absolute right-0 top-9 z-30 w-60 max-w-[75vw] rounded-xl border border-border bg-card p-3 text-left text-xs shadow-xl',
          open ? 'block' : 'hidden group-hover:block',
        )}
      >
        <p className="font-semibold">
          {title}: {verdict}
        </p>
        <p className="mt-1 text-muted-foreground">
          {result.state === 'none' ? (verdictText ? t('admin.statusPage.chkRknNoneHint') : t('admin.statusPage.chkNoneHint')) : hint}
          {result.total > 0 && <> {t('admin.statusPage.chkCount', { ok: result.ok, total: result.total })}</>}
        </p>
      </div>
    </div>
  )
}

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, CircleHelp, ExternalLink, KeyRound } from 'lucide-react'

import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { AdminModal } from './AdminModal'

/** «Откуда берутся данные»: два внешних сервиса, подключены они или нет и сколько лимита уходит. */
export function AdminStatusChecks() {
  const { t } = useTranslation()
  const [help, setHelp] = useState(false)
  const quota = useQuery({
    queryKey: ['admin-status-quota'],
    queryFn: () => api.adminStatusQuota(),
  })
  const data = quota.data
  const tight = data ? data.per_hour > data.limit * 0.8 : false
  const percent = data ? Math.min(100, Math.round((data.per_hour / data.limit) * 100)) : 0

  return (
    <section className="rounded-2xl border border-border/60 bg-card/50 p-4 sm:p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold tracking-tight">{t('admin.statusPage.checksTitle')}</h2>
          <p className="text-sm text-muted-foreground">{t('admin.statusPage.checksHint')}</p>
        </div>
        <button
          type="button"
          onClick={() => setHelp(true)}
          className="inline-flex min-h-9 items-center gap-1.5 rounded-lg border border-border px-3 text-sm font-medium hover:bg-muted"
        >
          <CircleHelp className="size-4" />
          {t('admin.statusPage.howToConnect')}
        </button>
      </div>

      {data && (
        <div className="mt-4 grid gap-3 sm:grid-cols-2">
          <div className="rounded-xl border border-border/60 bg-background/40 p-3.5">
            <div className="flex items-center justify-between gap-2">
              <span className="font-medium">Globalping</span>
              <Status on={data.token_set} onText={t('admin.statusPage.keyOn')} offText={t('admin.statusPage.gpOff')} />
            </div>
            <p className="mt-1 text-xs text-muted-foreground">{t('admin.statusPage.gpText')}</p>
            <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-muted">
              <div
                className={cn('h-full rounded-full', tight ? 'bg-amber-500' : 'bg-primary')}
                style={{ width: `${percent}%` }}
              />
            </div>
            <p className={cn('mt-1.5 text-xs', tight ? 'font-medium text-amber-600 dark:text-amber-400' : 'text-muted-foreground')}>
              {t('admin.statusPage.gpUsage', { used: data.per_hour, limit: data.limit })}
            </p>
            {data.globalping_paused_until && (
              <p className="mt-1.5 flex items-start gap-1.5 text-xs text-rose-500">
                <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
                {t('admin.statusPage.pausedUntil', { time: clock(data.globalping_paused_until) })}
              </p>
            )}
            {tight && (
              <p className="mt-1.5 flex items-start gap-1.5 text-xs text-amber-600 dark:text-amber-400">
                <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
                {t('admin.statusPage.gpTight')}
              </p>
            )}
          </div>

          <div className="rounded-xl border border-border/60 bg-background/40 p-3.5">
            <div className="flex items-center justify-between gap-2">
              <span className="font-medium">Latency Lab</span>
              <Status on={data.latencylab_set} onText={t('admin.statusPage.keyOn')} offText={t('admin.statusPage.llOff')} />
            </div>
            <p className="mt-1 text-xs text-muted-foreground">{t('admin.statusPage.llText')}</p>
            {data.latencylab_set && (
              <p className="mt-3 text-xs text-muted-foreground">
                {t('admin.statusPage.llQuota')}{' '}
                <ExtLink href="https://console.latencylab.ru">console.latencylab.ru</ExtLink>
              </p>
            )}
            {data.latencylab_paused_until && (
              <p className="mt-1.5 flex items-start gap-1.5 text-xs text-rose-500">
                <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
                {t('admin.statusPage.pausedUntil', { time: clock(data.latencylab_paused_until) })}
              </p>
            )}
          </div>
        </div>
      )}

      <AdminModal
        open={help}
        onClose={() => setHelp(false)}
        title={t('admin.statusPage.howTitle')}
        description={t('admin.statusPage.howLead')}
        size="md"
        icon={KeyRound}
        iconAccent="cyan"
      >
        <ol className="space-y-4 text-sm">
          <li>
            <p className="font-medium">1. {t('admin.statusPage.howStep1')}</p>
            <ul className="mt-1 space-y-1 text-muted-foreground">
              <li>
                {t('admin.statusPage.howGp')}{' '}
                <ExtLink href="https://dash.globalping.io">dash.globalping.io</ExtLink>
              </li>
              <li>
                {t('admin.statusPage.howLl')}{' '}
                <ExtLink href="https://console.latencylab.ru">console.latencylab.ru</ExtLink>
              </li>
            </ul>
          </li>
          <li>
            <p className="font-medium">2. {t('admin.statusPage.howStep2')}</p>
            <pre className="mt-2 overflow-x-auto rounded-lg bg-muted px-3 py-2.5 text-xs leading-relaxed">
              {'GLOBALPING_TOKEN=<token>\nLATENCYLAB_TOKEN=<token>'}
            </pre>
          </li>
          <li>
            <p className="font-medium">3. {t('admin.statusPage.howStep3')}</p>
            <p className="mt-1 text-muted-foreground">{t('admin.statusPage.howStep3Text')}</p>
          </li>
        </ol>
      </AdminModal>
    </section>
  )
}

function ExtLink({ href, children }: { href: string; children: string }) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className="inline-flex items-center gap-1 font-medium text-primary underline-offset-2 hover:underline"
    >
      {children}
      <ExternalLink className="size-3" aria-hidden />
    </a>
  )
}

function clock(iso: string): string {
  const at = new Date(iso)
  return Number.isNaN(at.getTime()) ? '' : at.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function Status({ on, onText, offText }: { on: boolean; onText: string; offText: string }) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium',
        on ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400' : 'bg-muted text-muted-foreground',
      )}
    >
      <span className={cn('size-1.5 rounded-full', on ? 'bg-emerald-500' : 'bg-muted-foreground/60')} />
      {on ? onText : offText}
    </span>
  )
}

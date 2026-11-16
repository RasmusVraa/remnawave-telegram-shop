import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, CircleHelp, ExternalLink, KeyRound } from 'lucide-react'

import { api, type AdminCheburcheckDTO, type AdminStatusQuotaDTO } from '@/lib/api'
import { cn } from '@/lib/utils'
import { Spinner } from '@/components/ui/spinner'
import { AdminModal } from './AdminModal'
import { formatAdminApiError } from '../utils/formatAdminApiError'

/** «Откуда берутся данные»: два внешних сервиса, подключены они или нет и сколько лимита уходит. */
export function AdminStatusChecks() {
  const { t } = useTranslation()
  const [help, setHelp] = useState(false)
  const quota = useQuery({
    queryKey: ['admin-status-quota'],
    queryFn: () => api.adminStatusQuota(),
  })
  const data = quota.data
  // Настоящий остаток у Globalping, если он ответил; иначе наша прикидка расхода.
  const real = data?.real && data.real.limit > 0 && data.real.remaining >= 0 ? data.real : null
  const used = real ? real.limit - real.remaining : (data?.per_hour ?? 0)
  const cap = real ? real.limit : (data?.limit ?? 1)
  const tight = data ? (real ? real.remaining < real.limit * 0.2 : data.per_hour > data.limit * 0.8) : false
  const percent = data ? Math.min(100, Math.round((used / cap) * 100)) : 0

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
        <>
          {data.cheburcheck && <SourcePicker cheb={data.cheburcheck} />}
          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            <div className="rounded-xl border border-border/60 bg-background/40 p-3.5">
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium">
                  Globalping
                  <RoleTag role={sourceRole(data.cheburcheck, 'globalping')} />
                </span>
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
                {real
                  ? `${t('admin.statusPage.gpReal', { remaining: real.remaining, limit: real.limit })}${
                      real.reset_sec > 0
                        ? `, ${t('admin.statusPage.gpRealReset', { min: Math.max(1, Math.round(real.reset_sec / 60)) })}`
                        : ''
                    }`
                  : t('admin.statusPage.gpUsage', { used: data.per_hour, limit: data.limit })}
              </p>
              {real && (
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('admin.statusPage.gpEstimate', { used: data.per_hour })}
                  {real.credits != null && <> · {t('admin.statusPage.gpCredits', { n: real.credits })}</>}
                </p>
              )}
              {!real && (
                <p className="mt-1 text-xs text-muted-foreground">{t('admin.statusPage.gpRealFail')}</p>
              )}
              {data.globalping_paused_until && (
                <p className="mt-1.5 flex items-start gap-1.5 text-xs text-rose-500">
                  <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
                  <span>
                    {t('admin.statusPage.pausedUntil', { time: clock(data.globalping_paused_until) })}
                    <span className="mt-0.5 block text-muted-foreground">
                      {data.globalping_pause_note && data.globalping_pause_note !== 'HTTP 429'
                        ? t('admin.statusPage.pauseNote', { note: data.globalping_pause_note })
                        : t('admin.statusPage.pauseNoteEmpty')}
                      {real && real.remaining >= Math.max(1, data.max_cost) && (
                        <> · {t('admin.statusPage.pauseNotCredits')}</>
                      )}
                      {real && data.max_cost > 0 && real.remaining < data.max_cost && (
                        <> · {t('admin.statusPage.pauseShortCredits', { cost: data.max_cost, remaining: real.remaining })}</>
                      )}
                    </span>
                  </span>
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
                  <span>
                    {t('admin.statusPage.pausedUntil', { time: clock(data.latencylab_paused_until) })}
                    {data.latencylab_pause_note && (
                      <span className="mt-0.5 block text-muted-foreground">
                        {t('admin.statusPage.pauseNote', { note: data.latencylab_pause_note })}
                      </span>
                    )}
                  </span>
                </p>
              )}
            </div>

            <CheburcheckCard cheb={data.cheburcheck} />
            <RegistryCard data={data.registry} />
          </div>
        </>
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
              <li>
                {t('admin.statusPage.howCheb')}{' '}
                <ExtLink href="https://cheburcheck.ru/kb/api">cheburcheck.ru/kb/api</ExtLink>
              </li>
            </ul>
          </li>
          <li>
            <p className="font-medium">2. {t('admin.statusPage.howStep2')}</p>
            <pre className="mt-2 overflow-x-auto rounded-lg bg-muted px-3 py-2.5 text-xs leading-relaxed">
              {'GLOBALPING_TOKEN=<token>\nLATENCYLAB_TOKEN=<token>\nCHEBURCHECK_TOKEN=<token>'}
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

type Role = '' | 'active' | 'reserve' | 'paused'

/** Роль источника «России»: кто работает сейчас, кто запасной. Без ключа Cheburcheck роли нет. */
function sourceRole(cheb: AdminCheburcheckDTO | undefined, which: 'globalping' | 'cheburcheck'): Role {
  if (!cheb || !cheb.configured) return ''
  if (which === 'cheburcheck' && cheb.paused_until) return 'paused'
  if (cheb.active === which) return 'active'
  return cheb.mode === 'auto' ? 'reserve' : ''
}

function RoleTag({ role }: { role: Role }) {
  const { t } = useTranslation()
  if (!role) return null
  return (
    <span
      className={cn(
        'ml-2 whitespace-nowrap rounded-md px-1.5 py-0.5 text-[11px] font-semibold',
        role === 'active' && 'bg-primary/15 text-primary',
        role === 'paused' && 'bg-rose-500/15 text-rose-500',
        role === 'reserve' && 'bg-muted text-muted-foreground',
      )}
    >
      {t(`admin.statusPage.role${role[0].toUpperCase()}${role.slice(1)}`)}
    </span>
  )
}

/** Выбор источника «России»: Globalping, Cheburcheck или автоматически (с резервом). */
function SourcePicker({ cheb }: { cheb: AdminCheburcheckDTO }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [error, setError] = useState('')
  const pick = useMutation({
    mutationFn: (value: string) => api.adminBotSettingsPatch({ settings: { STATUS_RU_SOURCE: value } }),
    onSuccess: async () => {
      setError('')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['admin-status-quota'] }),
        queryClient.invalidateQueries({ queryKey: ['admin-status-targets'] }),
        queryClient.invalidateQueries({ queryKey: ['admin-bot-settings'] }),
      ])
    },
    onError: (err) => setError(formatAdminApiError(err, t)),
  })
  const options: { value: AdminCheburcheckDTO['mode']; label: string; short?: string; needsKey: boolean }[] = [
    { value: 'globalping', label: 'Globalping', needsKey: false },
    { value: 'cheburcheck', label: 'Cheburcheck', needsKey: true },
    { value: 'auto', label: t('admin.statusPage.pickerAuto'), short: t('admin.statusPage.pickerAutoShort'), needsKey: true },
  ]
  // Без ключа выбор «Автоматически» и «Cheburcheck» ничего не меняет: работает Globalping.
  const current = cheb.configured ? cheb.mode : 'globalping'
  const hint = !cheb.configured
    ? t('admin.statusPage.pickerHintNoKey')
    : t(`admin.statusPage.pickerHint${cheb.mode === 'auto' ? 'Auto' : cheb.mode === 'globalping' ? 'Gp' : 'Cheb'}`)
  return (
    <div className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border/60 bg-background/40 p-3.5">
      <div className="min-w-0 sm:max-w-[34rem]">
        <p className="font-medium">{t('admin.statusPage.pickerTitle')}</p>
        <p className="mt-0.5 text-xs text-muted-foreground">{hint}</p>
        {error && <p className="mt-1 text-xs text-rose-500">{error}</p>}
      </div>
      <div className="inline-flex max-w-full flex-wrap rounded-lg border border-border bg-card p-0.5" role="radiogroup" aria-label={t('admin.statusPage.pickerTitle')}>
        {options.map((o) => {
          const active = current === o.value
          const busy = pick.isPending && pick.variables === o.value
          return (
            <button
              key={o.value}
              type="button"
              role="radio"
              aria-checked={active}
              disabled={pick.isPending || (o.needsKey && !cheb.configured)}
              title={o.needsKey && !cheb.configured ? t('admin.statusPage.pickerHintNoKey') : undefined}
              onClick={() => !active && pick.mutate(o.value)}
              className={cn(
                'inline-flex min-h-9 items-center gap-1.5 rounded-md px-2.5 text-sm font-medium transition-colors sm:px-3',
                active ? 'bg-primary text-primary-foreground' : 'text-foreground hover:bg-muted',
                o.needsKey && !cheb.configured && 'opacity-45',
              )}
            >
              {busy && <Spinner />}
              {o.short ? (
                <>
                  <span className="sm:hidden">{o.short}</span>
                  <span className="max-sm:hidden">{o.label}</span>
                </>
              ) : (
                o.label
              )}
            </button>
          )
        })}
      </div>
    </div>
  )
}

/** Cheburcheck: живые сканеры в российских сетях. Видит блокировку, а не только пинг. */
function CheburcheckCard({ cheb }: { cheb?: AdminCheburcheckDTO }) {
  const { t } = useTranslation()
  const configured = Boolean(cheb?.configured)
  const last = cheb?.last_at ? ago(cheb.last_at, t) : ''
  const note = cheb?.pause_note === 'token rejected' ? t('admin.statusPage.chebTokenRejected') : cheb?.pause_note && cheb.pause_note !== 'HTTP 429' ? t('admin.statusPage.pauseNote', { note: cheb.pause_note }) : t('admin.statusPage.pauseNoteEmpty')
  return (
    <div className="rounded-xl border border-border/60 bg-background/40 p-3.5">
      <div className="flex flex-wrap items-center justify-between gap-x-2 gap-y-1.5">
        <span className="font-medium">
          Cheburcheck
          <RoleTag role={sourceRole(cheb, 'cheburcheck')} />
        </span>
        <Status on={configured} onText={t('admin.statusPage.keyOn')} offText={t('admin.statusPage.chebOff')} />
      </div>
      <p className="mt-1 text-xs text-muted-foreground">{t('admin.statusPage.chebText')}</p>
      <p className="mt-1.5 text-xs text-muted-foreground">{t('admin.statusPage.chebDomain')}</p>
      {configured ? (
        <>
          {cheb?.credits != null && (
            <p className="mt-3 text-xs text-muted-foreground">{t('admin.statusPage.chebCredits', { n: cheb.credits })}</p>
          )}
          {last && (
            <p className={cn('text-xs text-muted-foreground', cheb?.credits != null ? 'mt-1' : 'mt-3')}>
              {t('admin.statusPage.chebLast', { ago: last, pass: cheb?.pass ?? 0, total: cheb?.total ?? 0 })}
            </p>
          )}
          {cheb?.paused_until && (
            <p className="mt-1.5 flex items-start gap-1.5 text-xs text-rose-500">
              <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
              <span>
                {t('admin.statusPage.pausedUntil', { time: clock(cheb.paused_until) })}
                <span className="mt-0.5 block text-muted-foreground">
                  {note}
                  {cheb.mode === 'auto' && <> · {t('admin.statusPage.chebFallback')}</>}
                </span>
              </span>
            </p>
          )}
        </>
      ) : (
        <p className="mt-3 text-xs text-muted-foreground">{t('admin.statusPage.chebNoKey')}</p>
      )}
    </div>
  )
}

/** «Реестр РКН»: числится ли адрес сервера в списках блокировок. Ключа не нужно. */
function RegistryCard({ data }: { data?: AdminStatusQuotaDTO['registry'] }) {
  const { t } = useTranslation()
  const updated = data?.info?.updated_at ? ago(data.info.updated_at, t) : ''
  const listed = (data?.listed ?? 0) > 0
  return (
    <div className="rounded-xl border border-border/60 bg-background/40 p-3.5">
      <div className="flex items-center justify-between gap-2">
        <span className="whitespace-nowrap font-medium">{t('admin.statusPage.rknTitle')}</span>
        <Status on onText={t('admin.statusPage.rknNoKey')} offText="" />
      </div>
      <p className="mt-1 text-xs text-muted-foreground">{t('admin.statusPage.rknText')}</p>
      {updated && <p className="mt-3 text-xs text-muted-foreground">{t('admin.statusPage.rknUpdated', { ago: updated })}</p>}
      {data && data.checked > 0 && (
        <p className={cn('mt-1.5 text-xs', listed ? 'font-medium text-rose-500' : 'text-muted-foreground')}>
          {t('admin.statusPage.rknListed', { listed: data.listed, checked: data.checked })}
        </p>
      )}
      {data?.paused_until && (
        <p className="mt-1.5 flex items-start gap-1.5 text-xs text-rose-500">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
          {t('admin.statusPage.pausedUntil', { time: clock(data.paused_until) })}
        </p>
      )}
      <p className="mt-1.5 text-xs">
        <ExtLink href="https://cheburcheck.ru">cheburcheck.ru</ExtLink>
      </p>
    </div>
  )
}

/** «12 мин», «3 ч»: сколько прошло с момента iso. */
function ago(iso: string, t: (key: string, o?: Record<string, unknown>) => string): string {
  const at = new Date(iso).getTime()
  if (Number.isNaN(at)) return ''
  const min = Math.max(0, Math.round((Date.now() - at) / 60000))
  return min < 90 ? t('admin.statusPage.rknAgoMin', { n: Math.max(1, min) }) : t('admin.statusPage.rknAgoHour', { n: Math.round(min / 60) })
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
        'inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium',
        on ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400' : 'bg-muted text-muted-foreground',
      )}
    >
      <span className={cn('size-1.5 rounded-full', on ? 'bg-emerald-500' : 'bg-muted-foreground/60')} />
      {on ? onText : offText}
    </span>
  )
}

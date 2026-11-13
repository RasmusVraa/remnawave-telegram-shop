import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Activity, ChevronDown } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { Skeleton } from '@/components/ui/skeleton'
import { CountryFlag } from '@/features/admin/components/CountryFlag'
import type { StatusMapNode, StatusProbe, StatusProbeHit } from '@/features/landing/components/StatusMap'
import { StateChip, stateTint } from './StateChip'
import { russiaLabel, russiaTone, summarizeStatus, type RowState } from './statusState'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

/**
 * Цвет столбика: один столбик — один замер. Доля — это успешные проверки замера из всех
 * (10 зондов из России и 1 из мира). Зелёный — все прошли, оранжевый — часть не прошла,
 * но больше половины ответило, красный — ответила половина или меньше.
 */
function historyLevel(value: number | null | undefined): string {
  if (value == null || Number.isNaN(value)) return ''
  if (value >= 1) return 'up'
  if (value > 0.5) return 'partial'
  return 'down'
}

function russiaHits(probe?: StatusProbe): StatusProbeHit[] {
  return (probe?.hits ?? []).filter((hit) => (hit.country ?? '').toUpperCase() === 'RU' && hit.kind !== 'whitelist')
}

function whitelistHits(probe?: StatusProbe): StatusProbeHit[] {
  return (probe?.hits ?? [])
    .filter((hit) => hit.kind === 'whitelist' && !(hit.city ?? '').trim())
    .sort((a, b) => (a.network ?? '').localeCompare(b.network ?? '', 'ru'))
}

function protocolText(ok: boolean | undefined, ms: number | undefined, up: string, down: string, ping: string) {
  if (ok == null && ms == null) return '—'
  if (ok === false) return down
  if (ms != null) return ping
  return up
}

function barTone(level: string): string {
  if (level === 'up') return 'bg-emerald-400'
  if (level === 'partial') return 'bg-amber-400'
  if (level === 'down') return 'bg-rose-400'
  return 'bg-muted-foreground/20'
}

function minutesSince(iso: string | undefined): number {
  if (!iso) return 0
  const ts = Date.parse(iso)
  if (Number.isNaN(ts)) return 0
  return Math.max(0, Math.round((Date.now() - ts) / 60000))
}

function newestMeasuredAt(nodes: StatusMapNode[]): string | undefined {
  let best = 0
  let iso = ''
  for (const node of nodes) {
    const raw = node.probe?.measured_at
    if (!raw) continue
    const ts = Date.parse(raw)
    if (!Number.isNaN(ts) && ts > best) {
      best = ts
      iso = raw
    }
  }
  return iso || undefined
}

/** Статус серверов внутри кабинета: карточки, которые на телефоне не разъезжаются в таблицу. */
export default function CabinetStatusPage() {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState<string | null>(null)
  const query = useQuery({
    queryKey: ['public-status'],
    queryFn: () => api.publicStatus(),
    refetchInterval: 30_000,
  })
  const data = query.data
  const nodes = data?.nodes ?? []
  const total = data?.total ?? nodes.length
  const summary = summarizeStatus(nodes)
  const available = data?.available === true
  const minutesAgo = minutesSince(newestMeasuredAt(nodes) ?? data?.updated_at)
  const tone = !available || total === 0 ? 'idle' : summary.tone

  return (
    <AppLayout>
      <PageReveal className="cabinet-status-page space-y-4">
        <RevealItem>
          <div>
            <h1 className="font-heading text-2xl font-bold tracking-tight sm:text-3xl">{t('cabinetStatus.title')}</h1>
            <p className="mt-1 max-w-xl text-sm text-muted-foreground">{t('cabinetStatus.lead')}</p>
            <span
              className={cn(
                'mt-3 inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold',
                tone === 'up' && 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300',
                tone === 'partial' && 'bg-amber-500/15 text-amber-800 dark:text-amber-200',
                tone === 'down' && 'bg-rose-500/15 text-rose-700 dark:text-rose-300',
                tone === 'idle' && 'bg-muted text-muted-foreground',
              )}
            >
              <Activity className="size-3.5" aria-hidden />
              {tone === 'idle' ? t('landing.status.empty') : t(`landing.status.${summary.level}`)}
            </span>
          </div>
          {available && (
            <p className="mt-2 text-xs text-muted-foreground">
              {minutesAgo < 1
                ? t('landing.status.updatedJust')
                : t('landing.status.updatedMin', { count: minutesAgo })}
            </p>
          )}
        </RevealItem>

        {query.isLoading && !data ? (
          <div className="space-y-3">
            {[0, 1, 2].map((i) => (
              <Skeleton key={i} className="h-36 w-full rounded-2xl" />
            ))}
          </div>
        ) : nodes.length === 0 ? (
          <RevealItem>
            <p className="cabinet-elevated-card px-4 py-8 text-sm text-muted-foreground">{t('landing.status.emptyHint')}</p>
          </RevealItem>
        ) : (
          <div className="grid items-start gap-3 sm:grid-cols-2">
            {nodes.map((node, index) => {
              const key = node.id || `${node.country}-${node.name}-${index}`
              return (
              <LocationCard
                key={key}
                node={node}
                locale={i18n.language}
                bypassProbes={data?.bypass_probes !== false}
                ruFailThreshold={data?.ru_fail_threshold}
                open={open === key}
                onToggle={() => {
                  setOpen((current) => (current === key ? null : key))
                }}
              />
              )
            })}
          </div>
        )}
      </PageReveal>
    </AppLayout>
  )
}

function LocationCard({
  node,
  locale,
  bypassProbes,
  ruFailThreshold,
  open,
  onToggle,
}: {
  bypassProbes: boolean
  ruFailThreshold?: number
  node: StatusMapNode
  locale: string
  open: boolean
  onToggle: () => void
}) {
  const { t, i18n } = useTranslation()
  const probe = node.probe
  const whitelist = Boolean(node.whitelist)
  const hits = whitelist ? whitelistHits(probe) : russiaHits(probe)
  const history = probe?.history ?? []
  const measured = history.filter((value): value is number => typeof value === 'number')
  const pct = measured.length
    ? Math.round((measured.reduce((sum, value) => sum + value, 0) / measured.length) * 1000) / 10
    : null
  const note = node.note?.trim()
  // Подпись только та, что задал админ. Название страны сюда не подставляем.
  const subtitle = note

  return (
      <div className={cn('cabinet-elevated-card overflow-hidden', stateTint(node))}>
        <button type="button" className="w-full px-4 py-4 text-left" aria-expanded={open} onClick={onToggle}>
          <span className="flex items-center gap-3">
            <CountryFlag code={node.country} className="h-5 w-7 rounded-[3px]" />
            <span className="min-w-0 flex-1">
              <span className="flex items-center gap-2">
                <span className="truncate font-heading text-base font-bold">{node.name}</span>
                <StateChip node={node} />
              </span>
              {subtitle ? <span className="mt-0.5 block truncate text-sm text-muted-foreground">{subtitle}</span> : null}
            </span>
            <ChevronDown className={cn('size-4 shrink-0 text-muted-foreground transition-transform', open && 'rotate-180')} />
          </span>

          <span className="mt-3 grid grid-cols-2 gap-2">
            <Metric label={t('landing.status.colWorld')}>
              <WorldLine node={node} />
            </Metric>
            <Metric label={whitelist ? t('landing.status.whitelistProbes') : t('landing.status.colRussia')}>
              <RussiaLine
                probe={probe}
                whitelist={whitelist}
                threshold={ruFailThreshold}
                skipped={Boolean(node.no_probe) || (whitelist && !bypassProbes)}
              />
            </Metric>
          </span>

          <span className="mt-3 block">
            <span className="mb-1.5 flex items-center justify-between gap-2 text-[11px] text-muted-foreground">
              <span>{t('landing.status.colHistory')}</span>
              {pct != null ? (
                <span>
                  {t('landing.status.historyPct', {
                    pct: pct.toLocaleString(i18n.language, { maximumFractionDigits: 1 }),
                    days: measured.length,
                  })}
                </span>
              ) : (
                <span>{t('landing.status.historyEmpty')}</span>
              )}
            </span>
            <span className="flex h-5 items-end gap-px" aria-hidden>
              {Array.from({ length: 30 }, (_, index) => (
                <i key={index} className={cn('h-full min-w-0 flex-1 rounded-[2px]', barTone(historyLevel(history[index])))} />
              ))}
            </span>
          </span>
        </button>

        {open && (
          <div className="border-t border-border/60 px-4 py-3">
            {hits.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t('landing.status.providersPending')}</p>
            ) : (
              <ul className="space-y-2">
                {hits.map((hit, index) => (
                  <li key={`${hit.network}-${hit.city}-${index}`} className="flex items-start justify-between gap-3 text-sm">
                    <span className="min-w-0">
                      <span className="block font-medium">{whitelist ? hit.network || '—' : hit.city || '—'}</span>
                      <span className="block text-muted-foreground">
                        {whitelist
                          ? `ICMP ${protocolText(hit.icmp_ok, hit.icmp_ms, t('landing.status.whitelistUp'), t('landing.status.protocolNo'), t('landing.status.hitPing', { ms: hit.icmp_ms }))} · TCP ${protocolText(hit.tcp_ok, hit.tcp_ms, t('landing.status.whitelistUp'), t('landing.status.protocolNo'), t('landing.status.hitPing', { ms: hit.tcp_ms }))}`
                          : hit.network || '—'}
                      </span>
                    </span>
                    <span className="inline-flex shrink-0 items-center gap-1.5 pt-0.5">
                      <span className={cn('size-2 rounded-full', hit.ok ? 'bg-emerald-400' : 'bg-rose-400')} />
                      <span>
                        {hit.ok
                          ? hit.ping_ms != null
                            ? t('landing.status.hitPing', { ms: hit.ping_ms })
                            : whitelist
                              ? t('landing.status.whitelistUp')
                              : t('landing.status.hitUp')
                          : t('landing.status.hitDown')}
                      </span>
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}
      </div>
  )
}

function Metric({ label, children }: { label: string; children: ReactNode }) {
  return (
    <span className="block rounded-xl bg-muted/60 px-3 py-2">
      <span className="block text-[11px] font-medium uppercase tracking-wide text-muted-foreground">{label}</span>
      <span className="mt-1 block text-sm font-medium">{children}</span>
    </span>
  )
}

function WorldLine({ node }: { node: StatusMapNode }) {
  const { t } = useTranslation()
  const probe = node.probe
  if (node.no_probe) return <span className="text-muted-foreground">—</span>
  if (!probe || probe.world_total === 0) return <span className="text-muted-foreground">{t('landing.status.worldPending')}</span>
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={cn('size-2 rounded-full', probe.world_ok > 0 ? 'bg-emerald-400' : 'bg-rose-400')} />
      {probe.world_ok > 0 ? t('landing.status.worldUp') : t('landing.status.worldDown')}
    </span>
  )
}

const RUSSIA_DOT: Record<RowState, string> = {
  up: 'bg-emerald-400',
  partial: 'bg-amber-400',
  down: 'bg-rose-400',
}

function RussiaLine({
  probe,
  whitelist,
  threshold,
  skipped,
}: {
  probe?: StatusProbe
  whitelist: boolean
  threshold?: number
  skipped: boolean
}) {
  const { t } = useTranslation()
  const operators = whitelistHits(probe)
  const total = whitelist ? operators.length : (probe?.russia_total ?? 0)
  const ok = whitelist ? operators.filter((hit) => hit.ok).length : (probe?.russia_ok ?? 0)
  const pings = operators.map((hit) => hit.ping_ms).filter((ms): ms is number => typeof ms === 'number')
  const avg = pings.length ? Math.round(pings.reduce((sum, ms) => sum + ms, 0) / pings.length) : null
  if (skipped) return <span className="text-muted-foreground">—</span>
  if (!probe || total === 0) return <span className="text-muted-foreground">{t('landing.status.worldPending')}</span>
  const tone = russiaTone(ok, total, whitelist, threshold)
  return (
    <span>
      <span className="inline-flex items-center gap-1.5">
        <span className={cn('size-2 rounded-full', RUSSIA_DOT[tone])} />
        {russiaLabel(t, tone, whitelist)}
      </span>
      <span className="mt-0.5 block text-xs font-normal text-muted-foreground">
        {whitelist ? t('landing.status.whitelistRatio', { ok, total }) : t('landing.status.russiaRatio', { ok, total })}
        {whitelist && avg != null ? ` · ${t('landing.status.hitPing', { ms: avg })}` : ''}
      </span>
    </span>
  )
}

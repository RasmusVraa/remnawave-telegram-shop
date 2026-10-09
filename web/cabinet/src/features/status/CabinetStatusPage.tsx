import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Activity, ChevronDown } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { Skeleton } from '@/components/ui/skeleton'
import { CountryFlag } from '@/features/admin/components/CountryFlag'
import type { StatusMapNode, StatusProbe, StatusProbeHit } from '@/features/landing/components/StatusMap'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

const WHITELIST_CITIES = ['Москва', 'Санкт-Петербург', 'Новосибирск', 'Инкогнито']
const WHITELIST_NETWORKS = ['Ростелеком', 'T2', 'МТС', 'Мегафон', 'Билайн']

function countryName(code: string | undefined, locale: string): string {
  if (!code || code.length !== 2) return ''
  try {
    return new Intl.DisplayNames([locale], { type: 'region' }).of(code) ?? ''
  } catch {
    return ''
  }
}

function historyLevel(value: number | null | undefined): string {
  if (value == null || Number.isNaN(value)) return ''
  if (value >= 0.99) return 'up'
  if (value > 0) return 'partial'
  return 'down'
}

function listRank(order: string[], value: string | undefined) {
  const index = order.indexOf(value ?? '')
  return index < 0 ? order.length : index
}

function russiaHits(probe?: StatusProbe): StatusProbeHit[] {
  return (probe?.hits ?? []).filter((hit) => (hit.country ?? '').toUpperCase() === 'RU' && hit.kind !== 'whitelist')
}

function whitelistHits(probe?: StatusProbe): StatusProbeHit[] {
  return (probe?.hits ?? [])
    .filter((hit) => hit.kind === 'whitelist')
    .sort((a, b) => {
      const city = listRank(WHITELIST_CITIES, a.city) - listRank(WHITELIST_CITIES, b.city)
      if (city !== 0) return city
      const cityName = (a.city ?? '').localeCompare(b.city ?? '', 'ru')
      if (cityName !== 0) return cityName
      const network = listRank(WHITELIST_NETWORKS, a.network) - listRank(WHITELIST_NETWORKS, b.network)
      if (network !== 0) return network
      return (a.network ?? '').localeCompare(b.network ?? '', 'ru')
    })
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
  const down = nodes.filter((node) => node.state === 'down').length
  const available = data?.available === true
  const minutesAgo = minutesSince(newestMeasuredAt(nodes) ?? data?.updated_at)
  const tone = !available || total === 0 ? 'idle' : down === 0 ? 'up' : 'partial'

  return (
    <AppLayout>
      <PageReveal className="space-y-4">
        <RevealItem>
          <div>
            <h1 className="font-heading text-2xl font-bold tracking-tight sm:text-3xl">{t('cabinetStatus.title')}</h1>
            <p className="mt-1 max-w-xl text-sm text-muted-foreground">{t('cabinetStatus.lead')}</p>
            <span
              className={cn(
                'mt-3 inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold',
                tone === 'up' && 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300',
                tone === 'partial' && 'bg-amber-500/15 text-amber-800 dark:text-amber-200',
                tone === 'idle' && 'bg-muted text-muted-foreground',
              )}
            >
              <Activity className="size-3.5" aria-hidden />
              {tone === 'up'
                ? t('landing.status.allUp')
                : tone === 'partial'
                  ? t('landing.status.partial')
                  : t('landing.status.empty')}
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
          <div className="grid gap-3 sm:grid-cols-2">
            {nodes.map((node) => (
              <LocationCard
                key={`${node.country}-${node.name}`}
                node={node}
                locale={i18n.language}
                open={open === `${node.country}-${node.name}`}
                onToggle={() => {
                  const key = `${node.country}-${node.name}`
                  setOpen((current) => (current === key ? null : key))
                }}
              />
            ))}
          </div>
        )}
      </PageReveal>
    </AppLayout>
  )
}

function LocationCard({
  node,
  locale,
  open,
  onToggle,
}: {
  node: StatusMapNode
  locale: string
  open: boolean
  onToggle: () => void
}) {
  const { t, i18n } = useTranslation()
  const region = countryName(node.country, locale)
  const probe = node.probe
  const whitelist = Boolean(node.whitelist) || whitelistHits(probe).length > 0
  const hits = whitelist ? whitelistHits(probe) : russiaHits(probe)
  const history = probe?.history ?? []
  const measured = history.filter((value): value is number => typeof value === 'number')
  const pct = measured.length
    ? Math.round((measured.reduce((sum, value) => sum + value, 0) / measured.length) * 1000) / 10
    : null
  const note = node.note?.trim()
  const subtitle = note || (region && region !== node.name ? region : '')

  return (
      <div className="cabinet-elevated-card overflow-hidden">
        <button type="button" className="w-full px-4 py-4 text-left" aria-expanded={open} onClick={onToggle}>
          <span className="flex items-center gap-3">
            <CountryFlag code={node.country} className="h-5 w-7 rounded-[3px]" />
            <span className="min-w-0 flex-1">
              <span className="block truncate font-heading text-base font-bold">{node.name}</span>
              {subtitle ? <span className="mt-0.5 block truncate text-sm text-muted-foreground">{subtitle}</span> : null}
            </span>
            <ChevronDown className={cn('size-4 shrink-0 text-muted-foreground transition-transform', open && 'rotate-180')} />
          </span>

          <span className="mt-3 grid grid-cols-2 gap-2">
            <Metric label={t('landing.status.colWorld')}>
              <WorldLine probe={probe} />
            </Metric>
            <Metric label={t('landing.status.colRussia')}>
              <RussiaLine probe={probe} whitelist={whitelist} />
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
                  <li key={`${hit.city}-${hit.network}-${index}`} className="flex items-start justify-between gap-3 text-sm">
                    <span className="min-w-0">
                      <span className="block font-medium">{hit.city || '—'}</span>
                      <span className="block text-muted-foreground">{hit.network || '—'}</span>
                    </span>
                    <span className="inline-flex shrink-0 items-center gap-1.5 pt-0.5">
                      <span className={cn('size-2 rounded-full', hit.ok ? 'bg-emerald-400' : 'bg-rose-400')} />
                      <span>
                        {whitelist
                          ? hit.ok
                            ? t('landing.status.inWhitelist')
                            : t('landing.status.hitDown')
                          : hit.ok
                            ? hit.ping_ms != null
                              ? t('landing.status.hitPing', { ms: hit.ping_ms })
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

function WorldLine({ probe }: { probe?: StatusProbe }) {
  const { t } = useTranslation()
  if (!probe || probe.world_total === 0) return <span className="text-muted-foreground">{t('landing.status.worldPending')}</span>
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={cn('size-2 rounded-full', probe.world_ok > 0 ? 'bg-emerald-400' : 'bg-rose-400')} />
      {probe.world_ok > 0 ? t('landing.status.worldUp') : t('landing.status.worldDown')}
    </span>
  )
}

function RussiaLine({ probe, whitelist }: { probe?: StatusProbe; whitelist: boolean }) {
  const { t } = useTranslation()
  const operators = whitelistHits(probe)
  const total = whitelist ? operators.length : (probe?.russia_total ?? 0)
  const ok = whitelist ? operators.filter((hit) => hit.ok).length : (probe?.russia_ok ?? 0)
  if (!probe || total === 0) return <span className="text-muted-foreground">{t('landing.status.worldPending')}</span>
  return (
    <span>
      <span className="inline-flex items-center gap-1.5">
        <span className={cn('size-2 rounded-full', ok > 0 ? 'bg-emerald-400' : 'bg-rose-400')} />
        {whitelist ? t('landing.status.inWhitelist') : ok > 0 ? t('landing.status.russiaUp') : t('landing.status.russiaDown')}
      </span>
      <span className="mt-0.5 block text-xs font-normal text-muted-foreground">
        {t('landing.status.russiaRatio', { ok, total })}
      </span>
    </span>
  )
}

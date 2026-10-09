import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown } from 'lucide-react'

import { CountryFlag } from '@/features/admin/components/CountryFlag'
import { cn } from '@/lib/utils'
import type { StatusMapNode, StatusProbe, StatusProbeHit } from './StatusMap'

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

function russiaHits(probe?: StatusProbe): StatusProbeHit[] {
  return (probe?.hits ?? []).filter((hit) => (hit.country ?? '').toUpperCase() === 'RU' && hit.kind !== 'whitelist')
}

const WHITELIST_CITIES = ['Москва', 'Санкт-Петербург', 'Новосибирск', 'Инкогнито']
const WHITELIST_NETWORKS = ['Ростелеком', 'T2', 'МТС', 'Мегафон', 'Билайн']

function listRank(order: string[], value: string | undefined) {
  const index = order.indexOf(value ?? '')
  return index < 0 ? order.length : index
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

/** Строки локаций: раскрытие и таблица зондов, с которых шёл пинг. */
export function StatusNodeList({ nodes }: { nodes: StatusMapNode[] }) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState<string | null>(null)
  const [providers, setProviders] = useState<string | null>(null)

  return (
    <div className="landing-card mt-6">
      <div className="hidden grid-cols-[1.2fr_1fr_1.1fr_1.3fr_auto] gap-3 border-b border-border/70 px-4 py-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground md:grid sm:px-6">
        <span>{t('landing.status.colLocation')}</span>
        <span>{t('landing.status.colWorld')}</span>
        <span>{t('landing.status.colRussia')}</span>
        <span>{t('landing.status.colHistory')}</span>
        <span className="sr-only">{t('landing.status.expand')}</span>
      </div>
      {nodes.length === 0 ? (
        <p className="px-4 py-8 text-sm text-muted-foreground sm:px-6">{t('landing.status.emptyHint')}</p>
      ) : (
        <ul>
          {nodes.map((node) => {
            const key = `${node.country}-${node.name}`
            const region = countryName(node.country, i18n.language)
            const probe = node.probe
            const hits = russiaHits(probe)
            const whitelist = whitelistHits(probe)
            const isWhitelist = Boolean(node.whitelist) || whitelist.length > 0
            const expanded = open === key
            const showHits = providers === key
            const history = probe?.history ?? []
            const measured = history.filter((v): v is number => typeof v === 'number')
            const pct = measured.length
              ? Math.round((measured.reduce((sum, v) => sum + v, 0) / measured.length) * 1000) / 10
              : null
            const note = node.note?.trim()
            return (
              <li key={key} className="border-b border-border/50 last:border-b-0">
                <button
                  type="button"
                  className="grid w-full grid-cols-1 gap-4 px-4 py-4 text-left md:grid-cols-[1.2fr_1fr_1.1fr_1.3fr_auto] md:items-center md:gap-3 sm:px-6"
                  aria-expanded={expanded}
                  onClick={() => {
                    setOpen(expanded ? null : key)
                    if (expanded) setProviders(null)
                  }}
                >
                  <span className="flex items-center gap-3">
                    <CountryFlag code={node.country} className="h-5 w-7 rounded-[3px]" />
                    <span>
                      <span className="block font-heading text-base font-bold">{node.name}</span>
                      {note ? <span className="mt-0.5 block text-sm text-muted-foreground">{note}</span> : null}
                      {!note && region && region !== node.name ? (
                        <span className="mt-0.5 block text-sm text-muted-foreground">{region}</span>
                      ) : null}
                    </span>
                  </span>
                  <ProbeWorld probe={probe} />
                  <ProbeRussia probe={probe} whitelist={isWhitelist} />
                  <History history={history} measured={measured} pct={pct} />
                  <ChevronDown className={cn('hidden size-4 text-muted-foreground transition-transform md:block', expanded && 'rotate-180')} />
                </button>
                {expanded && !isWhitelist && (
                  <div className="flex flex-col gap-3 px-4 pb-4 sm:flex-row sm:items-center sm:justify-between sm:px-6">
                    <p className="text-sm text-muted-foreground">
                      {probe && probe.russia_total > 0
                        ? probe.russia_ok === probe.russia_total
                          ? t('landing.status.providersLeadAll', { total: probe.russia_total })
                          : t('landing.status.providersLeadSome', { ok: probe.russia_ok, total: probe.russia_total })
                        : t('landing.status.providersPending')}
                    </p>
                    {hits.length > 0 && (
                      <button
                        type="button"
                        className="landing-cta landing-cta--ghost inline-flex h-10 shrink-0 items-center justify-center rounded-full px-4 text-sm font-semibold"
                        onClick={() => setProviders(showHits ? null : key)}
                      >
                        {showHits
                          ? t('landing.status.providersHide')
                          : t('landing.status.providersOpen', { count: hits.length })}
                      </button>
                    )}
                  </div>
                )}
                {expanded && isWhitelist && (
                  <div className="overflow-x-auto px-4 pb-4 sm:px-6">
                    {whitelist.length === 0 ? (
                      <p className="text-sm text-muted-foreground">{t('landing.status.worldPending')}</p>
                    ) : (
                      <table className="w-full min-w-[28rem] text-left text-sm">
                        <thead className="text-xs uppercase tracking-wide text-muted-foreground">
                          <tr>
                            <th className="py-2 pr-3 font-semibold">{t('landing.status.colCity')}</th>
                            <th className="py-2 pr-3 font-semibold">{t('landing.status.colProvider')}</th>
                            <th className="py-2 font-semibold">{t('landing.status.colAnswer')}</th>
                          </tr>
                        </thead>
                        <tbody>
                          {whitelist.map((hit, index) => (
                            <tr key={`${hit.network}-${hit.city}-${index}`} className="border-t border-border/40">
                              <td className="py-2 pr-3">{hit.city || '—'}</td>
                              <td className="py-2 pr-3">{hit.network || '—'}</td>
                              <td className="py-2">
                                <span className="inline-flex items-start gap-2">
                                  <span className={cn('landing-status-dot mt-1.5 shrink-0', hit.ok ? 'landing-status-dot--up' : 'landing-status-dot--down')} />
                                  <span>{hit.ok ? t('landing.status.inWhitelist') : t('landing.status.hitDown')}</span>
                                </span>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    )}
                  </div>
                )}
                {expanded && !isWhitelist && showHits && (
                  <div className="overflow-x-auto px-4 pb-4 sm:px-6">
                    <table className="w-full min-w-[36rem] text-left text-sm">
                      <thead className="text-xs uppercase tracking-wide text-muted-foreground">
                        <tr>
                          <th className="py-2 pr-3 font-semibold">{t('landing.status.colCity')}</th>
                          <th className="py-2 pr-3 font-semibold">{t('landing.status.colProvider')}</th>
                          <th className="py-2 font-semibold">{t('landing.status.colResult')}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {hits.map((hit, index) => (
                          <tr key={`${hit.city}-${hit.network}-${index}`} className="border-t border-border/40">
                            <td className="py-2 pr-3">{hit.city || '—'}</td>
                            <td className="py-2 pr-3 text-muted-foreground">{hit.network || '—'}</td>
                            <td className="py-2">
                              <span className="inline-flex items-center gap-2">
                                <span className={cn('landing-status-dot', hit.ok ? 'landing-status-dot--up' : 'landing-status-dot--down')} />
                                {hit.ok
                                  ? hit.ping_ms != null
                                    ? t('landing.status.hitPing', { ms: hit.ping_ms })
                                    : t('landing.status.hitUp')
                                  : t('landing.status.hitDown')}
                              </span>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

function ProbeWorld({ probe }: { probe?: StatusProbe }) {
  const { t } = useTranslation()
  return (
    <span>
      <span className="mb-1 block text-[11px] font-semibold uppercase tracking-wide text-muted-foreground md:hidden">
        {t('landing.status.colWorld')}
      </span>
      {!probe || probe.world_total === 0 ? (
        <span className="text-sm text-muted-foreground">{t('landing.status.worldPending')}</span>
      ) : (
        <span className="block">
          <span className="inline-flex items-center gap-2 text-sm font-medium">
            <span className={cn('landing-status-dot', probe.world_ok > 0 ? 'landing-status-dot--up' : 'landing-status-dot--down')} />
            {probe.world_ok > 0 ? t('landing.status.worldUp') : t('landing.status.worldDown')}
          </span>
        </span>
      )}
    </span>
  )
}

function ProbeRussia({ probe, whitelist }: { probe?: StatusProbe; whitelist?: boolean }) {
  const { t } = useTranslation()
  const operators = whitelistHits(probe)
  const total = whitelist ? operators.length : (probe?.russia_total ?? 0)
  const ok = whitelist ? operators.filter((hit) => hit.ok).length : (probe?.russia_ok ?? 0)
  return (
    <span>
      <span className="mb-1 block text-[11px] font-semibold uppercase tracking-wide text-muted-foreground md:hidden">
        {t('landing.status.colRussia')}
      </span>
      {!probe || total === 0 ? (
        <span className="text-sm text-muted-foreground">{t('landing.status.worldPending')}</span>
      ) : (
        <span className="block">
          <span className="inline-flex items-center gap-2 text-sm font-medium">
            <span className={cn('landing-status-dot', ok > 0 ? 'landing-status-dot--up' : 'landing-status-dot--down')} />
            {whitelist ? t('landing.status.inWhitelist') : ok > 0 ? t('landing.status.russiaUp') : t('landing.status.russiaDown')}
          </span>
          <span className="mt-0.5 block text-sm text-muted-foreground">
            {t('landing.status.russiaRatio', { ok, total })}
          </span>
        </span>
      )}
    </span>
  )
}

function History({
  history,
  measured,
  pct,
}: {
  history: Array<number | null>
  measured: number[]
  pct: number | null
}) {
  const { t } = useTranslation()
  return (
    <span>
      <span className="mb-1 block text-[11px] font-semibold uppercase tracking-wide text-muted-foreground md:hidden">
        {t('landing.status.colHistory')}
      </span>
      {measured.length === 0 ? (
        <span className="text-sm text-muted-foreground">{t('landing.status.historyEmpty')}</span>
      ) : (
        <span className="block">
          <span className="landing-status-bar" aria-hidden>
            {Array.from({ length: 30 }, (_, i) => (
              <i key={i} data-level={historyLevel(history[i])} />
            ))}
          </span>
          <span className="mt-1 flex justify-between text-[11px] text-muted-foreground">
            <span>{t('landing.status.historyPast')}</span>
            <span>{t('landing.status.historyPct', { pct, days: measured.length })}</span>
            <span>{t('landing.status.historyNow')}</span>
          </span>
        </span>
      )}
    </span>
  )
}

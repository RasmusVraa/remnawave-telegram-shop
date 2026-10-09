import { useLayoutEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { api } from '@/lib/api'
import { useLandingBrand } from './useLandingBrand'
import { useLandingCopy } from './useLandingCopy'
import { LandingHeader } from './components/LandingHeader'
import { LandingFooter } from './components/LandingFooter'
import { Rise } from './components/LandingMotion'
import { StatusMap, type StatusMapNode, type StatusProbe } from './components/StatusMap'
import { StatusNodeList } from './components/StatusNodeList'
import './landing.css'

interface StatusNode extends StatusMapNode {
  probe?: StatusProbe
}

interface StatusResponse {
  available?: boolean
  updated_at?: string
  online?: number
  total?: number
  show_map?: boolean
  probes_enabled?: boolean
  probe_world?: number
  probe_russia?: number
  probe_interval_min?: number
  title?: string
  lead?: string
  nodes?: StatusNode[]
}

/** Публичный статус серверов: /status. Данные — доступность узлов панели, без адресов. */
export default function StatusPage() {
  const { t } = useTranslation()
  const brand = useLandingBrand()
  const copy = useLandingCopy()

  useLayoutEffect(() => {
    const root = document.documentElement
    root.dataset.landing = '1'
    return () => {
      delete root.dataset.landing
    }
  }, [])

  const query = useQuery({
    queryKey: ['public-status'],
    queryFn: () => api.publicStatus(),
    refetchInterval: 30_000,
  })

  const data = query.data as StatusResponse | undefined
  const nodes = data?.nodes ?? []
  const total = data?.total ?? nodes.length
  const down = nodes.filter((n) => n.state === 'down').length
  const minutesAgo = minutesSince(newestMeasuredAt(nodes) ?? data?.updated_at)

  const available = data?.available === true
  const liveTitle = useMemo(() => {
    if (query.isLoading && !data) return t('landing.status.eyebrow')
    if (!available) return t('landing.status.unavailable')
    if (total === 0) return t('landing.status.empty')
    if (down === 0) return t('landing.status.allUp')
    return t('landing.status.partial')
  }, [available, data, down, query.isLoading, t, total])
  const probeWorld = data?.probe_world || 3
  const probeRussia = data?.probe_russia || 20
  const probeMinutes = data?.probe_interval_min || 20
  const title = data?.title?.trim() || liveTitle
  const lead =
    data?.lead?.trim() ||
    (available ? t('landing.status.leadProbes') : t('landing.status.whatBody'))

  return (
    <div className="landing-root">
      <div className="landing-backdrop" aria-hidden>
        <div className="landing-backdrop__wash" />
      </div>
      <div className="relative z-10">
        <LandingHeader brand={brand} copy={copy} />
        <main className="mx-auto max-w-6xl px-4 pb-20 pt-10 sm:px-6 sm:pt-16">
          <Rise>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <h1 className="text-balance font-heading text-4xl font-extrabold tracking-tight sm:text-5xl">
                {title}
              </h1>
              <p className="mt-3 max-w-2xl text-base text-muted-foreground">{lead}</p>
              {data?.probes_enabled === false && (
                <p className="mt-2 max-w-2xl text-sm text-muted-foreground">{t('landing.status.probesOff')}</p>
              )}
            </div>
            {available && (
              <p className="inline-flex items-center gap-2 text-sm text-muted-foreground">
                <span className="landing-status-dot landing-status-dot--up" />
                {minutesAgo < 1
                  ? t('landing.status.updatedJust')
                  : t('landing.status.updatedMin', { count: minutesAgo })}
              </p>
            )}
          </div>
          </Rise>

          {data?.show_map !== false && (
            <Rise delay={0.12}>
              <StatusMap nodes={nodes} />
            </Rise>
          )}

          <StatusNodeList nodes={nodes} />

          <div className="mt-8 grid gap-4 lg:grid-cols-3">
            <Rise delay={0.05} className="h-full">
              <article className="landing-card landing-status-note h-full p-5 sm:p-6">
                <span className="landing-status-mark landing-status-mark--world" aria-hidden />
                <h2 className="mt-4 font-heading text-xl font-bold">{t('landing.status.cardWorldTitle')}</h2>
                <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                  {t('landing.status.cardWorldBody', { count: probeWorld, minutes: probeMinutes })}
                </p>
              </article>
            </Rise>
            <Rise delay={0.12} className="h-full">
              <article className="landing-card landing-status-note h-full p-5 sm:p-6">
                <span className="landing-status-mark landing-status-mark--ru" aria-hidden />
                <h2 className="mt-4 font-heading text-xl font-bold">{t('landing.status.cardRussiaTitle')}</h2>
                <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                  {t('landing.status.cardRussiaBody', { count: probeRussia, minutes: probeMinutes })}
                </p>
              </article>
            </Rise>
            <Rise delay={0.18} className="h-full">
              <article className="landing-card landing-status-note h-full p-5 sm:p-6">
                <h2 className="font-heading text-xl font-bold">{t('landing.status.cardSupportTitle')}</h2>
                <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{t('landing.status.cardSupportBody')}</p>
                {brand.supportUrl && (
                  <a
                    href={brand.supportUrl}
                    className="landing-cta landing-cta--primary mt-5 inline-flex h-11 items-center justify-center rounded-full px-5 text-sm font-semibold"
                  >
                    {t('landing.status.cardSupportCta')}
                  </a>
                )}
              </article>
            </Rise>
          </div>
        </main>
        <LandingFooter brand={brand} />
      </div>
    </div>
  )
}

function newestMeasuredAt(nodes: StatusNode[]): string | undefined {
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

function minutesSince(iso: string | undefined): number {
  if (!iso) return 0
  const ts = Date.parse(iso)
  if (Number.isNaN(ts)) return 0
  return Math.max(0, Math.round((Date.now() - ts) / 60000))
}

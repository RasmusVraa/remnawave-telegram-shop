import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ArrowRight, ExternalLink } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { CountryFlag } from '@/features/admin/components/CountryFlag'
import { displayState, summarizeStatus, type RowState } from '@/features/status/statusState'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

const MAX_ROWS = 5
const ORDER: Record<string, number> = { down: 0, partial: 1, up: 2 }

const DOT: Record<RowState, string> = {
  up: 'bg-emerald-500',
  partial: 'bg-amber-500',
  down: 'bg-rose-500',
}

/** Живая сводка по серверам для раздела «Статус» на странице поддержки. */
export function useServerStatus() {
  return useQuery({
    queryKey: ['public-status'],
    queryFn: () => api.publicStatus(),
    refetchInterval: 60_000,
    staleTime: 30_000,
  })
}

export function ServerStatusSummary({ externalUrl }: { externalUrl?: string }) {
  const { t } = useTranslation()
  const { data, isLoading } = useServerStatus()
  const nodes = (data?.nodes ?? []).map((node) => ({ node, state: displayState(node) }))

  if (isLoading && !data) {
    return (
      <div className="space-y-2" aria-hidden>
        <Skeleton className="h-6 w-56 rounded-md" />
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} className="h-10 w-full rounded-lg" />
        ))}
      </div>
    )
  }

  if (nodes.length === 0) {
    return externalUrl ? (
      <Button asChild>
        <a href={externalUrl} target="_blank" rel="noopener noreferrer">
          {t('infoStatus.openExternal')}
          <ExternalLink className="size-4" />
        </a>
      </Button>
    ) : (
      <p className="text-sm text-muted-foreground">{t('infoStatus.unavailable')}</p>
    )
  }

  const { level, tone } = summarizeStatus((data?.nodes ?? []))
  const title = t(`infoStatus.${level}`)

  const sorted = [...nodes].sort((a, b) => (ORDER[a.state ?? 'up'] ?? 2) - (ORDER[b.state ?? 'up'] ?? 2))
  const shown = sorted.slice(0, MAX_ROWS)
  const rest = sorted.length - shown.length

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2.5">
        <span className={cn('size-3 rounded-full', DOT[tone])} />
        <p className="text-base font-semibold">{title}</p>
      </div>

      <ul className="divide-y divide-border/50 overflow-hidden rounded-xl border border-border/60">
        {shown.map(({ node, state }, index) => (
          <li key={node.id ?? `${node.name}-${index}`} className="flex items-center gap-3 px-3.5 py-2.5">
            <CountryFlag code={node.country} className="h-4 w-6 shrink-0 rounded-[3px]" />
            <span className="min-w-0 flex-1 truncate text-sm font-medium">{node.name}</span>
            <span className="inline-flex items-center gap-2 text-sm">
              <span className={cn('size-2 rounded-full', state ? DOT[state] : 'bg-muted-foreground/40')} />
              <span className={cn(state === 'down' && 'font-medium text-rose-500', !state && 'text-muted-foreground')}>
                {state ? t(`landing.status.row.${state}`) : t('infoStatus.counting')}
              </span>
            </span>
          </li>
        ))}
      </ul>

      <p className="text-sm text-muted-foreground">
        {tone === 'up' ? t('infoStatus.hintOk') : t('infoStatus.hintProblems')}
      </p>

      <Button asChild variant="outline">
        <Link to="/status">
          {rest > 0 ? t('infoStatus.allWithMore', { count: rest }) : t('infoStatus.all')}
          <ArrowRight className="size-4" />
        </Link>
      </Button>
    </div>
  )
}

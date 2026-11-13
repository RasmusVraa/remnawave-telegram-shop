import { useTranslation } from 'react-i18next'

import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { cn, formatDate } from '@/lib/utils'
import type { TrafficUsageResponse } from '@/lib/api'
import { ChartLegend, UsageChart, chartWindow } from '@/features/home/HomeParts'

/**
 * График расхода трафика за расчётный период.
 *
 * Рисуется чистым SVG: recharts весит около полумегабайта и живёт только в
 * админке — тянуть его на главную ради одной ломаной незачем.
 *
 * Показывается и на безлимитном тарифе: график про динамику по дням, а не про
 * долю от лимита, знаменатель ему не нужен. Для безлимита это вообще
 * единственная информация о трафике — полосы у него нет.
 */
export function TrafficUsageChart({
  data,
  loading,
  lang,
  className,
  bypassLabel,
}: {
  data: TrafficUsageResponse | undefined
  loading: boolean
  lang: string
  className?: string
  /** Название обхода для легенды (есть, когда у клиента отдельный лимит обхода). */
  bypassLabel?: string
}) {
  const { t } = useTranslation()

  if (loading) {
    return (
      <Card className={cn('cabinet-elevated-card', className)}>
        <CardContent className="px-4 py-4">
          <div className="flex items-start justify-between gap-2">
            <div className="space-y-1.5">
              <Skeleton className="h-4 w-32" />
              <Skeleton className="h-3 w-24" />
            </div>
            <Skeleton className="h-4 w-16" />
          </div>
          <Skeleton className="mt-3 h-16 w-full rounded-lg" />
        </CardContent>
      </Card>
    )
  }

  /*
   * Карточку показываем, только когда есть что рисовать.
   *
   * Точек меньше двух — линию не построить (шаг считается как 100/(n-1)).
   * Все точки нулевые — линия ложится по нулю и читается как поломка графика,
   * хотя на деле человек просто ещё не расходовал трафик. Проверяем именно
   * точки, а не total_bytes: панель может не отдать сумму, а ряд отдать.
   */
  if (!data?.enabled || data.points.length < 1) return null
  if (!data.points.some((p) => p > 0)) return null
  const win = chartWindow(data)

  return (
    <Card className={cn('cabinet-elevated-card', className)}>
      <CardContent className="px-4 py-4">
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            <p className="text-sm font-semibold">{t('dashboard.usageChartTitle')}</p>
            {data.period_start && (
              <p className="text-xs text-muted-foreground">
                {t('dashboard.usageChartSince', { date: formatDate(data.period_start, lang) })}
              </p>
            )}
          </div>
          <span className="shrink-0 text-sm font-semibold tabular-nums">
            {formatGb(data.total_bytes)} {t('dashboard.gigabytes')}
          </span>
        </div>
        {/* Те же столбики по дням, что в шторке «Расход трафика»: на ПК раньше рисовалась старая линия. */}
        <div className="mt-3 space-y-3">
          <UsageChart points={win.points} cats={win.cats} pad={win.pad} future={win.future} lang={lang} bypass={win.bypass} compact />
          {win.bypass && <ChartLegend label={bypassLabel} points={win.points} bypass={win.bypass} />}
        </div>
      </CardContent>
    </Card>
  )
}

function formatGb(bytes: number): string {
  const gb = bytes / 1024 ** 3
  if (gb >= 100) return String(Math.round(gb))
  return gb.toFixed(1)
}

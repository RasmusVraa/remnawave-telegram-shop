import { useTranslation } from 'react-i18next'
import { Megaphone } from 'lucide-react'

import type { AdminChannelGateDTO } from '@/lib/types/admin'

import { statsNumberLocale } from '../utils/statsFormat'
import { STATS_ACCENT } from '../utils/statsPalette'
import { StatsBar, StatsFootnote, StatsPanel, StatsPanelHead, StatsStatRow } from './StatsPanel'

interface StatsChannelGateBlockProps {
  gate: AdminChannelGateDTO
  className?: string
}

/** Блок показываем, когда функция включена либо за период экран подписки всё же показывался. */
export function shouldShowChannelGate(gate: AdminChannelGateDTO | null | undefined): gate is AdminChannelGateDTO {
  return Boolean(gate && (gate.enabled || gate.shown > 0 || gate.old_not_passed > 0))
}

/**
 * «Подписка на канал»: сколько новых клиентов увидели экран подписки, сколько
 * подписались и сколько ушли, не подписавшись. Старые клиенты — отдельной
 * строкой: у них VPN работает, «ушедшими» они не считаются.
 */
export function StatsChannelGateBlock({ gate, className }: StatsChannelGateBlockProps) {
  const { t, i18n } = useTranslation()
  const numberLocale = statsNumberLocale(i18n.language)
  const fmt = (n: number) => n.toLocaleString(numberLocale)

  const base = Math.max(gate.shown, 1)
  const pct = (n: number) => Math.round((n * 100) / base)
  const steps = [
    { label: t('admin.stats.channelGateShown'), value: gate.shown, color: STATS_ACCENT.blue, note: null as string | null },
    {
      label: t('admin.stats.channelGatePassed'),
      value: gate.passed,
      color: STATS_ACCENT.green,
      note: gate.shown > 0 ? t('admin.stats.channelGatePassedPct', { pct: pct(gate.passed) }) : null,
    },
    {
      label: t('admin.stats.channelGateLeft'),
      value: gate.not_passed,
      color: STATS_ACCENT.red,
      note: gate.shown > 0 ? `${pct(gate.not_passed)}%` : null,
    },
  ]

  return (
    <StatsPanel className={className}>
      <StatsPanelHead
        icon={Megaphone}
        color={STATS_ACCENT.violet}
        title={t('admin.stats.channelGateTitle')}
        subtitle={
          gate.channel
            ? t('admin.stats.channelGateSubtitle', { channel: gate.channel })
            : t('admin.stats.channelGateSubtitleNoChannel')
        }
        actions={
          !gate.enabled ? (
            <span className="rounded-full border border-border bg-muted/60 px-2.5 py-0.5 text-xs font-semibold text-muted-foreground">
              {t('admin.stats.channelGateOff')}
            </span>
          ) : undefined
        }
      />

      <div className="grid gap-x-9 gap-y-5 lg:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]">
        <div className="flex flex-col gap-3.5">
          {steps.map((step) => (
            <div key={step.label} className="flex flex-col gap-1.5">
              <div className="flex items-baseline justify-between gap-3 text-[13px]">
                <span className="min-w-0 truncate">{step.label}</span>
                <span className="shrink-0">
                  <span className="font-semibold tabular-nums">{fmt(step.value)}</span>
                  {step.note && <span className="ml-1.5 tabular-nums text-muted-foreground">{step.note}</span>}
                </span>
              </div>
              <StatsBar percent={(step.value * 100) / base} color={step.color} />
            </div>
          ))}
        </div>

        <div className="lg:-mt-2.5">
          <StatsStatRow label={t('admin.stats.channelGateTried')} value={fmt(gate.tried_failed)} />
          <StatsStatRow
            label={`${t('admin.stats.channelGateThinking')} ${t('admin.stats.channelGateThinkingHint')}`}
            value={fmt(gate.in_progress)}
          />
          <StatsStatRow
            label={`${t('admin.stats.channelGateOld')} ${t('admin.stats.channelGateOldHint')}`}
            value={fmt(gate.old_not_passed)}
            last
          />
        </div>
      </div>

      <StatsFootnote>{t('admin.stats.channelGateFootnote')}</StatsFootnote>
    </StatsPanel>
  )
}

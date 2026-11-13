import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, Gauge, Infinity as InfinityIcon, Plus, Route } from 'lucide-react'

import '@/lib/splitColor'
import { ResponsiveSheet } from '@/components/ResponsiveSheet'
import { Button } from '@/components/ui/button'
import { bypassLabel, isDefaultBypassLabel } from '@/lib/bypassLabel'
import { formatUtcDayMonth as formatDayMonth, utcPeriodMonthName } from '@/lib/splitDates'
import type { MyTrafficResponse, TrafficPack } from '@/lib/api'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { hapticSelection } from '@/lib/haptic'
import { trafficTone } from '@/lib/subscriptionTone'
import { formatGbValue, type HomeData } from './useHomeData'

/*
 * Раздельный трафик на главной: окно докупки и блок «обход» в шторке расхода.
 * Окно открывается из трёх мест — плашка и кнопка при исчерпанном обходе, кнопка
 * в шторке и кнопка «Докупить» в уведомлении (?buy=bypass), — поэтому его состояние
 * живёт в контексте, а не в одной из страниц.
 */

type BuyKind = 'main' | 'bypass'

const TrafficBuyContext = createContext<{ open: (kind?: BuyKind) => void; canBuy: boolean }>({
  open: () => {},
  canBuy: false,
})

export function useTrafficBuy() {
  return useContext(TrafficBuyContext)
}

const GB = 1024 ** 3

/** Обход исчерпан: расход дошёл до лимита (сквад воркер снимает с задержкой, а клиент это уже видит). */
export function isBypassBlocked(split: MyTrafficResponse): boolean {
  const b = split.bypass
  return Boolean(split.bypass_blocked) || Boolean(b && b.limit_bytes > 0 && b.used_bytes >= b.limit_bytes)
}

/** Размер пакета в ГБ без лишних нулей: «5», «20», «0,5». */
export function packSizeLabel(bytes: number, unit: string): string {
  return `${formatGbValue(bytes / GB)} ${unit}`
}

export function TrafficBuyProvider({ data, children }: { data: HomeData; children: ReactNode }) {
  const split = data.split
  const [open, setOpen] = useState(false)
  const [kind, setKind] = useState<BuyKind>('bypass')
  const [params, setParams] = useSearchParams()

  const canBuy = Boolean(split?.can_buy)
  const openBuy = useCallback(
    (k?: BuyKind) => {
      if (!canBuy) return
      const kinds = new Set((split?.packs ?? []).map((p) => p.kind))
      const want: BuyKind = k ?? (split?.bypass_blocked ? 'bypass' : kinds.has('bypass') ? 'bypass' : 'main')
      setKind(kinds.has(want) ? want : kinds.has('main') ? 'main' : 'bypass')
      setOpen(true)
    },
    [canBuy, split],
  )

  // Кнопка «Докупить» в уведомлении ведёт на /dashboard?buy=bypass.
  const wanted = params.get('buy')
  useEffect(() => {
    if (!wanted || !split) return
    if (wanted === 'main' || wanted === 'bypass') openBuy(wanted)
    const next = new URLSearchParams(params)
    next.delete('buy')
    setParams(next, { replace: true })
  }, [wanted, split]) // eslint-disable-line react-hooks/exhaustive-deps

  const value = useMemo(() => ({ open: openBuy, canBuy }), [openBuy, canBuy])
  return (
    <TrafficBuyContext.Provider value={value}>
      {children}
      {split && <TrafficBuySheet open={open} onClose={() => setOpen(false)} split={split} kind={kind} setKind={setKind} />}
    </TrafficBuyContext.Provider>
  )
}

function pricePerGb(p: TrafficPack): number {
  return p.amount_rub / (p.bytes / GB)
}

function TrafficBuySheet({
  open,
  onClose,
  split,
  kind,
  setKind,
}: {
  open: boolean
  onClose: () => void
  split: MyTrafficResponse
  kind: BuyKind
  setKind: (k: BuyKind) => void
}) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const [pending, setPending] = useState(false)
  const kinds = (['main', 'bypass'] as BuyKind[]).filter((k) => split.packs.some((p) => p.kind === k))
  const packs = split.packs.filter((p) => p.kind === kind).sort((a, b) => a.bytes - b.bytes)
  const [selected, setSelected] = useState<number | null>(null)
  const unit = t('dashboard.gigabytes')
  const lang = i18n.language

  // При смене вида и при открытии выбираем «популярный» (второй по размеру) пакет.
  useEffect(() => {
    if (!open) return
    setPending(false)
    const mid = packs[Math.min(1, packs.length - 1)]
    setSelected(mid ? mid.id : null)
  }, [open, kind, packs.map((p) => p.id).join(',')]) // eslint-disable-line react-hooks/exhaustive-deps

  const chosen = packs.find((p) => p.id === selected) ?? null
  const base = packs[0] ? pricePerGb(packs[0]) : 0
  const label = bypassLabel(split.label)
  const kindName = (k: BuyKind) => (k === 'bypass' ? label : t('trafficSplit.tabMain'))
  const info = kind === 'bypass' ? split.bypass : split.main
  const until = formatDayMonth(split.reset_at, lang)

  return (
    <ResponsiveSheet
      open={open}
      onClose={onClose}
      centered
      title={t('trafficSplit.buyTitle')}
      subtitle={t('trafficSplit.buySubtitle', { date: until })}
      footer={
        <div className="space-y-2">
          <Button
            className={cn(
              'h-[52px] w-full shrink-0 rounded-full text-base font-bold',
              kind === 'bypass' &&
                'border-0 bg-[hsl(var(--split-bypass-deep))] text-white shadow-[0_10px_30px_hsl(var(--split-bypass-deep)/0.3)] [background-image:linear-gradient(90deg,hsl(var(--split-bypass-deep)),hsl(var(--split-bypass-alt)))] hover:brightness-110',
            )}
            disabled={!chosen}
            loading={pending}
            onClick={() => {
              if (!chosen) return
              setPending(true)
              navigate(`/checkout/traffic?pack=${chosen.id}`)
              onClose()
            }}
          >
            {chosen
              ? t('trafficSplit.buyCta', { size: packSizeLabel(chosen.bytes, unit), price: `${formatNumber(chosen.amount_rub)} ${t('checkout.rub')}` })
              : t('trafficSplit.buy')}
          </Button>
          <p className="text-center text-[13px] text-muted-foreground">{t('trafficSplit.packNote', { date: until })}</p>
        </div>
      }
    >
      <div className="space-y-3.5">
        {kinds.length > 1 && (
          <div className="grid grid-cols-2 gap-1 rounded-2xl border border-border bg-muted p-1" role="tablist" aria-label={t('trafficSplit.buyTitle')}>
            {kinds.map((k) => (
              <button
                key={k}
                type="button"
                role="tab"
                aria-selected={k === kind}
                onClick={() => {
                  if (k !== kind) hapticSelection()
                  setKind(k)
                }}
                className={cn(
                  'flex h-10 items-center justify-center gap-2 rounded-xl text-sm font-semibold transition-colors',
                  k === kind ? 'bg-secondary text-foreground shadow-sm' : 'text-muted-foreground',
                )}
              >
                {k === 'bypass' ? <Route size={15} /> : <Gauge size={15} />}
                {kindName(k)}
              </button>
            ))}
          </div>
        )}

        {info && info.limit_bytes > 0 && (
          <p className="text-[13px] text-muted-foreground">
            {t('trafficSplit.nowUsed', {
              used: formatGbValue(info.used_bytes / GB),
              limit: formatGbValue(info.limit_bytes / GB),
              unit,
            })}
            {kind === 'bypass' && isBypassBlocked(split) ? ` · ${t('trafficSplit.nowBlocked', { label })}` : ''}
          </p>
        )}

        <div className="space-y-2.5">
          {packs.map((p, i) => {
            const saving = base > 0 ? Math.round((1 - pricePerGb(p) / base) * 100) : 0
            const on = p.id === selected
            return (
              <button
                key={p.id}
                type="button"
                onClick={() => {
                  if (!on) hapticSelection()
                  setSelected(p.id)
                }}
                aria-pressed={on}
                className={cn(
                  'relative flex w-full items-center gap-3.5 rounded-[18px] border px-4 py-3.5 text-left transition-colors',
                  on
                    ? kind === 'bypass'
                      ? 'border-[hsl(var(--split-bypass))] bg-[hsl(var(--split-bypass)/0.12)]'
                      : 'border-primary bg-primary/10'
                    : 'border-border bg-card/50',
                )}
              >
                {i === 1 && packs.length > 2 && (
                  <span className="absolute -top-2 right-3.5 rounded-full bg-emerald-600 px-2 py-0.5 text-[11px] font-bold text-white">
                    {t('trafficSplit.popular')}
                  </span>
                )}
                <span className="w-[88px] font-heading text-2xl font-extrabold leading-none">
                  {formatGbValue(p.bytes / GB)}
                  <small className="ml-1 text-sm font-bold text-muted-foreground">{unit}</small>
                </span>
                <span className="min-w-0 flex-1 text-[12.5px] text-muted-foreground">
                  {t('trafficSplit.perGb', { price: formatNumber(Math.round(pricePerGb(p) * 10) / 10) })}
                  {saving > 0 && <b className="text-emerald-500"> −{saving}%</b>}
                </span>
                <span className="text-lg font-extrabold">
                  {formatNumber(p.amount_rub)} {t('checkout.rub')}
                </span>
                <span
                  aria-hidden
                  className={cn(
                    'flex size-[22px] shrink-0 items-center justify-center rounded-full border-2',
                    on ? (kind === 'bypass' ? 'border-[hsl(var(--split-bypass))] bg-[hsl(var(--split-bypass))]' : 'border-primary bg-primary') : 'border-border',
                  )}
                >
                  {on && <span className="size-2 rounded-full bg-white" />}
                </span>
              </button>
            )
          })}
        </div>

      </div>
    </ResponsiveSheet>
  )
}

/** Кнопка «Докупить трафик» в шторке расхода (макет: фиолетовая у обхода, спокойная при лимитах у обоих видов). */
export function BuyTrafficButton({ data }: { data: HomeData }) {
  const { t } = useTranslation()
  const { open, canBuy } = useTrafficBuy()
  const split = data.split
  if (!canBuy || !split) return null
  const hasBypass = Boolean(split.bypass)
  const accent = hasBypass
  const label = bypassLabel(split.label)
  let text = t('trafficSplit.buy')
  if (accent && !split.packs.some((p) => p.kind === 'main')) {
    // Лимит только у обхода — кнопка называет его; если докупать можно и обычный трафик, остаётся «Докупить трафик».
    if (split.bypass && split.bypass.extra_bytes > 0) text = t('trafficSplit.buyMore')
    else text = isDefaultBypassLabel(split.label) ? t('trafficSplit.buyBypassDefault') : t('trafficSplit.buyBypass', { label })
  }
  return (
    <Button
      type="button"
      className={cn(
        'h-[52px] w-full shrink-0 rounded-full text-base font-bold',
        accent &&
          'border-0 bg-[hsl(var(--split-bypass-deep))] text-white shadow-[0_10px_30px_hsl(var(--split-bypass-deep)/0.3)] [background-image:linear-gradient(90deg,hsl(var(--split-bypass-deep)),hsl(var(--split-bypass-alt)))] hover:brightness-110',
      )}
      onClick={() => open(data.bypass?.blocked ? 'bypass' : undefined)}
    >
      <Plus size={18} />
      {text}
    </Button>
  )
}

/** Красная плашка в шторке: обход отключён, обычные серверы работают. */
function BlockedBanner({ label, date }: { label: string; date: string }) {
  const { t } = useTranslation()
  return (
    <div className="flex items-start gap-2.5 rounded-xl border border-destructive/45 bg-destructive/10 px-3 py-2.5 text-[13px] leading-snug">
      <AlertTriangle size={18} className="mt-px shrink-0 text-red-600 dark:text-red-400" aria-hidden />
      <span>
        <b>{t('trafficSplit.bannerTitle', { label })}</b> {t('trafficSplit.bannerBody', { date })}
      </span>
    </div>
  )
}

/** Блоки шторки при раздельном трафике: плашка, «Обычные серверы», «Обход». */
export function SplitBlocks({ data, lang, usedText }: { data: HomeData; lang: string; usedText: string }) {
  const { t } = useTranslation()
  const { split, bypass, trafficLimit, trafficUsed, trafficTone } = data
  if (!split || !bypass) return null
  const until = formatDayMonth(split.reset_at, lang)
  const month = utcPeriodMonthName(split.reset_at, lang)
  const mainColor = trafficTone === 'danger' ? 'bg-destructive' : trafficTone === 'warn' ? 'bg-amber-500' : 'bg-primary'
  return (
    <>
      {bypass.blocked && <BlockedBanner label={bypass.label} date={until} />}
      <div className="rounded-2xl border border-border/70 bg-card/60 px-4 py-3.5">
        <div className="mb-2.5 flex items-center gap-2 text-[15px] font-bold">
          <span className={cn('size-2.5 rounded-full', mainColor)} />
          {t('trafficSplit.mainTitle')}
          {trafficLimit === null && (
            <InfinityIcon size={26} strokeWidth={2.5} className="ml-auto text-primary" role="img" aria-label={t('trafficSplit.unlimitedTag')}>
              <title>{t('trafficSplit.unlimitedTag')}</title>
            </InfinityIcon>
          )}
        </div>
        {trafficLimit === null ? (
          <p className="font-heading text-[32px] font-extrabold leading-[1.1]">
            {usedText} <small className="text-lg font-bold text-muted-foreground">{t('trafficSplit.forMonth', { month })}</small>
          </p>
        ) : (
          <>
            <p className="font-heading text-[32px] font-extrabold leading-[1.1]">
              {formatGbValue(trafficUsed)} <small className="text-lg font-bold text-muted-foreground">{t('trafficSplit.ofLimit', { limit: formatGbValue(trafficLimit), unit: t('dashboard.gigabytes') })}</small>
            </p>
            <div className="mt-3 flex h-2.5 overflow-hidden rounded-full bg-muted">
              <span className={cn('h-full', mainColor)} style={{ width: `${Math.min(100, (trafficUsed / trafficLimit) * 100)}%` }} />
            </div>
            <p className="mt-2 text-[13px] text-muted-foreground">
              {t('trafficSplit.left', { left: formatGbValue(Math.max(0, trafficLimit - trafficUsed)), date: until })}
            </p>
          </>
        )}
      </div>
      <BypassBlock data={data} lang={lang} />
    </>
  )
}

/** Блок «обход» в шторке расхода: счётчик, полоска (тариф + докупленное), статус. */
export function BypassBlock({ data, lang }: { data: HomeData; lang: string }) {
  if (!data.split) return null
  return <BypassCard split={data.split} lang={lang} />
}

/** Карточка обхода: счётчик, полоска (тариф + докупленное), статус. Берёт данные прямо из /me/traffic. */
export function BypassCard({ split, lang }: { split: MyTrafficResponse; lang: string }) {
  const { t } = useTranslation()
  if (!split.bypass || split.bypass.limit_bytes <= 0) return null
  const info = split.bypass
  const pct = Math.min(100, (info.used_bytes / info.limit_bytes) * 100)
  const bypass = { label: bypassLabel(split.label), tone: trafficTone(pct) }
  const total = info.limit_bytes
  const baseW = total > 0 ? (Math.min(info.used_bytes, info.base_bytes) / total) * 100 : 0
  const extraW = total > 0 ? (Math.max(0, info.used_bytes - info.base_bytes) / total) * 100 : 0
  const tone = bypass.tone
  const color = tone === 'danger' ? 'bg-destructive' : tone === 'warn' ? 'bg-amber-500' : 'bg-[hsl(var(--split-bypass))]'
  const until = formatDayMonth(split.reset_at, lang)
  const blocked = isBypassBlocked(split)
  const left = Math.max(0, total - info.used_bytes) / GB
  return (
    <div className="rounded-2xl border border-border/70 bg-card/60 px-4 py-3.5">
      <div className="mb-2.5 flex items-center gap-2 text-[15px] font-bold">
        <span className={cn('size-2.5 rounded-full', color)} />
        {bypass.label}
        {blocked ? (
          <span className="ml-auto rounded-full bg-destructive/15 px-2.5 py-0.5 text-xs font-bold text-red-600 dark:text-red-400">{t('trafficSplit.exhausted')}</span>
        ) : tone === 'warn' ? (
          <span className="ml-auto rounded-full bg-amber-500/15 px-2.5 py-0.5 text-xs font-bold text-amber-500">{t('trafficSplit.almost')}</span>
        ) : null}
      </div>
      <p className="font-heading text-[32px] font-extrabold leading-[1.1]">
        {formatGbValue(info.used_bytes / GB)}{' '}
        <small className="text-lg font-bold text-muted-foreground">{t('trafficSplit.ofLimit', { limit: formatGbValue(total / GB), unit: t('dashboard.gigabytes') })}</small>
      </p>
      <div className="mt-3 flex h-2.5 overflow-hidden rounded-full bg-muted">
        <span className={cn('h-full', color)} style={{ width: `${Math.min(100, baseW)}%` }} />
        {extraW > 0 && <span className="h-full bg-[hsl(var(--split-bypass-soft))]" style={{ width: `${Math.min(100 - baseW, extraW)}%` }} />}
      </div>
      <p className="mt-2 text-[13px] text-muted-foreground">
        {blocked ? t('trafficSplit.blockedNote', { date: until, label: bypass.label }) : t('trafficSplit.left', { left: formatGbValue(left), date: until })}
      </p>
      {info.extra_bytes > 0 && (
        <p className="mt-2 flex items-center justify-between border-t border-border/60 pt-2 text-[13px] text-muted-foreground">
          <span>{t('trafficSplit.planPlusExtra', { base: formatGbValue(info.base_bytes / GB) })}</span>
          <b className="text-[hsl(var(--split-bypass-deep))] dark:text-[hsl(var(--split-bypass-soft))]">
            +{formatGbValue(info.extra_bytes / GB)} {t('dashboard.gigabytes')} · {t('trafficSplit.until', { date: until })}
          </b>
        </p>
      )}
    </div>
  )
}

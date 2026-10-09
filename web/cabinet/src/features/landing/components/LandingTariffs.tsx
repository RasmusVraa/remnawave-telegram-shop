import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { ArrowRight } from 'lucide-react'

import { api, type TariffItem, type TariffsResponse } from '@/lib/api'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { formatDecimals, formatInteger } from '@/lib/format'
import { cn } from '@/lib/utils'
import { LANDING_POPULAR_PLAN_PATTERNS } from '../landingContent'
import { logLandingMockHint, readLandingTariffsMock } from '../landingTariffsMock'
import type { LandingBrand } from '../useLandingBrand'
import { useCardSpotlight } from './LandingMotion'
import { SectionHeading } from './LandingPrimitives'

/**
 * Витрина тарифов.
 *
 * GET /cabinet/api/tariffs — публичный эндпоинт (см. registerAPIRoutes), поэтому
 * лендинг показывает реальные цены без авторизации. На карточке только название
 * и цена: описания и характеристики остаются в кабинете.
 *
 * Два режима подачи:
 *   section — самостоятельная секция с шапкой и якорем #tariffs;
 *   panel   — компактная ширма в колонке рядом с hero (раскладка «hero-side»).
 *
 * Блок не рендерится, если API недоступен или продавать нечего. Для локального
 * просмотра без бэкенда есть мок — см. landingTariffsMock.ts.
 */

export type LandingTariffsVariant = 'section' | 'panel' | 'stage'

interface TariffCardData {
  key: string
  /** Название тарифа или подпись периода («12 месяцев»). */
  name: string
  /** Крупная цена на карточке. */
  priceRub: number
  /** Мелкая строка под ценой: цена за месяц. */
  perMonthRub: number
  /** Скидка к базовой месячной цене, %. 0 — плашку не показываем. */
  savingsPct: number
  featured: boolean
}

function cardFromItem(item: TariffItem, monthPrice: number): TariffCardData {
  const perMonth = item.months > 0 ? item.price_rub / item.months : item.price_rub
  const pct =
    monthPrice > 0 && perMonth < monthPrice ? Math.round((1 - perMonth / monthPrice) * 100) : 0
  const haystack = `${item.slug} ${item.name}`.toLowerCase()
  return {
    key: item.slug,
    name: item.name,
    priceRub: item.price_rub,
    perMonthRub: perMonth,
    savingsPct: pct,
    featured: LANDING_POPULAR_PLAN_PATTERNS.some((p) => haystack.includes(p)),
  }
}

function PaymentLine() {
  const { t } = useTranslation()
  const { data } = useAuthBootstrap()
  const providers = data?.payment_providers
  const labels = [
    providers?.platega_sbp ? t('landing.tariffs.pay.sbp') : null,
    providers?.yookassa || providers?.platega_cards || providers?.platega_acquiring
      ? t('landing.tariffs.pay.card')
      : null,
    providers?.cryptopay || providers?.platega_crypto || providers?.heleket
      ? t('landing.tariffs.pay.crypto')
      : null,
    providers?.telegram ? t('landing.tariffs.pay.stars') : null,
  ].filter((label): label is string => Boolean(label))
  if (labels.length === 0) return null
  return (
    <p className="mt-5 text-center text-sm text-muted-foreground">
      {t('landing.tariffs.pay.label')} {labels.join(' · ')}
    </p>
  )
}

function formatRubInteger(n: number): string {
  return formatInteger(n)
}

function formatRub2(n: number): string {
  return formatDecimals(n, 2)
}

/**
 * classic-режим: тариф один, карточки — периоды подписки.
 * Выгода считается относительно цены месяца, «популярным» помечаем самый
 * длинный период (обычно годовой) — как на витрине периодов в кабинете.
 */
function buildPeriodCards(
  tariffs: TariffItem[],
  monthLabel: (n: number) => string,
): TariffCardData[] {
  const sorted = [...tariffs].sort((a, b) => a.months - b.months)
  const baseMonthly = sorted.find((p) => p.months === 1)?.price_rub ?? 0
  const longest = sorted[sorted.length - 1]?.months ?? 0

  return sorted.map((item) => {
    const perMonth = item.months > 0 ? item.price_rub / item.months : item.price_rub
    const pct =
      baseMonthly > 0 && perMonth < baseMonthly
        ? Math.round((1 - perMonth / baseMonthly) * 100)
        : 0
    return {
      key: `${item.slug}-${item.months}`,
      name: monthLabel(item.months),
      priceRub: item.price_rub,
      perMonthRub: perMonth,
      savingsPct: pct,
      featured: sorted.length > 1 && item.months === longest,
    }
  })
}

/**
 * tariffs-режим: карточки — тарифы, крупная цена за месяц.
 * Бэкенд флага «популярный» не отдаёт (в Go его нет), поэтому определяем
 * по названию/слагу — список шаблонов лежит в landingContent.ts.
 */
function buildPlanCards(tariffs: TariffItem[]): TariffCardData[] {
  const bySlug = new Map<string, TariffItem[]>()
  for (const item of tariffs) {
    const list = bySlug.get(item.slug) ?? []
    list.push(item)
    bySlug.set(item.slug, list)
  }

  return Array.from(bySlug.entries()).map(([slug, list]) => {
    list.sort((a, b) => a.months - b.months)
    const head = list[0]
    const monthly = head.monthly_base_rub || head.price_rub
    const haystack = `${slug} ${head.name}`.toLowerCase()
    return {
      key: slug,
      name: head.name,
      priceRub: monthly,
      perMonthRub: monthly,
      savingsPct: 0,
      featured: LANDING_POPULAR_PLAN_PATTERNS.some((p) => haystack.includes(p)),
    }
  })
}

/** Данные витрины: мок в dev-сборке, иначе публичная ручка. */
function useLandingTariffs() {
  const { t } = useTranslation()

  // Мок читаем один раз при монтировании: он не меняется без перезагрузки.
  const mock = useMemo(() => readLandingTariffsMock(), [])
  useEffect(() => {
    if (!mock) logLandingMockHint()
  }, [mock])

  const query = useQuery<TariffsResponse>({
    queryKey: ['landing-tariffs'],
    queryFn: () => api.tariffs(),
    staleTime: 5 * 60_000,
    retry: 1,
    enabled: !mock,
  })

  const data = mock ?? query.data

  const cards = useMemo<TariffCardData[]>(() => {
    if (!data?.tariffs?.length) return []
    return data.sales_mode === 'tariffs'
      ? buildPlanCards(data.tariffs)
      : buildPeriodCards(data.tariffs, (n) => t('tariffs.month', { count: n }))
  }, [data, t])

  return {
    cards,
    items: data?.tariffs ?? [],
    salesMode: data?.sales_mode ?? '',
    /** true — карточки описывают периоды, значит под ценой нужна цена за месяц. */
    isPeriods: data?.sales_mode !== 'tariffs',
    loading: !mock && query.isLoading,
  }
}

function TariffCard({
  card,
  href,
  isPeriods,
  stage,
  onMouseMove,
}: {
  card: TariffCardData
  href: string
  isPeriods: boolean
  stage?: boolean
  onMouseMove: (e: React.MouseEvent<HTMLElement>) => void
}) {
  const { t } = useTranslation()

  return (
    <a
      href={href}
      className={cn(
        'landing-tariff-card landing-card flex h-full flex-col p-4 sm:p-5',
        stage && 'landing-tariff-card--stage',
        card.featured && 'landing-card--featured',
      )}
      onMouseMove={onMouseMove}
    >
      <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span className="text-sm font-semibold leading-tight sm:text-base">{card.name}</span>
        {card.featured && <span className="landing-tariff-pill">{t('tariffs.popular')}</span>}
      </span>

      {card.savingsPct > 0 ? (
        <span className="mt-2 block text-xs font-semibold text-[hsl(var(--lp-cyan))]">
          {t('tariffs.saving', { pct: card.savingsPct })}
        </span>
      ) : (
        <span className="mt-2 block text-xs opacity-0" aria-hidden>
          &nbsp;
        </span>
      )}

      <span
        className={cn(
          'landing-price mt-auto block font-heading font-extrabold leading-none',
          stage ? 'pt-5 text-[1.7rem] sm:text-4xl' : 'pt-5 text-4xl sm:text-5xl',
        )}
      >
        {formatRubInteger(card.priceRub)} ₽
      </span>

      <span className="mt-1.5 block text-xs leading-4 text-muted-foreground sm:text-sm">
        {isPeriods
          ? `${formatRub2(card.perMonthRub)} ₽ ${t('landing.tariffs.perMonthFull')}`
          : t('landing.tariffs.perMonthFull')}
      </span>
    </a>
  )
}

export function LandingTariffs({
  brand,
  variant = 'section',
}: {
  brand: LandingBrand
  variant?: LandingTariffsVariant
}) {
  const { t } = useTranslation()
  const onMouseMove = useCardSpotlight()
  const { cards, items, salesMode, isPeriods, loading } = useLandingTariffs()
  const months = useMemo(() => {
    if (salesMode !== 'tariffs') return []
    return [...new Set(items.map((item) => item.months))].filter((n) => n > 0).sort((a, b) => a - b)
  }, [items, salesMode])
  const [month, setMonth] = useState<number | null>(null)
  const selectedMonth = month && months.includes(month) ? month : (months[0] ?? null)

  const periodCards = useMemo(() => {
    if (salesMode !== 'tariffs' || selectedMonth == null) return cards
    const baseBySlug = new Map<string, number>()
    for (const item of items) {
      if (item.months === 1) baseBySlug.set(item.slug, item.price_rub)
    }
    return items
      .filter((item) => item.months === selectedMonth)
      .map((item) => cardFromItem(item, baseBySlug.get(item.slug) ?? 0))
  }, [cards, items, salesMode, selectedMonth])

  // Пока грузится — держим место, чтобы hero и якорь #tariffs не «прыгали».
  if (loading) {
    if (variant === 'panel' || variant === 'stage') {
      return <div className="landing-stage min-h-[18rem]" aria-busy />
    }
    return <section id="tariffs" className="min-h-[40vh]" aria-busy />
  }
  if (cards.length === 0) return null

  // На лендинге тариф не выбирают — ведём в кабинет, дальше обычный флоу оплаты.
  const buyHref = brand.tariffsHref
  const shown = salesMode === 'tariffs' ? periodCards : cards

  if (variant === 'stage') {
    return (
      <div id="tariffs" className="landing-stage">
        <div className="flex flex-wrap items-end justify-between gap-3 px-1">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
              {t('landing.tariffs.eyebrow')}
            </p>
            <h2 className="mt-1 font-heading text-2xl font-bold tracking-tight">{t('landing.tariffs.title')}</h2>
          </div>
        </div>

        {months.length > 1 && (
          <div className="mt-4 flex flex-wrap gap-2">
            {months.map((value) => (
              <button
                key={value}
                type="button"
                onClick={() => setMonth(value)}
                className={cn(
                  'rounded-full px-3.5 py-1.5 text-sm font-semibold transition-colors',
                  value === selectedMonth ? 'landing-cta landing-cta--primary' : 'landing-cta landing-cta--ghost',
                )}
              >
                {value === 12 ? t('landing.tariffs.periodYear') : t('landing.tariffs.periodMonths', { count: value })}
              </button>
            ))}
          </div>
        )}

        <div className="mt-4 grid grid-cols-2 gap-3">
          {shown.map((card) => (
            <TariffCard
              key={card.key}
              card={card}
              href={buyHref}
              isPeriods={isPeriods || salesMode === 'tariffs'}
              stage
              onMouseMove={onMouseMove}
            />
          ))}
        </div>
        <PaymentLine />
      </div>
    )
  }

  if (variant === 'panel') {
    return (
      <div id="tariffs" className="w-full">
        <div className="px-1 pb-3">
          <span className="text-sm font-semibold text-muted-foreground">
            {t('landing.tariffs.eyebrow')}
          </span>
        </div>

        <div className="grid grid-cols-2 gap-3">
          {cards.map((card) => (
            <TariffCard
              key={card.key}
              card={card}
              href={buyHref}
              isPeriods={isPeriods}
              onMouseMove={onMouseMove}
            />
          ))}
        </div>

        <a
          href={buyHref}
          className="landing-cta landing-cta--primary group mt-4 inline-flex h-12 w-full items-center justify-center gap-2 rounded-full px-6 text-sm font-semibold"
        >
          {t('landing.tariffs.cta')}
          <ArrowRight className="size-4 transition-transform duration-300 group-hover:translate-x-0.5" />
        </a>
      </div>
    )
  }

  const columns =
    shown.length <= 2
      ? 'grid-cols-1 sm:grid-cols-2 sm:max-w-2xl sm:mx-auto'
      : shown.length === 3
        ? 'grid-cols-2 lg:grid-cols-3 lg:max-w-4xl lg:mx-auto'
        : 'grid-cols-2 lg:grid-cols-4'

  return (
    <section id="tariffs" className="px-4 py-14 sm:px-6 sm:py-20">
      <div className="mx-auto max-w-6xl">
        <SectionHeading
          eyebrow={t('landing.tariffs.eyebrow')}
          title={t('landing.tariffs.title')}
          description={t('landing.tariffs.subtitle')}
        />

        {months.length > 1 && (
          <div className="mt-8 flex flex-wrap justify-center gap-2">
            {months.map((value) => (
              <button
                key={value}
                type="button"
                onClick={() => setMonth(value)}
                className={cn(
                  'rounded-full px-4 py-2 text-sm font-semibold transition-colors',
                  value === selectedMonth
                    ? 'landing-cta landing-cta--solid'
                    : 'landing-cta landing-cta--ghost',
                )}
              >
                {value === 12 ? t('landing.tariffs.periodYear') : t('landing.tariffs.periodMonths', { count: value })}
              </button>
            ))}
          </div>
        )}

        <PaymentLine />

        <div className={cn('mt-8 grid gap-3 sm:gap-4', columns)}>
          {shown.map((card) => (
            <TariffCard
              key={card.key}
              card={card}
              href={buyHref}
              isPeriods={isPeriods}
              onMouseMove={onMouseMove}
            />
          ))}
        </div>

        <div className="mt-8 flex justify-center sm:mt-10">
          <a
            href={buyHref}
            className="landing-cta landing-cta--solid group inline-flex h-12 items-center justify-center gap-2 rounded-full px-7 text-sm font-semibold"
          >
            {t('landing.tariffs.cta')}
            <ArrowRight className="size-4 transition-transform duration-300 group-hover:translate-x-0.5" />
          </a>
        </div>
      </div>
    </section>
  )
}

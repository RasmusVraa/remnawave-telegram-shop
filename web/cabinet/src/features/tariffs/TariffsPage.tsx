import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Cloud, Smartphone, Star, Zap, type LucideIcon } from 'lucide-react'
import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { TariffDescription } from '@/components/TariffDescription'
import { PageTitleWithBack } from '@/components/PageTitleWithBack'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { api, type SubscriptionResponse, type TariffItem } from '@/lib/api'
import { cn } from '@/lib/utils'
import { hapticSelection } from '@/lib/haptic'
import { bypassLabelInline } from '@/lib/bypassLabel'
import { useAuthStore } from '@/store/auth'
import { formatDecimals, formatNumber } from '@/lib/format'
import {
  formatRubInteger,
  formatShowcasePriceRub,
  periodBaselineRub,
  periodSavingsPercent,
  showAnnualPriceFootnote,
  showcaseMonthlyRub,
  type TariffPriceDisplayMode,
  type TariffSavingsBadgeMode,
} from '@/features/tariffs/tariffShowcasePrice'

export default function TariffsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const planSlug = (searchParams.get('plan') ?? '').trim()
  const { user } = useAuthStore()
  const verified = user?.email_verified ?? false

  const { data, isLoading, error } = useQuery({
    queryKey: ['tariffs'],
    queryFn: () => api.tariffs(),
    staleTime: 5 * 60_000,
  })

  const { data: sub } = useQuery({
    queryKey: ['subscription'],
    queryFn: () => api.subscription(),
    staleTime: 60_000,
    enabled: verified,
  })

  function setPlan(slug: string) {
    setSearchParams({ plan: slug })
  }

  function clearPlan() {
    setSearchParams({})
  }

  function handleCheckout(tariff: TariffItem) {
    navigate(`/checkout?tariff=${encodeURIComponent(tariff.slug)}&months=${tariff.months}`)
  }

  // Заголовок со стрелкой «назад» должен стоять по левой границе карточек:
  // сетка центрируется своим max-width, поэтому ту же ширину получает и шапка.
  const columnClass = useMemo(() => {
    if (data?.sales_mode !== 'tariffs' || !data.tariffs) return undefined
    return tariffsColumnClass(buildCardPeriodsBySlug(data.tariffs).length)
  }, [data])

  const classicSorted =
    data?.sales_mode !== 'tariffs' && data?.tariffs
      ? [...data.tariffs].sort((a, b) => a.months - b.months)
      : null

  return (
    <AppLayout>
      <PageReveal className="space-y-6">
        <RevealItem className={cn(planSlug ? 'mx-auto w-full max-w-lg' : columnClass)}>
          <PageTitleWithBack
            title={t('tariffs.title')}
            subtitle={t('tariffs.subtitle')}
          />
        </RevealItem>

        {isLoading && (
          <RevealItem>
            <TariffsSkeleton />
          </RevealItem>
        )}

        {error && (
          <RevealItem>
            <p className="text-sm text-destructive">{t('errors.unknown')}</p>
          </RevealItem>
        )}

        {data && data.sales_mode === 'tariffs' && !planSlug && (
          <RevealItem className={columnClass}>
            <TariffsGrid
              tariffs={data.tariffs}
              priceDisplay={data.price_display ?? 'monthly'}
              onChoosePlan={setPlan}
              sub={sub}
            />
          </RevealItem>
        )}

        {data && data.sales_mode === 'tariffs' && planSlug && (
          <RevealItem>
            <TariffPeriodStep
              slug={planSlug}
              tariffs={data.tariffs}
              savingsBadge={data.savings_badge ?? 'none'}
              onBack={clearPlan}
              onSelect={handleCheckout}
            />
          </RevealItem>
        )}

        {data && data.sales_mode !== 'tariffs' && classicSorted && !planSlug && (
          <RevealItem>
            <ClassicIntro sorted={classicSorted} onContinue={() => setPlan('classic')} />
          </RevealItem>
        )}

        {data && data.sales_mode !== 'tariffs' && classicSorted && planSlug === 'classic' && (
          <RevealItem>
            <ClassicPeriodStep
              sorted={classicSorted}
              sub={sub}
              onBack={clearPlan}
              onSelect={handleCheckout}
            />
          </RevealItem>
        )}
      </PageReveal>
    </AppLayout>
  )
}

// ── Tariffs mode: шаг 1 — только карточки планов ────────────────────────────

/**
 * Ширина контентной колонки «Тарифов» на ПК: должна совпадать с max-width сетки
 * карточек, иначе шапка уезжает левее карточек внутри max-w-5xl страницы.
 * До 500px работает карусель во всю ширину — там ограничение не нужно.
 */
function tariffsColumnClass(cardsCount: number): string | undefined {
  if (cardsCount === 2) return 'mx-auto w-full min-[501px]:max-w-2xl'
  if (cardsCount >= 3) return 'mx-auto w-full min-[501px]:max-w-4xl'
  return undefined
}

function buildCardPeriodsBySlug(tariffs: TariffItem[]): TariffItem[][] {
  const bySlug = new Map<string, TariffItem[]>()
  for (const item of tariffs) {
    const list = bySlug.get(item.slug) ?? []
    list.push(item)
    bySlug.set(item.slug, list)
  }
  return Array.from(bySlug.values()).map((list) => {
    list.sort((a, b) => a.months - b.months)
    return list
  })
}

/**
 * Порядок мобильной карусели (≤500px): текущий тариф первым, популярный сразу за ним;
 * без подписки первым идёт популярный. Сетка на ПК использует исходный порядок API.
 */
function orderCarouselPeriods(periods: TariffItem[][], sub?: SubscriptionResponse): TariffItem[][] {
  if (periods.length <= 1) return periods
  const currentSlug = isSubscriptionActive(sub?.expire_at) ? sub?.tariff?.slug : undefined
  const current = currentSlug ? periods.find((p) => p[0]?.slug === currentSlug) : undefined
  const popular = periods.find((p) => p[0]?.is_popular && p !== current)
  const head = [current, popular].filter((p): p is TariffItem[] => p != null)
  return [...head, ...periods.filter((p) => !head.includes(p))]
}

function TariffsGrid({
  tariffs,
  priceDisplay,
  onChoosePlan,
  sub,
}: {
  tariffs: TariffItem[]
  priceDisplay: TariffPriceDisplayMode
  onChoosePlan: (slug: string) => void
  sub?: SubscriptionResponse
}) {
  const { t } = useTranslation()
  const cardPeriods = useMemo(() => buildCardPeriodsBySlug(tariffs), [tariffs])
  const showFootnote = showAnnualPriceFootnote(priceDisplay, cardPeriods)

  const carouselPeriods = useMemo(
    () => orderCarouselPeriods(cardPeriods, sub),
    [cardPeriods, sub],
  )

  const singleGridClass = cn('grid max-w-xs gap-4 mx-auto')
  const desktopGridClass = cn(
    'gap-4',
    cardPeriods.length === 2 ? 'grid-cols-1 sm:grid-cols-2' : 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-2',
  )

  if (cardPeriods.length === 1) {
    return (
      <div className="space-y-3">
        <div className={singleGridClass}>
          {cardPeriods.map((periods) => (
            <TariffPlanCard
              key={periods[0].slug}
              periods={periods}
              priceDisplay={priceDisplay}
              onChoosePlan={onChoosePlan}
              sub={sub}
            />
          ))}
        </div>
        {showFootnote && (
          <p className="text-center text-xs text-muted-foreground">{t('tariffs.annualPriceFootnote')}</p>
        )}
      </div>
    )
  }

  return (
    <>
      <div className="max-[500px]:block min-[501px]:hidden">
        <TariffsMobileCarousel
          cardPeriods={carouselPeriods}
          priceDisplay={priceDisplay}
          onChoosePlan={onChoosePlan}
          sub={sub}
        />
        {showFootnote && (
          <p className="mt-3 text-center text-xs text-muted-foreground">{t('tariffs.annualPriceFootnote')}</p>
        )}
      </div>
      <div className="hidden w-full min-[501px]:block">
        <div className={cn('grid', desktopGridClass)}>
          {cardPeriods.map((periods) => (
            <TariffPlanCard
              key={periods[0].slug}
              periods={periods}
              priceDisplay={priceDisplay}
              onChoosePlan={onChoosePlan}
              sub={sub}
            />
          ))}
          {showFootnote && (
            <p className="col-span-full text-center text-xs text-muted-foreground pt-1">
              {t('tariffs.annualPriceFootnote')}
            </p>
          )}
        </div>
      </div>
    </>
  )
}

/** Карусель ≤500px: snap-start — основная карточка почти на всю ширину, сосед у правого края с узким «peek» текста. */
function TariffsMobileCarousel({
  cardPeriods,
  priceDisplay,
  onChoosePlan,
  sub,
}: {
  cardPeriods: TariffItem[][]
  priceDisplay: TariffPriceDisplayMode
  onChoosePlan: (slug: string) => void
  sub?: SubscriptionResponse
}) {
  const { t } = useTranslation()
  const scrollRef = useRef<HTMLDivElement>(null)
  const [active, setActive] = useState(0)

  const updateActiveFromScroll = useCallback(() => {
    const root = scrollRef.current
    if (!root) return
    const slides = root.querySelectorAll<HTMLElement>('[data-tariff-slide]')
    if (!slides.length) return
    const rootRect = root.getBoundingClientRect()
    let best = 0
    let bestOverlap = -1
    slides.forEach((el, i) => {
      const r = el.getBoundingClientRect()
      const overlap = Math.min(r.right, rootRect.right) - Math.max(r.left, rootRect.left)
      if (overlap > bestOverlap) {
        bestOverlap = overlap
        best = i
      }
    })
    setActive((prev) => (prev === best ? prev : best))
  }, [])

  const orderKey = cardPeriods.map((p) => p[0].slug).join(',')

  useLayoutEffect(() => {
    const root = scrollRef.current
    if (root) root.scrollLeft = 0
    setActive(0)
    requestAnimationFrame(() => updateActiveFromScroll())
  }, [orderKey, updateActiveFromScroll])

  useEffect(() => {
    const root = scrollRef.current
    if (!root) return
    let raf = 0
    const onScroll = () => {
      cancelAnimationFrame(raf)
      raf = requestAnimationFrame(updateActiveFromScroll)
    }
    root.addEventListener('scroll', onScroll, { passive: true })
    const ro = new ResizeObserver(() => updateActiveFromScroll())
    ro.observe(root)
    return () => {
      cancelAnimationFrame(raf)
      root.removeEventListener('scroll', onScroll)
      ro.disconnect()
    }
  }, [updateActiveFromScroll])

  const scrollToIndex = (i: number) => {
    const root = scrollRef.current
    if (!root) return
    const slide = root.querySelectorAll<HTMLElement>('[data-tariff-slide]')[i]
    if (!slide) return
    slide.scrollIntoView({ behavior: 'smooth', inline: 'start', block: 'nearest' })
  }

  const n = cardPeriods.length
  const hasPopular = cardPeriods.some((p) => p[0]?.is_popular)

  return (
    <div className="w-full">
      <div
        ref={scrollRef}
        className={cn(
          'flex min-w-0 w-full items-stretch gap-2 overflow-x-auto overscroll-x-contain pb-1 touch-pan-x',
          // overflow-x режет и по вертикали: без отступов нашивка «Популярный» над рамкой
          // и свечение под карточкой обрезались бы.
          hasPopular && 'pb-4 pt-3',
          'scroll-pl-3 scroll-pr-3 snap-x snap-mandatory [-ms-overflow-style:none] [scrollbar-width:none]',
          '[&::-webkit-scrollbar]:hidden',
        )}
        style={{ WebkitOverflowScrolling: 'touch' }}
      >
        {cardPeriods.map((periods) => (
          <div
            key={periods[0].slug}
            data-tariff-slide
            className="flex min-h-0 w-[min(22rem,calc(100%-1.9rem))] shrink-0 snap-start flex-col self-stretch"
          >
            <TariffPlanCard
              layout="carousel"
              periods={periods}
              priceDisplay={priceDisplay}
              onChoosePlan={onChoosePlan}
              sub={sub}
            />
          </div>
        ))}
      </div>
      <nav
        className={cn('flex justify-center gap-2', hasPopular ? 'mt-0' : 'mt-4')}
        aria-label={t('tariffs.carouselListLabel')}
      >
        {cardPeriods.map((periods, i) => (
          <button
            key={periods[0].slug}
            type="button"
            aria-label={t('tariffs.carouselGoTo', { n: i + 1, total: n })}
            aria-current={i === active ? 'true' : undefined}
            className={cn(
              'h-2 rounded-full transition-[width,background-color] duration-200',
              i === active
                ? 'w-6 bg-primary'
                : periods[0].is_popular
                  // Популярный, пока на него не пролистали, — точка цвета темы, только тусклее.
                  ? 'w-2 bg-primary/50 hover:bg-primary/70'
                  : 'w-2 bg-muted-foreground/35 hover:bg-muted-foreground/55',
            )}
            onClick={() => {
              if (i !== active) hapticSelection()
              scrollToIndex(i)
            }}
          />
        ))}
      </nav>
      <p className="mt-2 text-center text-sm text-muted-foreground">
        {t('tariffs.carouselTotal', { count: n })}
      </p>
    </div>
  )
}

function TariffPlanCard({
  periods,
  priceDisplay,
  onChoosePlan,
  sub,
  layout = 'grid',
}: {
  periods: TariffItem[]
  priceDisplay: TariffPriceDisplayMode
  onChoosePlan: (slug: string) => void
  sub?: SubscriptionResponse
  layout?: 'grid' | 'carousel'
}) {
  const { t } = useTranslation()
  const head = periods[0]
  const showcaseMonthly = showcaseMonthlyRub(periods, priceDisplay)
  const active = isSubscriptionActive(sub?.expire_at)
  const isCurrent = Boolean(active && sub?.tariff?.slug === head.slug)
  const ctaLabel = !active ? t('tariffs.select') : isCurrent ? t('tariffs.ctaRenew') : t('tariffs.ctaChange')
  const isCarousel = layout === 'carousel'
  const isPopular = Boolean(head.is_popular)

  return (
    <Card
      role="button"
      tabIndex={0}
      data-tariff-card
      data-popular={isPopular || undefined}
      onClick={() => onChoosePlan(head.slug)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onChoosePlan(head.slug)
        }
      }}
      className={cn(
        'relative flex flex-col transition-[border-color,box-shadow,transform,filter] duration-200 cursor-pointer active:scale-[0.98]',
        isCarousel && 'min-h-0 w-full flex-1',
        /*
         * Цветом выделен только популярный тариф (рамка и свечение — index.css,
         * .tariff-card-popular). Текущий тариф отличается одной серой меткой:
         * в зелёной теме зелёными были бы и текущий, и популярный.
         */
        isPopular
          ? cn('tariff-card-popular', isCarousel ? 'tariff-card-popular--tight' : 'hover:-translate-y-0.5')
          : cn(tariffOtherCardHoverClassName, 'hover:brightness-[1.02]'),
      )}
    >
      {isPopular && (
        <span
          className={cn(
            'tariff-popular-badge pointer-events-none absolute -top-3 z-10 inline-flex items-center gap-1 rounded-full py-[5px] pl-2 pr-2.5 text-xs font-bold leading-none',
            isCarousel ? 'right-4' : 'right-5',
          )}
        >
          <Star size={11} fill="currentColor" strokeWidth={0} aria-hidden />
          {t('tariffs.popular')}
        </span>
      )}

      <CardHeader className="px-4 pb-2 pt-5">
        <CardTitle className="text-lg">{head.name}</CardTitle>
        <div className="flex items-baseline gap-1 mt-2">
          <span className="text-[2.5rem] font-bold leading-none tabular-nums">
            {formatShowcasePriceRub(showcaseMonthly, priceDisplay)}
          </span>
          <span className="text-sm text-muted-foreground">₽{t('tariffs.perMonth')}</span>
        </div>
      </CardHeader>

      {isCurrent && (
        /* У популярного сверху нашивка: метка «Текущий» опускается, чтобы не прилипнуть к ней. */
        <div className={cn('absolute right-4', isPopular ? 'top-[22px]' : 'top-4')}>
          <Badge variant="secondary" className={cn('w-fit text-xs font-normal', tariffCurrentPlanBadgeClassName)}>
            {t('tariffs.currentBadge')}
          </Badge>
        </div>
      )}

      <CardContent
        className={cn(
          'flex flex-col gap-4 p-4 pt-0',
          isCarousel ? 'min-h-0 flex-1' : 'mt-auto flex-1',
        )}
      >
        {isCarousel ? (
          <>
            <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto overscroll-y-contain [-webkit-overflow-scrolling:touch]">
              {head.description ? (
                <TariffDescription
                  text={head.description}
                  className="text-[0.875rem] text-muted-foreground leading-relaxed"
                />
              ) : null}
              <ul className="space-y-1.5 text-sm shrink-0">
                <FeatureLine icon={Cloud}>
                  {trafficLineText(head, t)}
                </FeatureLine>
                <FeatureLine icon={Smartphone}>
                  {head.device_limit > 0
                    ? t('tariffs.devices', { n: head.device_limit })
                    : t('tariffs.devicesUnlimited')}
                </FeatureLine>
              </ul>
            </div>
            <Button
              className={cn(
                'mt-auto w-full shrink-0 transition-[background-color,box-shadow,transform,filter] duration-200',
                isCurrent ? tariffRenewButtonClassName : tariffChangeCtaButtonClassName,
              )}
              variant="default"
              type="button"
            >
              {ctaLabel}
            </Button>
          </>
        ) : (
          <>
            {head.description ? (
              <TariffDescription text={head.description} className="text-[0.875rem] text-muted-foreground leading-relaxed" />
            ) : null}

            {/* mt-auto прижимает лимиты к кнопке: в соседних карточках ряда они на одной высоте. */}
            <ul className="mt-auto space-y-1.5 text-sm">
              <FeatureLine icon={Cloud}>
                {trafficLineText(head, t)}
              </FeatureLine>
              <FeatureLine icon={Smartphone}>
                {head.device_limit > 0
                  ? t('tariffs.devices', { n: head.device_limit })
                  : t('tariffs.devicesUnlimited')}
              </FeatureLine>
            </ul>

            <Button
              className={cn(
                'w-full transition-[background-color,box-shadow,transform,filter] duration-200',
                isCurrent ? tariffRenewButtonClassName : tariffChangeCtaButtonClassName,
              )}
              variant="default"
              type="button"
            >
              {ctaLabel}
            </Button>
          </>
        )}
      </CardContent>
    </Card>
  )
}

// ── Шаг 2 — выбор срока (режим tariffs) ────────────────────────────────────

function TariffPeriodStep({
  slug,
  tariffs,
  savingsBadge,
  onBack,
  onSelect,
}: {
  slug: string
  tariffs: TariffItem[]
  savingsBadge: TariffSavingsBadgeMode
  onBack: () => void
  onSelect: (t: TariffItem) => void
}) {
  const { t } = useTranslation()
  const periods = [...tariffs].filter((x) => x.slug === slug).sort((a, b) => a.months - b.months)
  const head = periods[0]

  useEffect(() => {
    if (periods.length === 0) onBack()
  }, [periods.length, onBack])

  if (!head) return null

  const formatRub2 = (n: number) => formatDecimals(n, 2)

  // Период изначально НЕ выбран — подсветка появляется только после клика.
  const [selectedMonths, setSelectedMonths] = useState<number | null>(null)

  const detailText =
    (head.description_detail?.trim() || head.description?.trim()) ?? ''

  return (
    <div className="space-y-4 max-w-lg mx-auto w-full">
      <div>
        <h2 className="text-xl font-semibold">{head.name}</h2>
        {detailText ? (
          <TariffDescription
            text={detailText}
            className="text-sm text-muted-foreground mt-1 leading-relaxed"
          />
        ) : null}
        <p className="text-sm text-muted-foreground mt-3">{t('tariffs.choosePeriodHint')}</p>
      </div>
      <div
        className={cn(
          'grid gap-2',
          periods.length <= 2 ? 'grid-cols-1 sm:grid-cols-2' : 'grid-cols-2',
        )}
      >
        {periods.map((p) => (
          <Button
            key={p.months}
            type="button"
            variant="outline"
            className={cn(
              /*
               * Ховер и нажатие приходят из варианта outline — свой
               * tariffOtherCardHoverClassName тут только спорил с ним за бордер.
               */
              'relative h-auto min-h-[88px] flex-col items-start justify-start gap-0 px-4 py-3 rounded-[var(--radius)] backdrop-blur-[2px] border duration-200',
              tariffCardShadowClassName,
              p.months === selectedMonths
                ? 'border-primary bg-primary/10 shadow-[0_10px_26px_-12px_hsl(var(--cabinet-accent)/0.5)] dark:bg-primary/15'
                : 'border-border bg-card dark:bg-[hsl(var(--card))]',
            )}
            // Внутри колонка, а правый верхний угол занят плашкой скидки.
            overlaySpinnerClassName="absolute bottom-3 right-3"
            onClick={() => {
              if (p.months !== selectedMonths) hapticSelection()
              setSelectedMonths(p.months)
              onSelect(p)
            }}
          >
            {(() => {
              const perMonthRub = p.months > 0 ? p.price_rub / p.months : 0
              const savingPct = savingsBadge === 'none' ? null : periodSavingsPercent(periods, p)
              const baselineRub =
                savingsBadge === 'old_price' && savingPct != null
                  ? periodBaselineRub(periods, p.months)
                  : 0
              return (
                <>
                  {savingPct != null && (
                    /*
                     * Угол — единственное место плашки в обоих режимах: в строке
                     * цены она читалась как часть суммы, а рядом с зачёркнутой
                     * базой строка ещё и распухала.
                     */
                    <span className={cn(periodSavingBadgeClassName, 'absolute right-2.5 top-2.5')}>
                      −{savingPct}%
                    </span>
                  )}
                  <span
                    className="text-lg leading-7 font-medium tabular-nums text-foreground dark:text-[rgb(241,245,249)]"
                  >
                    {savingsBadge === 'none' ? (
                      pluralizeMonths(p.months)
                    ) : (
                      /*
                       * С плашкой в углу «12 месяцев» на мобилке упирается в неё
                       * (у кнопки whitespace-nowrap, так что текст полез бы под
                       * плашку). Короткая форма — как в редакторе тарифа в админке.
                       */
                      <>
                        <span className="min-[501px]:hidden">{pluralizeMonthsShort(p.months)}</span>
                        <span className="hidden min-[501px]:inline">{pluralizeMonths(p.months)}</span>
                      </>
                    )}
                  </span>
                  <span className="flex flex-wrap items-baseline gap-x-1.5">
                    <span
                      className="text-[0.95rem] leading-5 font-semibold tabular-nums text-primary"
                    >
                      {formatRubInteger(p.price_rub)} ₽
                    </span>
                    {baselineRub > 0 && (
                      <span className="text-[0.7rem] min-[501px]:text-[0.78rem] leading-5 font-normal tabular-nums text-muted-foreground line-through">
                        {formatRubInteger(baselineRub)} ₽
                      </span>
                    )}
                  </span>
                  <span
                    className="mt-1 text-[0.7rem] leading-4 font-normal tabular-nums text-muted-foreground dark:text-[rgb(101,114,134)] tracking-[-1px]"
                  >
                    {formatRub2(perMonthRub)} ₽{t('tariffs.perMonth')}
                  </span>
                </>
              )
            })()}
          </Button>
        ))}
      </div>
    </div>
  )
}

// ── Classic: шаг 1 — вводная, шаг 2 — сроки ───────────────────────────────

function ClassicIntro({
  sorted,
  onContinue,
}: {
  sorted: TariffItem[]
  onContinue: () => void
}) {
  const { t } = useTranslation()
  const head = sorted[0]
  if (!head) return null

  return (
    <div className="space-y-4 max-w-xl">
      <div className="flex flex-wrap gap-4 text-sm text-muted-foreground">
        <span className="flex items-center gap-1.5">
          <Cloud size={14} className="text-primary shrink-0" />
          {trafficLineText(head, t)}
        </span>
        <span className="flex items-center gap-1.5">
          <Smartphone size={14} className="text-primary shrink-0" />
          {head.device_limit > 0
            ? t('tariffs.devices', { n: head.device_limit })
            : t('tariffs.devicesUnlimited')}
        </span>
      </div>
      <Card>
        <CardContent className="px-5 py-6 space-y-4">
          <div className="flex items-baseline gap-1">
            <span className="text-3xl font-bold">{formatNumber(head.monthly_base_rub)}</span>
            <span className="text-sm text-muted-foreground">₽{t('tariffs.perMonth')}</span>
          </div>
          {head.description ? (
            <TariffDescription text={head.description} className="text-sm text-muted-foreground leading-relaxed" />
          ) : null}
          <Button type="button" className="w-full" onClick={onContinue}>
            {t('tariffs.classicChoosePeriod')}
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}

function ClassicPeriodStep({
  sorted,
  sub,
  onBack,
  onSelect,
}: {
  sorted: TariffItem[]
  sub?: SubscriptionResponse
  onBack: () => void
  onSelect: (t: TariffItem) => void
}) {
  const { t } = useTranslation()
  const baseMonthlyRub = sorted[0]?.monthly_base_rub ?? sorted[0]?.price_rub ?? 0

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">{t('tariffs.choosePeriodHint')}</p>
      <div className={cn(
        'grid gap-3',
        sorted.length <= 2
          ? 'grid-cols-1 sm:grid-cols-2 max-w-xl'
          : 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-4',
      )}>
        {sorted.map((tariff) => (
          <PeriodCard
            key={tariff.slug + tariff.months}
            tariff={tariff}
            baseMonthlyRub={baseMonthlyRub}
            onSelect={onSelect}
            sub={sub}
          />
        ))}
      </div>
    </div>
  )
}

function PeriodCard({
  tariff,
  baseMonthlyRub,
  onSelect,
  sub,
}: {
  tariff: TariffItem
  baseMonthlyRub: number
  onSelect: (t: TariffItem) => void
  sub?: SubscriptionResponse
}) {
  const { t } = useTranslation()
  const active = isSubscriptionActive(sub?.expire_at)
  const monthsMatch =
    sub?.subscription_period_months == null || sub.subscription_period_months === tariff.months
  const isCurrent = Boolean(active && sub?.tariff?.slug === tariff.slug && monthsMatch)
  const ctaLabel = !active ? t('tariffs.select') : isCurrent ? t('tariffs.ctaRenew') : t('tariffs.ctaChange')

  const savingPct =
    tariff.monthly_base_rub < baseMonthlyRub
      ? Math.round((1 - tariff.monthly_base_rub / baseMonthlyRub) * 100)
      : 0

  const monthLabel = pluralizeMonths(tariff.months)

  return (
    <Card
      data-tariff-card
      className={cn(
        'relative flex flex-col transition-[border-color,box-shadow] duration-200',
        isCurrent
          ? tariffCurrentCardClassName
          : cn(tariffOtherCardHoverClassName, 'hover:brightness-[1.01]'),
        tariff.is_popular && !isCurrent && 'border-primary/50',
      )}
    >
      {tariff.is_popular && (
        <div className="absolute -top-3 left-1/2 -translate-x-1/2">
          <Badge className="gap-1">
            <Zap size={10} />
            {t('tariffs.popular')}
          </Badge>
        </div>
      )}

      <CardHeader className="pb-2 pt-5">
        <CardTitle className="text-base">{monthLabel}</CardTitle>
        {isCurrent && (
          <Badge
            variant="secondary"
            className={cn('w-fit text-xs mt-1 font-normal', tariffCurrentBadgeClassName)}
          >
            {t('tariffs.currentBadge')}
          </Badge>
        )}
        {savingPct > 0 && (
          <Badge variant="success" className="w-fit text-xs mt-1">
            {t('tariffs.saving', { pct: savingPct })}
          </Badge>
        )}
      </CardHeader>

      <CardContent className="flex flex-col flex-1 gap-3">
        <div className="flex-1">
          <div className="flex items-baseline gap-1">
            <span className="text-2xl font-bold">{formatNumber(tariff.price_rub)}</span>
            <span className="text-sm text-muted-foreground">₽</span>
          </div>
          <p className="text-xs text-muted-foreground mt-0.5">
            {formatNumber(tariff.monthly_base_rub)} ₽{t('tariffs.perMonth')}
          </p>
        </div>

        <Button
          className={cn(
            'w-full transition-[background-color,box-shadow,transform,filter] duration-200',
            isCurrent ? tariffRenewButtonClassName : tariffChangeCtaButtonClassName,
          )}
          variant="default"
          size="sm"
          type="button"
          onClick={() => onSelect(tariff)}
        >
          {ctaLabel}
        </Button>
      </CardContent>
    </Card>
  )
}

function FeatureLine({
  icon: Icon,
  children,
}: {
  icon: LucideIcon
  children: React.ReactNode
}) {
  return (
    <li className="flex items-center gap-2 text-muted-foreground">
      <Icon size={14} className="text-primary shrink-0" strokeWidth={2} />
      {children}
    </li>
  )
}

/**
 * Трафик на карточке тарифа. При отдельном лимите обхода «Безлимит трафика» — неправда:
 * безлимитны только обычные серверы, а на обходе свой месячный лимит. Название обхода —
 * как его задал админ (TRAFFIC_BYPASS_LABEL), без склонения и со строчной: оно внутри фразы.
 */
function trafficLineText(head: TariffItem, t: TFunction): string {
  if (head.traffic_gb) return t('tariffs.traffic', { n: head.traffic_gb })
  if (head.bypass_gb == null) return t('tariffs.trafficUnlimited')
  return t('tariffs.trafficUnlimitedPlusBypass', {
    n: formatNumber(head.bypass_gb),
    label: bypassLabelInline(head.bypass_label),
  })
}

function pluralizeMonths(n: number): string {
  if (n === 1) return '1 месяц'
  if (n >= 2 && n <= 4) return `${n} месяца`
  return `${n} месяцев`
}

/** Короткая форма для узких карточек: рядом с плашкой «−N %» полная не влезает. */
function pluralizeMonthsShort(n: number): string {
  return `${n} мес.`
}

/** Плашка экономии — тот же вид, что у процентов в редакторе тарифа в админке. */
const periodSavingBadgeClassName =
  'rounded-md bg-emerald-500/15 px-1.5 py-0.5 text-[0.7rem] font-medium leading-tight tabular-nums text-emerald-600 dark:text-emerald-400'

function isSubscriptionActive(expireAt: string | null | undefined): boolean {
  if (expireAt == null || expireAt === '') return false
  const t = Date.parse(expireAt)
  if (Number.isNaN(t)) return false
  return t > Date.now()
}

/** Бейдж «Текущий»: бордер и фон от --primary пресета. */
const tariffCurrentBadgeClassName =
  'border border-primary bg-primary/10 text-primary dark:bg-primary/15 dark:text-primary shadow-none'

/** «Текущий» на карточке тарифа — серая метка без цвета темы: цветом выделен только популярный. */
const tariffCurrentPlanBadgeClassName =
  'border border-border bg-muted/60 text-muted-foreground shadow-none dark:border-white/15 dark:bg-white/[0.06] dark:text-foreground/80'

/**
 * Текущий тариф: inset primary + те же «плавающие» тени, что у Card (иначе в dark побеждает dark:shadow карточки).
 */
const tariffCurrentCardClassName =
  'border-primary shadow-[inset_0_0_0_1px_hsl(var(--primary)),0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] dark:shadow-[inset_0_0_0_1px_hsl(var(--primary)),0_12px_30px_rgba(3,10,24,0.42),inset_0_1px_0_rgba(255,255,255,0.03)] hover:-translate-y-0.5 hover:border-primary hover:shadow-[inset_0_0_0_1px_hsl(var(--primary)),0_10px_32px_-8px_hsl(var(--primary)/0.42),0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] dark:hover:shadow-[inset_0_0_0_1px_hsl(var(--primary)),0_14px_40px_-6px_hsl(var(--primary)/0.38),0_12px_30px_rgba(3,10,24,0.5),inset_0_1px_0_rgba(255,255,255,0.05)]'

/** Карточки остальных тарифов — ховер без смены яркости всей карточки. */
const tariffOtherCardHoverClassName =
  'hover:border-primary/35 hover:shadow-[0_10px_28px_-12px_hsl(var(--foreground)/0.12)] dark:hover:shadow-[0_12px_32px_-10px_hsl(var(--primary)/0.22)]'

/** Базовая тень как у `Card` (карточки тарифов на сетке). */
const tariffCardShadowClassName =
  'shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] dark:shadow-[0_12px_30px_rgba(3,10,24,0.42),inset_0_1px_0_rgba(255,255,255,0.03)]'

/** Кнопка «Продлить»: явный primary (тема уже задаёт --primary в :root / .dark). */
const tariffRenewButtonClassName =
  'bg-primary text-primary-foreground shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] hover:brightness-110 hover:shadow-[0_6px_20px_-6px_hsl(var(--primary)/0.55)] active:scale-[0.98]'

/** Кнопки «Сменить тариф» / «Выбрать»: тот же акцент, что и «Продлить» (--primary пресета). */
const tariffChangeCtaButtonClassName =
  'bg-primary text-primary-foreground shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] hover:brightness-110 hover:shadow-[0_6px_20px_-6px_hsl(var(--primary)/0.55)] active:scale-[0.98]'

function TariffsSkeleton() {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
      {[1, 2, 3].map((i) => (
        <Card key={i}>
          <CardHeader>
            <Skeleton className="h-5 w-1/2" />
            <Skeleton className="mt-2 h-8 w-2/3" />
          </CardHeader>
          <CardContent className="space-y-2">
            <Skeleton className="h-3 w-full" />
            <Skeleton className="h-3 w-3/4" />
            <Skeleton className="mt-4 h-9 w-full rounded-md" />
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

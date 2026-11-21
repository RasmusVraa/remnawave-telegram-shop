import { type ReactNode, type UIEvent, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  BarChart3,
  Users,
  CreditCard,
  TicketPercent,
  Zap,
  Gem,
  Handshake,
  Megaphone,
  Network,
  Server,
  Activity,
  RefreshCw,
  LayoutDashboard,
  ChevronLeft,
  ShieldCheck,
  X,
  SlidersHorizontal,
  Radar,
  PanelLeftClose,
  PanelLeftOpen,
  Star,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

import { cn } from '@/lib/utils'
import { useAdminBootstrap } from '../hooks/useAdminBootstrap'
import { useAdminPartnerPending } from '../hooks/useAdminPartners'
import { useAdminReviewsPending } from '../hooks/useAdminReviews'
import { useAdminAttention } from '../monitoring/useMonitoring'
import { AdminChrome } from './AdminChrome'
import { AdminBreadcrumbs } from './AdminBreadcrumbs'
import { AdminPageMetaContext, type AdminPageMeta } from './useAdminPageMeta'
import { AdminShellProvider, useAdminShell } from './AdminShellContext'
import { useAdminLeftEdgeSwipe } from '../hooks/useAdminLeftEdgeSwipe'
import { useAdminSelectionHaptics } from '../hooks/useAdminSelectionHaptics'
import { useAdminMobileNavWidth } from '../hooks/useAdminMobileNavWidth'
import { useAdminSidebarCollapsed } from '../hooks/useAdminSidebarCollapsed'
import { useAdminNavTooltip } from './AdminNavTooltip'

interface AdminLayoutProps {
  children: ReactNode
  /**
   * Мета страницы: хвост хлебных крошек и режим шапки на телефоне.
   *
   * Пропом, а не через `useAdminPageMeta`: страницы сами рендерят
   * `<AdminLayout>`, то есть находятся НАД провайдером контекста, и хук у них
   * всегда получал null — хвост крошек молча не доезжал, вместо «@username»
   * оставался запасной «Пользователь #id». Хук оставлен для того, что живёт
   * внутри layout.
   */
  meta?: AdminPageMeta
}

interface AdminNavItem {
  to: string
  icon: LucideIcon
  labelKey: string
  condition?: boolean
  /** Счётчик несделанных дел: заявки партнёров плюс заявки на вывод. */
  badge?: number
  /** Красный бейдж — то, что горит сейчас (инциденты «Мониторинга»). */
  badgeAlert?: boolean
}

interface AdminNavGroup {
  labelKey: string
  items: AdminNavItem[]
}

/**
 * Прокрутка бокового меню между страницами. Каждая страница рендерит свой
 * `<AdminLayout>`, меню монтируется заново и без этого прыгало в начало —
 * выбранный внизу пункт («Настройки бота») уезжал из вида.
 */
let savedNavScrollTop = 0
const NAV_SCROLL_MARGIN_PX = 16
/** Зазор между низом меню и краем окна. */
const NAV_BOTTOM_GAP_PX = 16
/** Ниже этого меню не сжимаем даже в очень низком окне. */
const NAV_MIN_HEIGHT_PX = 160
/** Сколько после перехода держать выбранный пункт в виде, пока меню меняет высоту. */
const NAV_REVEAL_WINDOW_MS = 1000

export function AdminLayout({ children, meta }: AdminLayoutProps) {
  return (
    <AdminShellProvider>
      <AdminLayoutInner meta={meta}>{children}</AdminLayoutInner>
    </AdminShellProvider>
  )
}

function AdminLayoutInner({ children, meta }: AdminLayoutProps) {
  const { t } = useTranslation()
  const location = useLocation()
  const { data: bootstrap } = useAdminBootstrap()
  const {
    mobileNavOpen,
    mobileNavOffsetPx,
    mobileNavDragging,
    closeMobileNav,
    mobileHeaderVisible,
  } = useAdminShell()
  const panelWidth = useAdminMobileNavWidth()
  const navProgress = panelWidth > 0 ? Math.min(1, mobileNavOffsetPx / panelWidth) : 0
  const navLayerVisible = mobileNavOpen || mobileNavOffsetPx > 0
  /*
   * Кликабельность слоя привязана к видимой позиции панели, а не к флагу
   * `mobileNavOpen`: флаг живёт ещё 300мс после закрытия, и всё это время
   * прозрачный бэкдроп на весь экран съедал первый тап по странице.
   */
  const navLayerInteractive = mobileNavOffsetPx > 0
  const [pageMeta, setPageMeta] = useState<AdminPageMeta>({})
  /*
   * Значение контекста обязано быть стабильным: `useAdminPageMeta` держит ctx
   * в зависимостях эффекта, и новый объект на каждый рендер запускал круг
   * «сбросить мету → выставить заново». До паузы доживал сброс, поэтому в
   * хлебных крошках вместо «@username» всегда стоял запасной «Пользователь #id».
   */
  const metaContextValue = useMemo(() => ({ setMeta: setPageMeta }), [])
  // Проп страницы важнее того, что выставили изнутри layout.
  const effectiveMeta = meta ? { ...pageMeta, ...meta } : pageMeta

  useAdminLeftEdgeSwipe(true)
  useAdminSelectionHaptics()

  const { collapsed, toggle: toggleCollapsed } = useAdminSidebarCollapsed()
  const navTip = useAdminNavTooltip(collapsed)

  const salesModeTariffs = bootstrap?.sales_mode === 'tariffs'
  const loyaltyEnabled = bootstrap?.loyalty_enabled ?? false
  const partnerEnabled = bootstrap?.partner_enabled ?? false
  // Бейдж считает заявки вместе с выплатами: иначе выплаты висят
  // незамеченными по несколько дней.
  const partnerPending = useAdminPartnerPending()
  const reviewsPending = useAdminReviewsPending()
  // Инциденты «Мониторинга» видно с любой страницы админки.
  const attention = useAdminAttention()
  const incidentCount = attention.data?.items?.length ?? 0

  const navAsideRef = useRef<HTMLElement>(null)
  const navScrollRef = useRef<HTMLDivElement>(null)

  /** До какого момента подкручивать меню к выбранному пункту при пересчёте высоты. */
  const revealUntilRef = useRef(0)

  /** Докрутить меню так, чтобы выбранный пункт был виден целиком. */
  function revealActiveNavItem() {
    const el = navScrollRef.current
    const active = el?.querySelector<HTMLElement>('[aria-current="page"]')
    if (!el || !active) return
    const box = el.getBoundingClientRect()
    const item = active.getBoundingClientRect()
    if (item.bottom > box.bottom - NAV_SCROLL_MARGIN_PX) el.scrollTop += item.bottom - box.bottom + NAV_SCROLL_MARGIN_PX
    else if (item.top < box.top + NAV_SCROLL_MARGIN_PX) el.scrollTop -= box.top + NAV_SCROLL_MARGIN_PX - item.top
  }

  /*
   * Меню кончается у нижнего края окна. Прежний max-h считал верх меню равным
   * «прилипшему» (сразу под шапкой), а до прокрутки страницы оно стоит ниже —
   * на высоту хлебных крошек, и низ списка уходил за окно: последние пункты
   * не докрутить. Верх меняется при прокрутке страницы, поэтому пересчёт.
   */
  useLayoutEffect(() => {
    const aside = navAsideRef.current
    if (!aside) return
    let frame = 0
    const fit = () => {
      frame = 0
      // Не выше «прилипшего» меню: если страница короче меню, sticky не
      // держит, верх уезжает вверх, и без этой границы меню растягивалось бы.
      const stickyTop = parseFloat(getComputedStyle(aside).top) || 0
      const top = Math.max(aside.getBoundingClientRect().top, stickyTop)
      aside.style.maxHeight = `${Math.max(NAV_MIN_HEIGHT_PX, window.innerHeight - top - NAV_BOTTOM_GAP_PX)}px`
      // После перехода страница отматывается наверх, меню сжимается, и
      // выбранный пункт уходил вниз из вида.
      if (performance.now() < revealUntilRef.current) revealActiveNavItem()
    }
    const schedule = (e: Event) => {
      // Прокрутка самого меню его высоту не меняет.
      if (e.target instanceof Node && aside.contains(e.target)) return
      if (!frame) frame = requestAnimationFrame(fit)
    }
    fit()
    // capture: прокрутку ловим у любого контейнера, а не только у window.
    document.addEventListener('scroll', schedule, { passive: true, capture: true })
    window.addEventListener('resize', schedule)
    return () => {
      cancelAnimationFrame(frame)
      document.removeEventListener('scroll', schedule, { capture: true })
      window.removeEventListener('resize', schedule)
    }
  }, [])

  // До отрисовки, иначе меню мелькнёт в начале списка и прыгнет вниз.
  useLayoutEffect(() => {
    const el = navScrollRef.current
    if (el) el.scrollTop = savedNavScrollTop
  }, [])

  /*
   * Прямой заход по ссылке или F5 на нижней странице: выбранный пункт должен
   * быть виден. Повторяем, когда меняется состав пунктов — «Лояльность» и
   * «Партнёры» появляются после загрузки bootstrap и сдвигают список вниз.
   */
  useLayoutEffect(() => {
    revealUntilRef.current = performance.now() + NAV_REVEAL_WINDOW_MS
    revealActiveNavItem()
  }, [location.pathname, collapsed, loyaltyEnabled, partnerEnabled])

  function onNavScroll(e: UIEvent<HTMLDivElement>) {
    savedNavScrollTop = e.currentTarget.scrollTop
    navTip.hide()
  }

  const navGroups: AdminNavGroup[] = [
    {
      labelKey: 'admin.nav.group.overview',
      items: [
        { to: '/admin', icon: LayoutDashboard, labelKey: 'admin.nav.dashboard' },
        {
          to: '/admin/monitoring',
          icon: Radar,
          labelKey: 'admin.nav.monitoring',
          badge: incidentCount,
          badgeAlert: true,
        },
        { to: '/admin/stats', icon: BarChart3, labelKey: 'admin.nav.stats' },
        { to: '/admin/nodes', icon: Network, labelKey: 'admin.nav.nodes' },
      ],
    },
    {
      labelKey: 'admin.nav.group.management',
      items: [
        { to: '/admin/users', icon: Users, labelKey: 'admin.nav.users' },
        { to: '/admin/payments', icon: CreditCard, labelKey: 'admin.nav.payments' },
        { to: '/admin/reviews', icon: Star, labelKey: 'admin.nav.reviews', badge: reviewsPending.data?.counts.pending },
      ],
    },
    {
      labelKey: 'admin.nav.group.marketing',
      items: [
        { to: '/admin/promos', icon: TicketPercent, labelKey: 'admin.nav.promos' },
        { to: '/admin/broadcast', icon: Megaphone, labelKey: 'admin.nav.broadcast' },
        { to: '/admin/loyalty', icon: Gem, labelKey: 'admin.nav.loyalty', condition: loyaltyEnabled },
        {
          to: '/admin/partners',
          icon: Handshake,
          labelKey: 'admin.nav.partners',
          condition: partnerEnabled,
          badge: partnerPending.data?.total,
        },
      ],
    },
    {
      labelKey: 'admin.nav.group.system',
      items: [
        // Без condition: под списком тарифов теперь живут настройки продукта
        // (триал, HWID, курс звёзд), которые нужны в любом режиме продаж.
        { to: '/admin/tariffs', icon: Zap, labelKey: 'admin.nav.tariffs' },
        { to: '/admin/settings', icon: SlidersHorizontal, labelKey: 'admin.nav.settings' },
        { to: '/admin/status', icon: Activity, labelKey: 'admin.nav.status' },
        { to: '/admin/infra', icon: Server, labelKey: 'admin.nav.infra' },
        { to: '/admin/sync', icon: RefreshCw, labelKey: 'admin.nav.sync' },
      ],
    },
  ]

  function isActive(to: string): boolean {
    if (to === '/admin') return location.pathname === '/admin'
    return location.pathname === to || location.pathname.startsWith(`${to}/`)
  }

  const toggleLabel = t(collapsed ? 'admin.nav.expand' : 'admin.nav.collapse')
  const toggleHotkey = /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘B' : 'Ctrl+B'

  function onToggleCollapsed() {
    navTip.hide()
    toggleCollapsed()
  }

  /**
   * `compact` — свёрнутое меню на ПК: одни иконки, подписи всплывают сбоку.
   * Шторка на телефоне всегда полная, поэтому флаг считается тут, а не
   * берётся из `collapsed` напрямую.
   */
  function renderSidebar(desktop: boolean) {
    const compact = desktop && collapsed
    const tipFor = (label: string, hint?: string) => (compact ? navTip.bind(label, hint) : {})
    // Подпись не пропадает рывком: ширина и прозрачность уходят вместе с колонкой.
    const labelCn = cn(
      'truncate transition-[max-width,opacity] duration-200 ease-out',
      compact ? 'max-w-0 opacity-0' : 'max-w-[12rem] opacity-100',
    )

    return (
      <>
        {/* С кнопкой сворачивания шапке тесно: без px-1 и с gap-2 «Админ-панель» влезает целиком. */}
        <div className={cn('mb-6 flex items-center', compact ? 'flex-col gap-2' : desktop ? 'gap-2' : 'gap-2.5 px-1')}>
          <div
            className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/15 dark:bg-primary/25"
            {...tipFor(t('admin.dashboard.title'), bootstrap?.version)}
          >
            <ShieldCheck className="size-5 text-primary" />
          </div>
          {!compact && (
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-semibold">{t('admin.dashboard.title')}</p>
              {bootstrap?.version && (
                <p className="truncate text-xs text-muted-foreground" title={bootstrap.version}>
                  {bootstrap.version}
                </p>
              )}
            </div>
          )}
          {desktop && (
            <button
              type="button"
              onClick={onToggleCollapsed}
              aria-label={toggleLabel}
              aria-expanded={!collapsed}
              aria-keyshortcuts="Control+B Meta+B"
              title={compact ? undefined : `${toggleLabel} (${toggleHotkey})`}
              className="flex size-7 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent/80 hover:text-foreground"
              {...tipFor(toggleLabel, toggleHotkey)}
            >
              {compact ? <PanelLeftOpen className="size-[18px]" /> : <PanelLeftClose className="size-[18px]" />}
            </button>
          )}
        </div>

        <nav className="space-y-5" aria-label={t('admin.nav.label')}>
          {navGroups.map((group) => {
            const visible = group.items.filter((item) => item.condition !== false)
            if (visible.length === 0) return null
            return (
              <div key={group.labelKey}>
                {compact ? (
                  // Вместо заголовка группы — короткая черта: деление на группы остаётся видно.
                  <p className="mb-2 flex h-4 items-center justify-center" aria-hidden>
                    <span className="h-px w-5 bg-border" />
                  </p>
                ) : (
                  <p className="mb-2 px-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
                    {t(group.labelKey)}
                  </p>
                )}
                <ul className="space-y-0.5">
                  {visible.map((item) => {
                    const Icon = item.icon
                    const active = isActive(item.to)
                    const label = t(item.labelKey)
                    return (
                      <li key={item.to}>
                        <Link
                          to={item.to}
                          onClick={closeMobileNav}
                          aria-current={active ? 'page' : undefined}
                          className={cn(
                            'relative flex items-center rounded-lg py-2 text-sm font-medium transition-colors',
                            // Высота как у развёрнутого: скрытая подпись всё равно держит строку.
                            compact ? 'justify-center' : 'gap-2.5 px-3',
                            active
                              ? 'bg-primary/15 text-primary shadow-sm dark:bg-primary/20'
                              : 'text-muted-foreground hover:bg-accent/80 hover:text-foreground',
                          )}
                          {...tipFor(label)}
                        >
                          <Icon className={cn('shrink-0', compact ? 'size-[18px]' : 'size-4', active && 'text-primary')} />
                          <span className={labelCn}>{label}</span>
                          {item.badge ? (
                            <span
                              className={cn(
                                'shrink-0 rounded-full text-[10px] font-semibold',
                                // В свёрнутом бейдж садится на угол иконки.
                                compact
                                  ? 'absolute left-1/2 top-0.5 ml-1 h-4 min-w-[1rem] px-1 text-center leading-4 ring-2 ring-background'
                                  : 'ml-auto px-1.5 py-0.5',
                                item.badgeAlert ? 'bg-rose-500 text-white' : 'bg-primary text-primary-foreground',
                              )}
                            >
                              {item.badge}
                            </span>
                          ) : null}
                        </Link>
                      </li>
                    )
                  })}
                </ul>
              </div>
            )
          })}
        </nav>

        <div className="mt-auto border-t border-border/50 pt-4">
          <Link
            to="/dashboard"
            onClick={closeMobileNav}
            className={cn(
              'flex items-center rounded-lg py-2 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground',
              compact ? 'justify-center' : 'gap-2 px-3',
            )}
            {...tipFor(t('admin.backToCabinet'))}
          >
            <ChevronLeft className="size-4 shrink-0" />
            <span className={labelCn}>{t('admin.backToCabinet')}</span>
          </Link>
        </div>
      </>
    )
  }

  return (
    <AdminPageMetaContext.Provider value={metaContextValue}>
      <AdminChrome hideMobileHeader={effectiveMeta.mobileBareHeader}>
        <div className="admin-shell relative z-[1] mx-auto w-full max-w-7xl px-3 pb-8 pt-2 sm:px-4 sm:pt-4">
          <div
            className={cn(
              'fixed inset-0 z-[210] lg:hidden',
              !navLayerInteractive && 'pointer-events-none',
            )}
            aria-hidden={!navLayerVisible}
          >
            <div
              className={cn(
                'absolute inset-0 bg-black/50 backdrop-blur-sm',
                !mobileNavDragging && 'transition-opacity duration-300 ease-out',
              )}
              style={{ opacity: navProgress }}
              onClick={closeMobileNav}
            />
            <aside
              data-admin-nav-panel
              className={cn(
                'absolute bottom-0 left-0 flex w-72 max-w-[85vw] flex-col overflow-y-auto border-r border-border bg-background p-4 shadow-xl will-change-transform',
                !mobileNavDragging && 'transition-[transform,top] duration-300 ease-out',
                mobileHeaderVisible
                  ? 'top-[calc(3.5rem+var(--cabinet-tg-safe-top))]'
                  : 'top-[var(--cabinet-tg-safe-top)]',
              )}
              style={{ transform: `translateX(${mobileNavOffsetPx - panelWidth}px)` }}
            >
              <button
                type="button"
                onClick={closeMobileNav}
                className="mb-4 self-end rounded-lg p-2 hover:bg-accent"
                aria-label={t('common.close')}
              >
                <X className="size-5" />
              </button>
              {renderSidebar(false)}
            </aside>
          </div>

          <div
            className={cn(
              'flex flex-col gap-4 lg:grid lg:items-start lg:gap-x-8 lg:gap-y-4 lg:transition-[grid-template-columns] lg:duration-200 lg:ease-out',
              collapsed ? 'lg:grid-cols-[4.25rem_minmax(0,1fr)]' : 'lg:grid-cols-[14rem_minmax(0,1fr)]',
            )}
          >
            <AdminBreadcrumbs
              pathname={location.pathname}
              pageMeta={effectiveMeta}
              className={cn(
                'lg:col-start-2 lg:row-start-1',
                effectiveMeta.mobileBareHeader && 'max-lg:hidden',
              )}
            />

            <aside
              ref={navAsideRef}
              className="hidden w-full min-w-0 shrink-0 lg:col-start-1 lg:row-start-2 lg:z-20 lg:flex lg:max-h-[calc(100dvh-3.75rem-var(--cabinet-tg-safe-top))] lg:flex-col lg:self-start lg:sticky lg:top-[calc(3.75rem+var(--cabinet-tg-safe-top))]">
              {/*
               * Прокручивается карточка, а не aside: у aside полоса рисовалась
               * справа от скруглённой рамки. Справа отступ меньше на 10px —
               * это место полосы (admin-nav-scroll в index.css).
               */}
              <div
                ref={navScrollRef}
                onScroll={onNavScroll}
                className={cn(
                  'admin-nav-scroll min-h-0 overflow-y-auto overflow-x-hidden overscroll-y-contain rounded-xl border border-border/60 bg-card/50 py-4 backdrop-blur-sm transition-[padding] duration-200 ease-out',
                  collapsed ? 'pl-2.5 pr-0' : 'pl-4 pr-1.5',
                )}
              >
                {renderSidebar(true)}
              </div>
            </aside>
            {navTip.node}

            <main className="relative z-[1] min-w-0 overflow-x-clip lg:col-start-2 lg:row-start-2">
              {children}
            </main>
          </div>
        </div>
      </AdminChrome>
    </AdminPageMetaContext.Provider>
  )
}

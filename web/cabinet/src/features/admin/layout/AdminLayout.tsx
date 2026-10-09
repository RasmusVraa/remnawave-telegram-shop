import { type ReactNode, useMemo, useState } from 'react'
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
  Server,
  Activity,
  RefreshCw,
  LayoutDashboard,
  ChevronLeft,
  ShieldCheck,
  X,
  SlidersHorizontal,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

import { cn } from '@/lib/utils'
import { useAdminBootstrap } from '../hooks/useAdminBootstrap'
import { useAdminPartnerPending } from '../hooks/useAdminPartners'
import { AdminChrome } from './AdminChrome'
import { AdminBreadcrumbs } from './AdminBreadcrumbs'
import { AdminPageMetaContext, type AdminPageMeta } from './useAdminPageMeta'
import { AdminShellProvider, useAdminShell } from './AdminShellContext'
import { useAdminLeftEdgeSwipe } from '../hooks/useAdminLeftEdgeSwipe'
import { useAdminMobileNavWidth } from '../hooks/useAdminMobileNavWidth'

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
}

interface AdminNavGroup {
  labelKey: string
  items: AdminNavItem[]
}

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

  const salesModeTariffs = bootstrap?.sales_mode === 'tariffs'
  const loyaltyEnabled = bootstrap?.loyalty_enabled ?? false
  const partnerEnabled = bootstrap?.partner_enabled ?? false
  // Бейдж считает заявки вместе с выплатами: иначе выплаты висят
  // незамеченными по несколько дней.
  const partnerPending = useAdminPartnerPending()

  const navGroups: AdminNavGroup[] = [
    {
      labelKey: 'admin.nav.group.overview',
      items: [
        { to: '/admin', icon: LayoutDashboard, labelKey: 'admin.nav.dashboard' },
        { to: '/admin/stats', icon: BarChart3, labelKey: 'admin.nav.stats' },
      ],
    },
    {
      labelKey: 'admin.nav.group.management',
      items: [
        { to: '/admin/users', icon: Users, labelKey: 'admin.nav.users' },
        { to: '/admin/payments', icon: CreditCard, labelKey: 'admin.nav.payments' },
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

  const sidebarContent = (
    <>
      <div className="mb-6 flex items-center gap-2.5 px-1">
        <div className="flex size-9 items-center justify-center rounded-lg bg-primary/15 dark:bg-primary/25">
          <ShieldCheck className="size-5 text-primary" />
        </div>
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold">{t('admin.dashboard.title')}</p>
          {bootstrap?.version && (
            <p className="truncate text-xs text-muted-foreground" title={bootstrap.version}>
              {bootstrap.version}
            </p>
          )}
        </div>
      </div>

      <nav className="space-y-5" aria-label={t('admin.nav.label')}>
        {navGroups.map((group) => {
          const visible = group.items.filter((item) => item.condition !== false)
          if (visible.length === 0) return null
          return (
            <div key={group.labelKey}>
              <p className="mb-2 px-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
                {t(group.labelKey)}
              </p>
              <ul className="space-y-0.5">
                {visible.map((item) => {
                  const Icon = item.icon
                  const active = isActive(item.to)
                  return (
                    <li key={item.to}>
                      <Link
                        to={item.to}
                        onClick={closeMobileNav}
                        className={cn(
                          'flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                          active
                            ? 'bg-primary/15 text-primary shadow-sm dark:bg-primary/20'
                            : 'text-muted-foreground hover:bg-accent/80 hover:text-foreground',
                        )}
                      >
                        <Icon className={cn('size-4 shrink-0', active && 'text-primary')} />
                        <span className="truncate">{t(item.labelKey)}</span>
                        {item.badge ? (
                          <span className="ml-auto shrink-0 rounded-full bg-primary px-1.5 py-0.5 text-[10px] font-semibold text-primary-foreground">
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
          className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <ChevronLeft className="size-4" />
          {t('admin.backToCabinet')}
        </Link>
      </div>
    </>
  )

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
              {sidebarContent}
            </aside>
          </div>

          <div className="flex flex-col gap-4 lg:grid lg:grid-cols-[14rem_minmax(0,1fr)] lg:items-start lg:gap-x-8 lg:gap-y-4">
            <AdminBreadcrumbs
              pathname={location.pathname}
              pageMeta={effectiveMeta}
              className={cn(
                'lg:col-start-2 lg:row-start-1',
                effectiveMeta.mobileBareHeader && 'max-lg:hidden',
              )}
            />

            <aside className="hidden w-56 shrink-0 lg:col-start-1 lg:row-start-2 lg:z-20 lg:flex lg:max-h-[calc(100dvh-3.75rem-var(--cabinet-tg-safe-top))] lg:flex-col lg:self-start lg:overflow-y-auto lg:overscroll-y-contain lg:sticky lg:top-[calc(3.75rem+var(--cabinet-tg-safe-top))]">
              <div className="rounded-xl border border-border/60 bg-card/50 p-4 backdrop-blur-sm">
                {sidebarContent}
              </div>
            </aside>

            <main className="relative z-[1] min-w-0 overflow-x-clip lg:col-start-2 lg:row-start-2">
              {children}
            </main>
          </div>
        </div>
      </AdminChrome>
    </AdminPageMetaContext.Provider>
  )
}

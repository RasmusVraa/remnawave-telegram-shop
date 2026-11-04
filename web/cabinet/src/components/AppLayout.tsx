import { type ReactNode, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  Activity,
  Home,
  Zap,
  MessageCircle,
  User,
  Menu,
  X,
  TicketPercent,
  Users,
  Handshake,
  LogOut,
  ShieldCheck,
  FileText,
  Newspaper,
  Star,
  ChevronRight,
  type LucideIcon,
} from 'lucide-react'

import { ConfirmModal } from './ConfirmModal'
import { Logo } from './Logo'
import { Button } from './ui/button'
import { cn } from '@/lib/utils'
import { FortuneWheelIcon } from '@/features/fortune/FortuneWheelIcon'
import { hapticImpactLight } from '@/lib/telegram-web-app'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { useAuthStore } from '@/store/auth'
import { CabinetDecorLayer } from '@/features/decor/CabinetDecorLayer'
import { CabinetDecorHeader } from '@/features/decor/CabinetDecorHeader'
import { DecorNavIcon } from '@/features/decor/decorNavIcons'
import { useCabinetDecorTheme } from '@/features/decor/useCabinetDecorTheme'
import { CabinetOnboarding } from '@/features/onboarding/CabinetOnboarding'
import { NotificationBell } from '@/features/notifications/NotificationBell'
import { useSupportSummary } from '@/features/support/useSupportChat'

interface AppLayoutProps {
  children: ReactNode
}

type NavItem = {
  to: string
  icon: typeof Home
  labelKey: string
  /** Доп. пути, при которых считаем пункт активным (например /connections → подписка). */
  activePrefixes?: string[]
}

/*
 * «Подписка» слилась с главной (SPEC редизайна главной), поэтому гид
 * подключения /connections подсвечивает «Главную».
 */
const baseNavItems: NavItem[] = [
  { to: '/dashboard', icon: Home, labelKey: 'nav.dashboard', activePrefixes: ['/connections'] },
  { to: '/tariffs', icon: Zap, labelKey: 'nav.tariffs' },
  { to: '/support', icon: MessageCircle, labelKey: 'nav.support' },
  { to: '/profile', icon: User, labelKey: 'nav.profile', activePrefixes: ['/accounts', '/link'] },
]

const mobileBottomNavItems: NavItem[] = baseNavItems

function navItemActive(pathname: string, item: NavItem): boolean {
  if (pathname === item.to) return true
  if (item.activePrefixes) {
    return item.activePrefixes.some((p) => pathname === p || pathname.startsWith(`${p}/`))
  }
  return pathname.startsWith(`${item.to}/`)
}

function overflowActive(pathname: string, to: string): boolean {
  return pathname === to || pathname.startsWith(`${to}/`)
}

/** Активный пункт в выпадающем меню и нижней мобильной навигации (как у текста ссылки). */
const navAccentActiveClass = 'text-primary'

/**
 * Пункт меню ☰. Внутренний (to) или внешний (href — новости и отзывы ведут
 * в Telegram), со своим «хвостом» справа: подсказка или бейдж.
 */
type MenuItem = {
  key: string
  icon: LucideIcon
  label: string
  to?: string
  href?: string
  extra?: ReactNode
  decorTo?: string
}

function MenuEntry({
  item,
  className,
  children,
}: {
  item: MenuItem
  className: string
  children: ReactNode
}) {
  if (item.href) {
    return (
      <a href={item.href} target="_blank" rel="noopener noreferrer" role="menuitem" className={className}>
        {children}
      </a>
    )
  }
  return (
    <Link to={item.to ?? '/'} role="menuitem" className={className}>
      {children}
    </Link>
  )
}

function NavIconWithDot({ showDot, children }: { showDot: boolean; children: ReactNode }) {
  return (
    <span className="relative inline-flex">
      {children}
      {showDot ? <span className="absolute -right-0.5 -top-0.5 size-2 rounded-full bg-primary ring-2 ring-card" aria-hidden /> : null}
    </span>
  )
}

export function AppLayout({ children }: AppLayoutProps) {
  const { t } = useTranslation()
  const location = useLocation()
  const decorTheme = useCabinetDecorTheme()
  const { data: bootstrap } = useAuthBootstrap()
  // Публичные страницы (переход по deep link) тоже в этой шапке: без сессии
  // колокольчик спрашивал бы счётчик и получал 401.
  const hasSession = Boolean(useAuthStore((st) => st.accessToken))
  const user = useAuthStore((s) => s.user)
  const partnerMaxPercent = bootstrap?.partner_max_percent ?? 0
  const newsUrl = bootstrap?.site_links?.channel?.trim()
  const feedbackUrl = bootstrap?.site_links?.feedback?.trim()
  /*
   * Меню ☰ по разделам. Пункты нижней навигации и шапки (Главная, Тарифы,
   * Поддержка, Профиль) сюда не входят — это были дубли; из дублей шапки ПК
   * осталась только «Админ-панель» внизу, перед «Выйти».
   * Выключенные разделы убираются: пункт, ведущий на «недоступно», — это
   * обещание, которого продукт не выполняет.
   */
  const menuGroups = useMemo(() => {
    const bonuses: MenuItem[] = [
      { key: 'promo', icon: TicketPercent, label: t('nav.promocodes'), to: '/promocodes', decorTo: '/promocodes' },
      {
        key: 'referral',
        icon: Users,
        label: t('nav.referral'),
        to: '/referral',
        decorTo: '/referral',
        extra: <span className="ml-auto shrink-0 text-xs text-muted-foreground">{t('home.menuReferralHint')}</span>,
      },
    ]
    if (bootstrap?.partner_nav_visible !== false) {
      bonuses.push({
        key: 'partner',
        icon: Handshake,
        label: t('nav.partner'),
        to: '/partner',
        decorTo: '/partner',
        // Ставка прямо в меню: партнёрка — единственный пункт, который что-то
        // обещает, и обещание должно быть видно до перехода.
        extra:
          partnerMaxPercent > 0 ? (
            <span className="ml-auto shrink-0 rounded-full bg-primary/15 px-2 py-0.5 text-xs font-semibold text-primary">
              {t('nav.partnerBadge', { percent: `${partnerMaxPercent}%` })}
            </span>
          ) : undefined,
      })
    }
    if (bootstrap?.fortune_nav_visible !== false) {
      bonuses.push({ key: 'fortune', icon: FortuneWheelIcon, label: t('nav.fortune'), to: '/fortune', decorTo: '/fortune' })
    }
    const about: MenuItem[] = [{ key: 'info', icon: FileText, label: t('nav.info'), to: '/support#cabinet-info' }]
    if (bootstrap?.status_nav_visible !== false) {
      about.push({ key: 'servers', icon: Activity, label: t('nav.servers'), to: '/status', decorTo: '/status' })
    }
    if (newsUrl) about.push({ key: 'news', icon: Newspaper, label: t('dashboard.newsCardTitle'), href: newsUrl })
    if (feedbackUrl) about.push({ key: 'feedback', icon: Star, label: t('dashboard.feedbackCardTitle'), href: feedbackUrl })
    return [
      { key: 'bonuses', title: t('home.menuBonuses'), items: bonuses },
      { key: 'about', title: t('home.menuAbout'), items: about },
    ]
  }, [t, bootstrap?.partner_nav_visible, bootstrap?.fortune_nav_visible, bootstrap?.status_nav_visible, partnerMaxPercent, newsUrl, feedbackUrl])
  const navItems = useMemo(() => {
    if (user?.is_admin) {
      return [
        ...baseNavItems,
        { to: '/admin', icon: ShieldCheck, labelKey: 'admin.dashboard.title' as const, activePrefixes: [] as string[] },
      ]
    }
    return baseNavItems
  }, [user?.is_admin])
  const logout = useAuthStore((s) => s.logout)
  const supportChatEnabled = Boolean(bootstrap?.support_chat_enabled)
  const { data: supportSummary } = useSupportSummary(supportChatEnabled, 60_000)
  const supportUnread = supportSummary?.unread_count ?? 0
  const [menuOpen, setMenuOpen] = useState(false)
  /*
   * Анимация меню: menuMounted держит разметку, пока идёт уход, menuVisible
   * переключает классы перехода (тот же приём, что у шторок). Снимаем по
   * таймеру, а не по transitionend — при «уменьшить движение» события нет.
   */
  const [menuMounted, setMenuMounted] = useState(false)
  const [menuVisible, setMenuVisible] = useState(false)
  // Прокрутка мобильного меню включается после выезда: во время анимации
  // пункты на миг выходят за край контейнера, и мелькала полоса прокрутки.
  const [menuSettled, setMenuSettled] = useState(false)
  useEffect(() => {
    if (menuOpen) {
      setMenuMounted(true)
      let inner = 0
      let settle = 0
      const outer = requestAnimationFrame(() => {
        inner = requestAnimationFrame(() => {
          setMenuVisible(true)
          settle = window.setTimeout(() => setMenuSettled(true), 220)
        })
      })
      return () => {
        cancelAnimationFrame(outer)
        cancelAnimationFrame(inner)
        window.clearTimeout(settle)
      }
    }
    setMenuSettled(false)
    setMenuVisible(false)
    const id = window.setTimeout(() => setMenuMounted(false), 220)
    return () => window.clearTimeout(id)
  }, [menuOpen])
  const [logoutConfirmOpen, setLogoutConfirmOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)
  const mobileNavRef = useRef<HTMLElement>(null)

  /*
   * Высота нижнего меню в --cabinet-bottom-nav-h: на iPhone у меню отступ под
   * безопасную зону, и оно выше, чем в Telegram на Android. Фиксированные
   * плашки над меню (кнопка оплаты в CheckoutPage) встают ровно на его верх.
   */
  useEffect(() => {
    const nav = mobileNavRef.current
    if (!nav || typeof ResizeObserver === 'undefined') return
    const root = document.documentElement
    const sync = () => root.style.setProperty('--cabinet-bottom-nav-h', `${nav.offsetHeight}px`)
    sync()
    const ro = new ResizeObserver(sync)
    ro.observe(nav)
    return () => {
      ro.disconnect()
      root.style.removeProperty('--cabinet-bottom-nav-h')
    }
  }, [])

  useEffect(() => {
    if (!menuOpen) return
    if (!window.matchMedia('(min-width: 640px)').matches) return
    function onDoc(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false)
      }
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [menuOpen])

  useEffect(() => {
    setMenuOpen(false)
  }, [location.pathname, location.hash])

  /*
   * Авто-скрытие мобильной шапки убрано намеренно.
   * Шапка — sticky-элемент в потоке документа, и скрытие её строки через display:none
   * меняло высоту документа на ~56px. Браузер пересчитывал scrollY → приходил новый
   * scroll-эвент → обработчик возвращал шапку → высота росла обратно → снова scroll.
   * Получался самоподдерживающийся цикл на частоте rAF (мигание шапки в Telegram Mini App,
   * особенно на старте: requestFullscreen анимирует вьюпорт и меняет --tg-safe-area-inset-top,
   * а контент дашборда догружается асинхронно — оба дают «фальшивые» дельты скролла).
   * Возвращать фичу можно только layout-нейтрально: fixed-шапка + распорка и transform,
   * никакого display:none/height внутри потока.
   */

  useEffect(() => {
    if (!menuOpen) return
    if (window.matchMedia('(min-width: 640px)').matches) return
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prev
    }
  }, [menuOpen])

  useEffect(() => {
    if (!logoutConfirmOpen) return
    function onEsc(e: KeyboardEvent) {
      if (e.key === 'Escape') setLogoutConfirmOpen(false)
    }
    document.addEventListener('keydown', onEsc)
    return () => document.removeEventListener('keydown', onEsc)
  }, [logoutConfirmOpen])

  function requestLogoutConfirm() {
    setMenuOpen(false)
    setLogoutConfirmOpen(true)
  }

  function confirmLogout() {
    setLogoutConfirmOpen(false)
    logout()
  }

  return (
    <div className="cabinet-shell relative flex min-h-dvh flex-col">
      <div className="cabinet-shell-gradient" aria-hidden />
      <CabinetDecorLayer />
      {/* iPhone: фон шапки над краем экрана, под панелями браузера (index.css) */}
      <div className="cabinet-edge-top" aria-hidden />
      <header className="relative sticky top-0 z-50 isolate shrink-0 border-b border-border/80 bg-card/92 backdrop-blur-xl shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] dark:border-primary/12 dark:shadow-[0_8px_32px_-8px_rgba(0,0,0,0.55),inset_0_1px_0_rgba(255,255,255,0.06)] cabinet-app-header cabinet-edge-header">
        <div>
          <CabinetDecorHeader />
          <div className="max-w-5xl mx-auto flex items-center gap-2 px-2.5 py-2 sm:gap-4 sm:px-3 sm:py-2">
          <Link
            to="/dashboard"
            className="flex min-w-0 shrink-0 items-center rounded-md outline-none ring-offset-background transition-opacity hover:opacity-90 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
          >
            <Logo size="sm" />
          </Link>

          <nav
            className="hidden sm:flex flex-1 items-center justify-start gap-0.5 min-w-0 overflow-x-auto py-0.5"
            aria-label={t('nav.main')}
          >
            {navItems.map(({ to, icon: Icon, labelKey, activePrefixes }) => {
              const active = navItemActive(location.pathname, { to, icon: Icon, labelKey, activePrefixes })
              const isTariffs = to === '/tariffs'
              const label = t(labelKey)
              return (
                <Button
                  key={`${to}-${labelKey}`}
                  variant="ghost"
                  size="sm"
                  asChild
                  className={cn(
                    // cabinet-nav-link — хук для декор-тем: подсветка активного
                    // пункта и hover переопределяются темой, а не здесь.
                    'cabinet-nav-link group shrink-0 h-9 !gap-1 rounded-xl px-2 text-muted-foreground hover:text-foreground transition-all duration-200',
                    active && 'bg-secondary text-foreground shadow-sm',
                  )}
                >
                  <Link
                    to={to}
                    id={to === '/profile' ? 'cabinet-onboarding-profile-nav-desktop' : undefined}
                    aria-current={active ? 'page' : undefined}
                    aria-label={label}
                    className="flex items-center"
                  >
                    <NavIconWithDot showDot={to === '/support' && supportUnread > 0}>
                      <DecorNavIcon
                        theme={decorTheme}
                        to={to}
                        defaultIcon={Icon}
                        className={cn(
                          'size-[18px]',
                          active ? 'text-foreground' : 'text-muted-foreground',
                          isTariffs && !active && 'tariffs-shine-icon',
                        )}
                        strokeWidth={1.75}
                      />
                    </NavIconWithDot>
                    <span
                      className={cn(
                        'max-w-0 overflow-hidden whitespace-nowrap text-sm font-medium opacity-0 transition-all duration-200',
                        'group-hover:max-w-[9rem] group-hover:opacity-100',
                      )}
                    >
                      {label}
                    </span>
                  </Link>
                </Button>
              )
            })}
          </nav>

          <div className="flex items-center gap-0.5 sm:gap-1 ml-auto shrink-0">
            {/* Колокольчик — всем вошедшим, с подпиской или без. */}
            {hasSession && <NotificationBell />}
            {/* Язык и тема — только в профиле («Настройки»): тёмная тема стоит по
                умолчанию, язык берётся из браузера, менять их часто незачем. */}
            <div className="relative" ref={menuRef}>
              <Button
                variant="ghost"
                size="icon"
                type="button"
                aria-expanded={menuOpen}
                aria-haspopup="true"
                aria-label={t('nav.moreMenu')}
                title={t('nav.moreMenu')}
                className={cn('size-10', menuOpen && 'bg-secondary')}
                onClick={() => {
                  hapticImpactLight()
                  setMenuOpen((o) => !o)
                }}
              >
                {menuOpen ? <X className="size-[18px]" strokeWidth={1.75} /> : <Menu className="size-[18px]" strokeWidth={1.75} />}
              </Button>
              {menuMounted && (
                /* ПК: плоский список без карточек внутри карточки (SPEC §9). */
                <div
                  role="menu"
                  className={cn(
                    'absolute right-0 top-full z-[500] mt-1.5 hidden w-[300px] origin-top-right rounded-[18px] border border-border bg-card p-2 shadow-[0_24px_60px_rgba(0,0,0,0.55)] transition-[opacity,transform] duration-200 ease-out sm:block',
                    menuVisible ? 'translate-y-0 scale-100 opacity-100' : 'pointer-events-none -translate-y-1 scale-[0.97] opacity-0',
                  )}
                >
                  {menuGroups.map((g, gi) => (
                    <div key={g.key}>
                      {gi > 0 && <div className="mx-1 my-1.5 h-px bg-border" />}
                      <p className="px-3 pb-1 pt-2.5 text-[11px] font-bold uppercase tracking-[0.08em] text-muted-foreground">
                        {g.title}
                      </p>
                      {g.items.map((item) => {
                        const active = item.to ? overflowActive(location.pathname, item.to.split('#')[0]) : false
                        return (
                          <MenuEntry
                            key={item.key}
                            item={item}
                            className={cn(
                              'flex h-10 w-full items-center gap-3 rounded-[10px] px-3 text-[14.5px] text-foreground/90 transition-colors hover:bg-secondary hover:text-foreground',
                              active && 'bg-secondary text-primary',
                            )}
                          >
                            {item.decorTo ? (
                              <DecorNavIcon
                                theme={decorTheme}
                                to={item.decorTo}
                                defaultIcon={item.icon}
                                className={cn('size-[18px] shrink-0', active ? 'text-primary' : 'text-muted-foreground')}
                                strokeWidth={1.75}
                              />
                            ) : (
                              <item.icon className="size-[18px] shrink-0 text-muted-foreground" strokeWidth={1.75} />
                            )}
                            <span className="truncate">{item.label}</span>
                            {item.extra}
                          </MenuEntry>
                        )
                      })}
                    </div>
                  ))}
                  <div className="mx-1 my-1.5 h-px bg-border" />
                  {user?.is_admin ? (
                    <Link
                      to="/admin"
                      role="menuitem"
                      className={cn(
                        'flex h-10 w-full items-center gap-3 rounded-[10px] px-3 text-[14.5px] text-foreground/90 transition-colors hover:bg-secondary hover:text-foreground',
                        overflowActive(location.pathname, '/admin') && 'bg-secondary text-primary',
                      )}
                    >
                      <ShieldCheck className="size-[18px] shrink-0 text-muted-foreground" strokeWidth={1.75} />
                      {t('admin.dashboard.title')}
                    </Link>
                  ) : null}
                  <button
                    type="button"
                    role="menuitem"
                    className="flex h-10 w-full items-center gap-3 rounded-[10px] px-3 text-left text-[14.5px] text-destructive transition-colors hover:bg-destructive/10"
                    onClick={requestLogoutConfirm}
                  >
                    <LogOut className="size-[18px] shrink-0" strokeWidth={1.75} />
                    {t('nav.logout')}
                  </button>
                </div>
              )}
            </div>
          </div>
          </div>
        </div>
      </header>

      {menuMounted && (
        /* Телефон: экран меню — группы списками, как в профиле (SPEC §9). */
        <div
          className={cn(
            'fixed inset-x-0 bottom-0 top-[calc(56px+var(--cabinet-tg-safe-top))] z-40 flex min-h-0 flex-col bg-background transition-opacity duration-200 ease-out sm:hidden',
            menuVisible ? 'opacity-100' : 'pointer-events-none opacity-0',
          )}
        >
          <div className={cn('min-h-0 flex-1 overscroll-contain bg-background', menuSettled ? 'overflow-y-auto' : 'overflow-hidden')}>
          <div
            className={cn(
              'pb-[calc(6.5rem+max(env(safe-area-inset-bottom,0px),var(--cabinet-tg-safe-bottom)))] transition-transform duration-200 ease-out',
              menuVisible ? 'translate-y-0' : '-translate-y-2',
            )}
          >
            {menuGroups.map((g) => (
              <div key={g.key}>
                <p className="px-5 pb-2 pt-[22px] text-xs font-bold uppercase tracking-[0.08em] text-muted-foreground">{g.title}</p>
                <div className="mx-4 overflow-hidden rounded-[18px] border border-border bg-card/55">
                  {g.items.map((item, i) => (
                    <MenuEntry
                      key={item.key}
                      item={item}
                      className={cn(
                        'flex h-12 items-center gap-3 px-3.5 text-[15px] text-foreground/90 transition-colors active:bg-secondary',
                        i > 0 && 'border-t border-border',
                      )}
                    >
                      {item.decorTo ? (
                        <DecorNavIcon
                          theme={decorTheme}
                          to={item.decorTo}
                          defaultIcon={item.icon}
                          className="size-[18px] shrink-0 text-muted-foreground"
                          strokeWidth={1.75}
                        />
                      ) : (
                        <item.icon className="size-[18px] shrink-0 text-muted-foreground" strokeWidth={1.75} />
                      )}
                      <span className="min-w-0 flex-1 truncate">{item.label}</span>
                      {item.extra ? <span className="flex shrink-0 [&>span]:ml-0">{item.extra}</span> : null}
                      <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
                    </MenuEntry>
                  ))}
                </div>
              </div>
            ))}
            <div className="mx-4 mt-[22px] overflow-hidden rounded-[18px] border border-border bg-card/55">
              {user?.is_admin ? (
                <Link
                  to="/admin"
                  role="menuitem"
                  className="flex h-12 items-center gap-3 border-b border-border px-3.5 text-[15px] text-foreground/90 active:bg-secondary"
                >
                  <ShieldCheck className="size-[18px] shrink-0 text-muted-foreground" strokeWidth={1.75} />
                  <span className="min-w-0 flex-1 truncate">{t('admin.dashboard.title')}</span>
                  <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
                </Link>
              ) : null}
              <button
                type="button"
                role="menuitem"
                className="flex h-12 w-full items-center gap-3 px-3.5 text-left text-[15px] text-destructive active:bg-destructive/10"
                onClick={requestLogoutConfirm}
              >
                <LogOut className="size-[18px] shrink-0" strokeWidth={1.75} />
                {t('nav.logout')}
              </button>
            </div>
          </div>
          </div>
        </div>
      )}

      <ConfirmModal
        open={logoutConfirmOpen}
        tone="danger"
        icon={LogOut}
        title={t('nav.logoutConfirmTitle')}
        confirmLabel={t('nav.logoutConfirmYes')}
        onClose={() => setLogoutConfirmOpen(false)}
        onConfirm={confirmLogout}
      />

      <div className="cabinet-shell-content relative z-10 min-h-0 flex-1 overflow-x-hidden touch-pan-y">
        {/*
          Без animate-fade-in: появление контента теперь ведёт PageReveal внутри страниц
          (components/PageReveal.tsx). Общий фейд поверх «лесенки» давал двойное проявление.
        */}
        <div className="mx-auto w-full max-w-5xl px-4 py-6 sm:py-8 pb-[max(1rem,calc(5.75rem+max(env(safe-area-inset-bottom,0px),var(--cabinet-tg-safe-bottom))))] sm:pb-8 [&>*]:mx-auto [&>*]:w-full">
          {children}
        </div>
      </div>

      <CabinetOnboarding />

      <nav
        ref={mobileNavRef}
        className="cabinet-edge-nav fixed inset-x-0 bottom-0 z-50 sm:hidden px-2 pb-[max(0.5rem,var(--cabinet-tg-safe-bottom))] pointer-events-none"
        aria-label={t('nav.mobile')}
      >
        <div className="cabinet-edge-nav-bar pointer-events-auto flex items-stretch justify-around gap-0 overflow-x-auto rounded-2xl border border-border bg-card/95 px-1 py-1.5 backdrop-blur-md shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)]">
          {mobileBottomNavItems.map(({ to, icon: Icon, labelKey, activePrefixes }) => {
            const active = navItemActive(location.pathname, { to, icon: Icon, labelKey, activePrefixes })
            const label = t(labelKey)
            const isTariffs = to === '/tariffs'
            return (
              <Link
                key={`${to}-${labelKey}-mob`}
                to={to}
                id={to === '/profile' ? 'cabinet-onboarding-profile-nav-mobile' : undefined}
                aria-label={label}
                aria-current={active ? 'page' : undefined}
                onClick={() => hapticImpactLight()}
                className={cn(
                  'flex min-w-0 flex-1 max-w-[4.5rem] flex-col items-center justify-center gap-0.5 rounded-xl px-0.5 py-1 text-muted-foreground transition-colors',
                  active && cn('bg-secondary', navAccentActiveClass),
                )}
              >
                <NavIconWithDot showDot={to === '/support' && supportUnread > 0}>
                  <DecorNavIcon
                    theme={decorTheme}
                    to={to}
                    defaultIcon={Icon}
                    className={cn(
                      'size-[20px] shrink-0',
                      !active && 'text-muted-foreground',
                      isTariffs && !active && 'tariffs-shine-icon',
                      active && navAccentActiveClass,
                    )}
                    strokeWidth={1.75}
                  />
                </NavIconWithDot>
                <span className="w-full text-center text-[10px] leading-tight font-medium truncate">{label}</span>
              </Link>
            )
          })}
        </div>
      </nav>
    </div>
  )
}

import { type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ChevronLeft, Menu, ShieldCheck } from 'lucide-react'

import { Logo } from '@/components/Logo'
import { ThemeToggle } from '@/components/ThemeToggle'
import { LangToggle } from '@/components/LangToggle'
import { cn } from '@/lib/utils'
import { useAdminShell } from './AdminShellContext'
import { CabinetDecorLayer } from '@/features/decor/CabinetDecorLayer'
import { CabinetDecorHeader } from '@/features/decor/CabinetDecorHeader'

interface AdminChromeProps {
  children: ReactNode
  /** Ниже lg страница рисует свою верхнюю строку — общий ряд chrome прячется. */
  hideMobileHeader?: boolean
}

/**
 * Минимальный chrome для `/admin/*`: без user-cabinet nav и mobile bottom bar.
 */
export function AdminChrome({ children, hideMobileHeader }: AdminChromeProps) {
  const { t } = useTranslation()
  const { toggleMobileNav, mobileNavExpanded, mobileHeaderVisible } = useAdminShell()

  return (
    <div className="relative flex min-h-dvh flex-col">
      <div className="cabinet-shell-gradient" aria-hidden />
      <CabinetDecorLayer />
      {/* iPhone: фон шапки над краем экрана, под панелями браузера (index.css) */}
      <div className="cabinet-edge-top" aria-hidden />
      <header
        className={cn(
          'relative sticky top-0 z-50 shrink-0 border-b border-border/80 bg-card/92 backdrop-blur-xl shadow-sm cabinet-app-header cabinet-edge-header',
          'dark:border-primary/12 dark:shadow-[0_8px_32px_-8px_rgba(0,0,0,0.55),inset_0_1px_0_rgba(255,255,255,0.06)]',
          // Safe-area top остаётся; прячется только ряд chrome.
          !mobileHeaderVisible && 'max-md:border-b-0 max-md:shadow-none',
          hideMobileHeader && 'max-lg:border-b-0 max-lg:shadow-none',
        )}
      >
        <div className={cn(!mobileHeaderVisible && 'max-md:hidden', hideMobileHeader && 'max-lg:hidden')}>
          <CabinetDecorHeader />
          <div className="mx-auto flex max-w-7xl items-center gap-2 px-3 py-2 sm:gap-3 sm:px-4">
          <button
            type="button"
            data-admin-nav-toggle
            onClick={toggleMobileNav}
            aria-expanded={mobileNavExpanded}
            className="inline-flex shrink-0 items-center gap-2 rounded-lg border border-border/60 bg-card/80 px-2.5 py-2 text-sm font-medium lg:hidden"
            aria-label={t('admin.nav.menu')}
          >
            <Menu className="size-4" />
            <span className="sr-only sm:not-sr-only">{t('admin.nav.menu')}</span>
          </button>

          <Link
            to="/admin"
            className="hidden min-w-0 shrink-0 items-center rounded-md outline-none ring-offset-background transition-opacity hover:opacity-90 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 lg:flex"
          >
            <Logo size="sm" />
          </Link>

          <div className="flex min-w-0 items-center gap-2 border-l border-border/60 pl-2 sm:pl-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/15 dark:bg-primary/20">
              <ShieldCheck className="size-4 text-primary" />
            </div>
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold leading-tight">{t('admin.dashboard.title')}</p>
            </div>
          </div>

          <div className="ml-auto flex shrink-0 items-center gap-0.5 sm:gap-1">
            <Link
              to="/dashboard"
              className="hidden items-center gap-1 rounded-lg px-2 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground sm:inline-flex"
            >
              <ChevronLeft className="size-4" />
              {t('admin.backToCabinet')}
            </Link>
            <LangToggle className="h-9 px-3" />
            <ThemeToggle className="size-9 [&_svg]:size-[18px]" />
          </div>
          </div>
        </div>
      </header>

      <div className="relative z-10 min-h-0 flex-1">
        {children}
      </div>
    </div>
  )
}

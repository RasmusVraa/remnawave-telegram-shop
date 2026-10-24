import { useEffect, useState, type ReactNode } from 'react'
import {
  Check,
  ChevronRight,
  Copy,
  FileText,
  Home,
  MessageCircle,
  MonitorSmartphone,
  Newspaper,
  Shield,
  Sparkles,
  Star,
  Ticket,
  User,
  Users,
  Zap,
  type LucideIcon,
} from 'lucide-react'

import { applyTheme } from '@/hooks/useTheme'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { DECOR_THEME_IDS, type DecorThemeId } from '@/features/decor/decorThemes'
import { decorThemeOptionLabelStyle } from '@/features/decor/decorThemeAdmin'
import { CabinetDecorHeader } from '@/features/decor/CabinetDecorHeader'
import { CabinetDecorLayer } from '@/features/decor/CabinetDecorLayer'
import { useDecorCardSpotlight } from '@/features/decor/useDecorCardSpotlight'

/**
 * Dev-превью декор-тем: /cabinet/dev/theme.
 *
 * Маршрут регистрируется только при import.meta.env.DEV (см. App.tsx) — нужен,
 * чтобы оценить тему без запущенного бота и живой сессии.
 *
 * Блоки — заглушки, но собраны из настоящих примитивов (Card, Button,
 * .subscription-feature-card, .connect-device-cta, .dashboard-quick-link),
 * поэтому все эффекты темы работают ровно так, как на реальных страницах.
 *
 * data-cabinet-decor ставится здесь напрямую: обычно это делает
 * CabinetDecorThemeSync из ответа /auth/bootstrap, которого в dev нет.
 */

type Page = 'dashboard' | 'subscription'
type NavVariant = 'pill' | 'underline'

function initialDecor(): DecorThemeId {
  if (typeof window === 'undefined') return 'nebula'
  const raw = new URLSearchParams(window.location.search).get('decor')
  return (DECOR_THEME_IDS as readonly string[]).includes(raw ?? '') ? (raw as DecorThemeId) : 'nebula'
}

export default function ThemePreviewPage() {
  const [theme, setTheme] = useState<'dark' | 'light'>('dark')
  const [decor, setDecor] = useState<DecorThemeId>(initialDecor)
  const [page, setPage] = useState<Page>('dashboard')
  const [navVariant, setNavVariant] = useState<NavVariant>('underline')

  // Подсветка карточек под курсором — тот же хук, что в проде.
  useDecorCardSpotlight()

  useEffect(() => {
    applyTheme(theme)
  }, [theme])

  useEffect(() => {
    const root = document.documentElement
    if (decor === 'off') {
      delete root.dataset.cabinetDecor
    } else {
      root.dataset.cabinetDecor = decor
    }
    return () => {
      delete root.dataset.cabinetDecor
    }
  }, [decor])

  // Вариант подсветки навигации: в проде атрибута нет и работает подчёркивание.
  useEffect(() => {
    const root = document.documentElement
    if (navVariant === 'underline') {
      delete root.dataset.nebulaNav
    } else {
      root.dataset.nebulaNav = navVariant
    }
    return () => {
      delete root.dataset.nebulaNav
    }
  }, [navVariant])

  return (
    <div className="cabinet-shell relative min-h-dvh">
      <div className="cabinet-shell-gradient" aria-hidden />
      <CabinetDecorLayer theme={decor} />

      <HeaderStub page={page} decor={decor} />

      <div className="relative z-10 mx-auto max-w-3xl px-4 py-6 sm:px-6">
        <Toolbar
          theme={theme}
          onTheme={setTheme}
          decor={decor}
          onDecor={setDecor}
          page={page}
          onPage={setPage}
          navVariant={navVariant}
          onNavVariant={setNavVariant}
        />

        {/* key на теме — перемонтирует блоки, чтобы заново отыграла лесенка появления. */}
        <div key={`${decor}-${page}`} className="mt-6">
          {page === 'dashboard' ? <DashboardStub /> : <SubscriptionStub />}
        </div>
      </div>
    </div>
  )
}

function Toolbar({
  theme,
  onTheme,
  decor,
  onDecor,
  page,
  onPage,
  navVariant,
  onNavVariant,
}: {
  theme: 'dark' | 'light'
  onTheme: (v: 'dark' | 'light') => void
  decor: DecorThemeId
  onDecor: (v: DecorThemeId) => void
  page: Page
  onPage: (v: Page) => void
  navVariant: NavVariant
  onNavVariant: (v: NavVariant) => void
}) {
  return (
    <div className="rounded-[var(--radius)] border border-border/70 bg-card/70 p-3 backdrop-blur-md">
      <p className="px-1 pb-2 text-[0.65rem] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
        Превью декор-темы · только в dev
      </p>

      <div className="flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-2">
          <span className="text-xs font-medium text-muted-foreground">Тема</span>
          <select
            value={decor}
            onChange={(e) => onDecor(e.target.value as DecorThemeId)}
            className="admin-input h-8 rounded-lg px-2 text-xs"
            style={decorThemeOptionLabelStyle(decor)}
          >
            {DECOR_THEME_IDS.map((id) => (
              <option key={id} value={id}>
                {id}
              </option>
            ))}
          </select>
        </label>

        <Segmented
          items={[
            { id: 'dark', label: 'Тёмная' },
            { id: 'light', label: 'Светлая' },
          ]}
          value={theme}
          onChange={(v) => onTheme(v as 'dark' | 'light')}
        />

        <Segmented
          items={[
            { id: 'dashboard', label: 'Главная' },
            { id: 'subscription', label: 'Подписка' },
          ]}
          value={page}
          onChange={(v) => onPage(v as Page)}
        />
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-border/60 pt-3">
        <span className="text-xs font-medium text-muted-foreground">Навигация в шапке</span>
        <Segmented
          items={[
            { id: 'underline', label: 'Подчёркивание' },
            { id: 'pill', label: 'Акцентная пилюля' },
          ]}
          value={navVariant}
          onChange={(v) => onNavVariant(v as NavVariant)}
        />
      </div>
    </div>
  )
}

/* ── Заглушка шапки: та же разметка, что в AppLayout ─────────────────────── */

const NAV_STUB: Array<{ id: string; icon: LucideIcon; label: string }> = [
  { id: 'dashboard', icon: Home, label: 'Главная' },
  { id: 'subscription', icon: Sparkles, label: 'Подписка' },
  { id: 'tariffs', icon: Zap, label: 'Тарифы' },
  { id: 'support', icon: MessageCircle, label: 'Поддержка' },
  { id: 'profile', icon: User, label: 'Профиль' },
  { id: 'admin', icon: Shield, label: 'Админка' },
]

/**
 * Копия десктопной навигации: те же Button variant="ghost" + cabinet-nav-link
 * + aria-current, поэтому CSS темы применяется ровно как в настоящей шапке.
 * Подпись раскрывается на hover — как в кабинете.
 */
function HeaderStub({ page, decor }: { page: Page; decor: DecorThemeId }) {
  return (
    <header className="cabinet-app-header relative z-20 border-b border-border/80 bg-card/92 px-4 backdrop-blur-xl sm:px-6">
      <CabinetDecorHeader theme={decor} />
      <div className="mx-auto flex h-14 max-w-5xl items-center gap-3">
        <span className="flex shrink-0 items-center gap-2">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Shield size={16} />
          </span>
          <span className="font-heading text-base font-bold tracking-tight">Meows VPN</span>
        </span>

        <nav className="flex min-w-0 flex-1 items-center gap-0.5 overflow-x-auto py-0.5">
          {NAV_STUB.map(({ id, icon: Icon, label }) => {
            const active = id === page
            return (
              <Button
                key={id}
                variant="ghost"
                size="sm"
                asChild
                className={cn(
                  'cabinet-nav-link group h-9 shrink-0 !gap-1 rounded-xl px-2 text-muted-foreground transition-all duration-200 hover:text-foreground',
                  active && 'bg-secondary text-foreground shadow-sm',
                )}
              >
                <a
                  href={`#${id}`}
                  aria-current={active ? 'page' : undefined}
                  aria-label={label}
                  className="flex items-center"
                >
                  <Icon size={18} strokeWidth={1.75} />
                  <span className="max-w-0 overflow-hidden whitespace-nowrap text-sm font-medium opacity-0 transition-all duration-200 group-hover:max-w-[9rem] group-hover:opacity-100">
                    {label}
                  </span>
                </a>
              </Button>
            )
          })}
        </nav>
      </div>
    </header>
  )
}

function Segmented({
  items,
  value,
  onChange,
}: {
  items: Array<{ id: string; label: string }>
  value: string
  onChange: (v: string) => void
}) {
  return (
    <div className="inline-flex gap-1 rounded-lg border border-border/70 p-0.5">
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          onClick={() => onChange(item.id)}
          aria-pressed={value === item.id}
          className={cn(
            'rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
            value === item.id
              ? 'bg-primary/15 text-primary'
              : 'text-muted-foreground hover:text-foreground',
          )}
        >
          {item.label}
        </button>
      ))}
    </div>
  )
}

/* ── Заглушка главной ──────────────────────────────────────────────────────── */

function DashboardStub() {
  return (
    <div className="space-y-4">
      <Card className="subscription-feature-card">
        <CardContent className="space-y-5 px-5 py-5 sm:px-6 sm:py-6">
          <div className="flex items-center justify-between gap-3">
            <div>
              <p className="text-sm text-muted-foreground">Подписка</p>
              <p className="mt-0.5 font-heading text-xl font-bold">Премиум</p>
            </div>
            <span className="rounded-full border border-primary/35 bg-primary/15 px-3 py-1 text-xs font-semibold text-primary">
              Активна
            </span>
          </div>

          <TrafficStub />

          <a href="#connect" className="connect-device-cta connect-device-cta--highlight group block">
            <div className="connect-device-cta-inner flex items-center gap-3 px-4 py-3 text-card-foreground">
              <span className="connect-device-cta-icon inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
                <MonitorSmartphone size={16} />
              </span>
              <div className="min-w-0">
                <p className="font-medium">Подключить устройство</p>
                <p className="text-xs text-muted-foreground">Доступно 5 устройств</p>
              </div>
              <ChevronRight size={18} className="connect-device-cta-chevron ml-auto shrink-0" />
            </div>
          </a>

          <div className="rounded-[var(--radius)] border border-border bg-muted/40 px-4 py-3">
            <p className="text-sm font-medium">Действует до 14 марта 2027</p>
            <p className="text-xs text-muted-foreground">осталось 203 дня</p>
          </div>
        </CardContent>
      </Card>

      <Card className="subscription-feature-card">
        <CardContent className="space-y-4 px-6 py-7">
          <div>
            <p className="font-heading text-lg font-bold">Баланс</p>
            <p className="mt-1 text-sm text-muted-foreground">
              Пополните счёт, чтобы продлить подписку в один клик.
            </p>
          </div>
          <p className="font-heading text-3xl font-extrabold tabular-nums">1 240 ₽</p>
          <Button className="w-full">Пополнить</Button>
        </CardContent>
      </Card>

      <div className="grid grid-cols-2 gap-3">
        <QuickLink icon={Zap} label="Тарифы" />
        <QuickLink icon={Users} label="Рефералы" />
        <QuickLink icon={Ticket} label="Промокоды" />
        <QuickLink icon={FileText} label="Информация" />
        <QuickLink icon={Newspaper} label="Новости" />
        <QuickLink icon={Star} label="Отзывы" />
      </div>
    </div>
  )
}

function QuickLink({ icon: Icon, label }: { icon: LucideIcon; label: string }) {
  return (
    <a
      href="#quick"
      className="group subscription-feature-card dashboard-quick-link flex h-full min-h-[3.25rem] items-center justify-between gap-2 px-3 py-4 sm:px-4"
    >
      <span className="flex items-center gap-2.5">
        <span className="inline-flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
          <Icon size={16} />
        </span>
        <span className="text-sm font-medium">{label}</span>
      </span>
      <ChevronRight size={16} className="text-muted-foreground transition-transform group-hover:translate-x-0.5" />
    </a>
  )
}

/* ── Заглушка страницы подписки ────────────────────────────────────────────── */

function SubscriptionStub() {
  const [copied, setCopied] = useState(false)

  return (
    <div className="space-y-4">
      <Card className="subscription-feature-card">
        <CardContent className="space-y-5 px-5 py-5 sm:px-6 sm:py-6">
          <div className="flex items-center justify-between gap-3">
            <p className="font-heading text-lg font-bold">Моя подписка</p>
            <span className="rounded-full border border-primary/35 bg-primary/15 px-3 py-1 text-xs font-semibold text-primary">
              Премиум
            </span>
          </div>

          <TrafficStub />

          <a href="#connect" className="connect-device-cta connect-device-cta--highlight group block">
            <div className="connect-device-cta-inner flex items-center gap-3 px-4 py-3 text-card-foreground">
              <span className="connect-device-cta-icon inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
                <MonitorSmartphone size={16} />
              </span>
              <div className="min-w-0">
                <p className="font-medium">Подключить устройство</p>
                <p className="text-xs text-muted-foreground">Доступно 5 устройств</p>
              </div>
              <ChevronRight size={18} className="connect-device-cta-chevron ml-auto shrink-0" />
            </div>
          </a>
        </CardContent>
      </Card>

      <Card className="subscription-feature-card">
        <CardContent className="space-y-3 px-5 py-5 sm:px-6">
          <p className="text-sm font-medium">Ссылка подписки</p>
          <div className="flex items-center gap-2">
            <code className="min-w-0 flex-1 truncate rounded-lg border border-border bg-muted/50 px-3 py-2 font-mono text-xs text-muted-foreground">
              https://cabinet.example.com/sub/8f3c1a…
            </code>
            <Button
              variant="outline"
              size="icon"
              onClick={() => setCopied((v) => !v)}
              aria-label="Скопировать"
            >
              {copied ? <Check size={16} /> : <Copy size={16} />}
            </Button>
          </div>
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <DeviceRow name="iPhone 15 Pro" platform="iOS · 2 дня назад" />
        <DeviceRow name="MacBook Air" platform="macOS · сегодня" />
        <DeviceRow name="Xiaomi TV" platform="Android TV · неделю назад" />
        <DeviceRow name="Рабочий ПК" platform="Windows · сегодня" />
      </div>

      <Card className="subscription-feature-card">
        <CardContent className="space-y-4 px-6 py-6">
          <p className="font-heading text-lg font-bold">Частые вопросы</p>
          <FaqStub question="Как подключить VPN?">
            Установите приложение, откройте раздел подключения и импортируйте подписку — дальше
            всё работает само.
          </FaqStub>
          <FaqStub question="На скольких устройствах можно пользоваться?">
            Количество зависит от тарифа. Лимит можно расширить прямо в кабинете.
          </FaqStub>
          <Button className="w-full">Продлить подписку</Button>
        </CardContent>
      </Card>
    </div>
  )
}

function DeviceRow({ name, platform }: { name: string; platform: string }) {
  return (
    <div className="cabinet-elevated-card flex items-center gap-3 px-4 py-3">
      <span className="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
        <MonitorSmartphone size={16} />
      </span>
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">{name}</p>
        <p className="truncate text-xs text-muted-foreground">{platform}</p>
      </div>
    </div>
  )
}

/** Нативный details — тот же приём, что на странице «Информация». */
function FaqStub({ question, children }: { question: string; children: ReactNode }) {
  return (
    <details className="group rounded-xl border border-border/70 bg-card/70 p-4 open:border-primary/35">
      <summary className="flex cursor-pointer items-center justify-between gap-3 text-sm font-medium">
        {question}
        <span className="text-muted-foreground transition-transform group-open:rotate-180">⌄</span>
      </summary>
      <p className="pt-3 text-sm leading-relaxed text-muted-foreground">{children}</p>
    </details>
  )
}

function TrafficStub() {
  return (
    <div>
      <div className="flex items-baseline justify-between text-sm">
        <span className="text-muted-foreground">Трафик</span>
        <span className="font-semibold tabular-nums">64 из 100 ГБ</span>
      </div>
      <div className="mt-2 h-2.5 overflow-hidden rounded-full bg-muted">
        <div className="h-full w-[64%] rounded-full bg-primary" />
      </div>
    </div>
  )
}

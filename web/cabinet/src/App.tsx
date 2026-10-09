import { lazy, Suspense, useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import { useAuthStore } from '@/store/auth'
import { ProtectedRoute } from '@/components/ProtectedRoute'
import { RouteErrorBoundary } from '@/components/RouteErrorBoundary'
import { AdminRoute } from '@/components/AdminRoute'
import { BrandFavicon } from '@/components/BrandFavicon'
import { ThemePolicyProvider } from '@/components/ThemePolicyProvider'
import { ToastProvider } from '@/components/ui/toast'
import { CabinetDecorThemeSync } from '@/features/decor/CabinetDecorThemeSync'
import { isTelegramMiniAppSession } from '@/lib/telegram-web-app-loader'

// Главная — единственная страница, загружаемая сразу: мини-апп открывается на ней,
// и отдельный запрос за чанком добавил бы round-trip к первому экрану.
import DashboardPage from '@/features/dashboard/DashboardPage'

/*
 * Остальные маршруты — через lazy.
 *
 * Раньше всё импортировалось статически и уезжало в один чанк на 2 МБ. Из него
 * 39% исходников — админка, плюс recharts, который используется только в
 * features/admin/stats (10 файлов). То есть обычный пользователь мини-аппа
 * скачивал целиком админпанель с графической библиотекой, чтобы посмотреть
 * остаток трафика. Теперь админка, лендинг, dev-превью и редкие экраны едут
 * отдельными чанками и подгружаются по требованию.
 *
 * default-экспорт страниц сохранён, поэтому lazy() принимает импорт как есть.
 */

// Auth pages (9a)
const LoginPage = lazy(() => import('@/features/auth/LoginPage'))
const RegisterPage = lazy(() => import('@/features/auth/RegisterPage'))
const VerifyEmailPage = lazy(() => import('@/features/auth/VerifyEmailPage'))
const ForgotPasswordPage = lazy(() => import('@/features/auth/ForgotPasswordPage'))
const ResetPasswordPage = lazy(() => import('@/features/auth/ResetPasswordPage'))

// Публичный лендинг (9a): отдельный маршрут, корень SPA по-прежнему ведёт в кабинет.
const LandingPage = lazy(() => import('@/features/landing/LandingPage'))
const StatusPage = lazy(() => import('@/features/landing/StatusPage'))

// Dev-превью компонентов. Маршруты ниже регистрируются только при import.meta.env.DEV;
// через lazy модули не попадают и в прод-чанки.
const ConnectCtaPreviewPage = lazy(() => import('@/features/dev/ConnectCtaPreviewPage'))
const ThemePreviewPage = lazy(() => import('@/features/dev/ThemePreviewPage'))
const RedesignPreviewPage = lazy(() => import('@/features/dev/RedesignPreviewPage'))

// Protected pages (9b)
const SubscriptionPage = lazy(() => import('@/features/subscription/SubscriptionPage'))
const TariffsPage = lazy(() => import('@/features/tariffs/TariffsPage'))
const CheckoutPage = lazy(() => import('@/features/checkout/CheckoutPage'))
const PaymentStatusPage = lazy(() => import('@/features/checkout/PaymentStatusPage'))
const SettingsPage = lazy(() => import('@/features/settings/SettingsPage'))
const LinkEmailPage = lazy(() => import('@/features/settings/LinkEmailPage'))
const MergePreviewPage = lazy(() => import('@/features/settings/MergePreviewPage'))
const ProfilePage = lazy(() => import('@/features/profile/ProfilePage'))
const ConnectionsPage = lazy(() => import('@/features/connections/ConnectionsPage'))
const DeepLinkRedirectPage = lazy(() => import('@/features/connections/DeepLinkRedirectPage'))
const ReferralProgramPage = lazy(() => import('@/features/referral/ReferralProgramPage'))
const PartnerProgramPage = lazy(() => import('@/features/partner/PartnerProgramPage'))
const FortunePage = lazy(() => import('@/features/fortune/FortunePage'))
const LoyaltyProgramPage = lazy(() => import('@/features/loyalty/LoyaltyProgramPage'))
const PromoCodesPage = lazy(() => import('@/features/promocodes/PromoCodesPage'))
const SupportPage = lazy(() => import('@/features/support/SupportPage'))
const InfoPage = lazy(() => import('@/features/info/InfoPage'))

// Админка: самый крупный кусок, обычному пользователю не нужен никогда.
const AdminDashboardPage = lazy(() => import('@/features/admin/pages/AdminDashboardPage'))
const AdminStatsPage = lazy(() => import('@/features/admin/pages/AdminStatsPage'))
const AdminUsersPage = lazy(() => import('@/features/admin/pages/AdminUsersPage'))
const AdminUserDetailPage = lazy(() => import('@/features/admin/pages/AdminUserDetailPage'))
const AdminPaymentsPage = lazy(() => import('@/features/admin/pages/AdminPaymentsPage'))
const AdminPromosPage = lazy(() => import('@/features/admin/pages/AdminPromosPage'))
const AdminTariffsPage = lazy(() => import('@/features/admin/pages/AdminTariffsPage'))
const AdminLoyaltyPage = lazy(() => import('@/features/admin/pages/AdminLoyaltyPage'))
const AdminPartnersPage = lazy(() => import('@/features/admin/pages/AdminPartnersPage'))
const AdminBroadcastPage = lazy(() => import('@/features/admin/pages/AdminBroadcastPage'))
const AdminInfraPage = lazy(() => import('@/features/admin/pages/AdminInfraPage'))
const AdminStatusPage = lazy(() => import('@/features/admin/pages/AdminStatusPage'))
const AdminSyncPage = lazy(() => import('@/features/admin/pages/AdminSyncPage'))
const AdminSettingsPage = lazy(() => import('@/features/admin/pages/AdminSettingsPage'))

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 30_000,
    },
  },
})

/**
 * Кабинет живёт под /cabinet. Лендинг — на корне домена (/) и на старом /landing.
 * Basename выбираем по фактическому пути, иначе router с '/cabinet' не сматчит корень.
 */
function resolveBasename(): string {
  if (typeof window === 'undefined') return '/cabinet'
  return window.location.pathname.startsWith('/cabinet') ? '/cabinet' : ''
}

const PUBLIC_AUTH_PATHS = new Set([
  '/login',
  '/register',
  '/verify-email',
  '/password/forgot',
  '/password/reset',
])

/** Показывать оболочку до init auth (публичные страницы + deeplink и лендинг без сессии). */
const PUBLIC_SHELL_PATHS = new Set([
  ...PUBLIC_AUTH_PATHS,
  '/deeplink',
  '/landing',
  // Страница приглашения: её открывает получатель ссылки, у которого сессии
  // нет и быть не может — ждать инициализации auth незачем.
  '/connect',
  // Dev-превью: без этого страница ждала бы инициализацию auth. В прод не попадает.
  ...(import.meta.env.DEV ? ['/dev/connect-cta', '/dev/theme', '/dev/redesign'] : []),
])

function normalizePath(pathname: string): string {
  const p = (pathname || '/').replace(/\/+$/, '')
  return p === '' ? '/' : p
}

function isPublicShellPath(pathname: string): boolean {
  if (normalizePath(pathname) === '/status') return true
  if (PUBLIC_SHELL_PATHS.has(normalizePath(pathname))) return true
  // Корень домена — лендинг. Внутри /cabinet «/» это вход в кабинет, его так не помечаем.
  if (typeof window !== 'undefined' && window.location.pathname.startsWith('/cabinet')) return false
  const path = normalizePath(pathname)
  return path === '/' || path === '/landing'
}

function AppRoutes() {
  const location = useLocation()
  const { initialized, initialize } = useAuthStore()
  const showAuthShellEarly = isPublicShellPath(location.pathname)

  useEffect(() => {
    window.scrollTo({ top: 0, left: 0, behavior: 'auto' })
  }, [location.pathname, location.search])

  useEffect(() => {
    void initialize()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  if (!initialized && !showAuthShellEarly) {
    return <FullscreenSpinner />
  }

  return (
    <RouteErrorBoundary>
    <Suspense fallback={<FullscreenSpinner />}>
    <Routes>
      {/* ── Public auth routes ─────────────────────────── */}
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/verify-email" element={<VerifyEmailPage />} />
      <Route path="/password/forgot" element={<ForgotPasswordPage />} />
      <Route path="/password/reset" element={<ResetPasswordPage />} />

      {/* Публичная страница редиректа на custom scheme — открывается из мини-приложения во внешнем браузере (без сессии). */}
      <Route path="/deeplink" element={<DeepLinkRedirectPage />} />

      {/*
        Приглашение «подключить ещё устройство»: тот же гайд, что и /connections,
        но подписка берётся из токена в #t=..., а не из сессии. Отдельный роут, а
        не флаг на /connections, чтобы владелец случайно не открыл гостевой режим
        и чтобы ProtectedRoute на основном пути остался нетронутым.
      */}
      <Route path="/connect" element={<ConnectionsPage />} />

      {/* Витрина на корне домена. В мини-приложении сразу уходим в кабинет. */}
      <Route path="/landing" element={<PublicLanding />} />
      <Route path="/status" element={<StatusPage />} />

      {/* Только dev: превью акцентной кнопки «Подключить устройство». */}
      {import.meta.env.DEV && (
        <Route path="/dev/connect-cta" element={<ConnectCtaPreviewPage />} />
      )}

      {/* Только dev: превью декор-тем на заглушках главной и подписки. */}
      {import.meta.env.DEV && <Route path="/dev/theme" element={<ThemePreviewPage />} />}

      {/* Только dev: варианты редизайна главной и подписки на моках. */}
      {import.meta.env.DEV && <Route path="/dev/redesign" element={<RedesignPreviewPage />} />}

      {/* ── Protected routes ───────────────────────────── */}
      <Route
        path="/dashboard"
        element={
          <ProtectedRoute requireVerified>
            <DashboardPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/subscription"
        element={
          <ProtectedRoute requireVerified>
            <SubscriptionPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/tariffs"
        element={
          <ProtectedRoute requireVerified>
            <TariffsPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/checkout"
        element={
          <ProtectedRoute requireVerified>
            <CheckoutPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/payment/status/:id"
        element={
          <ProtectedRoute>
            <PaymentStatusPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/payment/status"
        element={
          <ProtectedRoute>
            <PaymentStatusPage />
          </ProtectedRoute>
        }
      />

      <Route path="/settings" element={<Navigate to="/profile" replace />} />

      <Route
        path="/profile"
        element={
          <ProtectedRoute requireVerified>
            <ProfilePage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/accounts"
        element={
          <ProtectedRoute>
            <SettingsPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/accounts/email"
        element={
          <ProtectedRoute>
            <LinkEmailPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/connections"
        element={
          <ProtectedRoute requireVerified>
            <ConnectionsPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/payments"
        element={
          <ProtectedRoute requireVerified>
            <Navigate to="/profile#history" replace />
          </ProtectedRoute>
        }
      />
      <Route
        path="/promocodes"
        element={
          <ProtectedRoute requireVerified>
            <PromoCodesPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/support"
        element={
          <ProtectedRoute requireVerified>
            <SupportPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/info"
        element={
          <ProtectedRoute requireVerified>
            <InfoPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/referral"
        element={
          <ProtectedRoute requireVerified>
            <ReferralProgramPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/partner"
        element={
          <ProtectedRoute requireVerified>
            <PartnerProgramPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/fortune"
        element={
          <ProtectedRoute requireVerified>
            <FortunePage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/loyalty"
        element={
          <ProtectedRoute requireVerified>
            <LoyaltyProgramPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/link/merge"
        element={
          <ProtectedRoute requireVerified>
            <MergePreviewPage />
          </ProtectedRoute>
        }
      />

      {/* ── Admin routes ─────────────────────────────── */}
      <Route
        path="/admin"
        element={
          <ProtectedRoute>
            <AdminRoute>
              <AdminDashboardPage />
            </AdminRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/stats"
        element={
          <ProtectedRoute>
            <AdminRoute>
              <AdminStatsPage />
            </AdminRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/users"
        element={
          <ProtectedRoute>
            <AdminRoute>
              <AdminUsersPage />
            </AdminRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/users/:id"
        element={
          <ProtectedRoute>
            <AdminRoute>
              <AdminUserDetailPage />
            </AdminRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/payments"
        element={
          <ProtectedRoute>
            <AdminRoute>
              <AdminPaymentsPage />
            </AdminRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/promos"
        element={
          <ProtectedRoute>
            <AdminRoute>
              <AdminPromosPage />
            </AdminRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/tariffs"
        element={
          <ProtectedRoute>
            <AdminRoute>
              <AdminTariffsPage />
            </AdminRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/loyalty"
        element={
          <ProtectedRoute>
            <AdminRoute>
              <AdminLoyaltyPage />
            </AdminRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/partners"
        element={
          <ProtectedRoute>
            <AdminRoute>
              <AdminPartnersPage />
            </AdminRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/broadcast"
        element={
          <ProtectedRoute>
            <AdminRoute>
              <AdminBroadcastPage />
            </AdminRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/status"
        element={
          <ProtectedRoute>
            <AdminRoute>
              <AdminStatusPage />
            </AdminRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/infra"
        element={
          <ProtectedRoute>
            <AdminRoute>
              <AdminInfraPage />
            </AdminRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/settings"
        element={
          <ProtectedRoute>
            <AdminRoute>
              <AdminSettingsPage />
            </AdminRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/sync"
        element={
          <ProtectedRoute>
            <AdminRoute>
              <AdminSyncPage />
            </AdminRoute>
          </ProtectedRoute>
        }
      />

      {/* Корень: лендинг на домене, дашборд внутри /cabinet. */}
      <Route path="/" element={<DomainRoot />} />

      {/* Fallbacks */}
      <Route path="*" element={<Navigate to="/dashboard" replace />} />
    </Routes>
    </Suspense>
    </RouteErrorBoundary>
  )
}

/** Мини-приложение Telegram открывает кабинет, а не витрину. Hash с tgWebAppData сохраняем. */
function redirectMiniAppToCabinet(): boolean {
  if (!isTelegramMiniAppSession()) return false
  const { search, hash } = window.location
  window.location.replace(`/cabinet/${search}${hash}`)
  return true
}

function PublicLanding() {
  if (redirectMiniAppToCabinet()) return <FullscreenSpinner />
  return <LandingPage />
}

function DomainRoot() {
  if (window.location.pathname.startsWith('/cabinet')) {
    return <Navigate to="/dashboard" replace />
  }
  return <PublicLanding />
}

/** Общий индикатор: инициализация auth и подгрузка чанка страницы выглядят одинаково. */
function FullscreenSpinner() {
  return (
    <div className="flex min-h-dvh items-center justify-center">
      <span className="size-8 rounded-full border-2 border-primary border-t-transparent animate-spin" />
    </div>
  )
}

export default function App() {
  return (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <ThemePolicyProvider>
          <ToastProvider>
            <BrandFavicon />
            <CabinetDecorThemeSync />
            <BrowserRouter basename={resolveBasename()}>
              <AppRoutes />
            </BrowserRouter>
          </ToastProvider>
        </ThemePolicyProvider>
      </QueryClientProvider>
    </I18nextProvider>
  )
}

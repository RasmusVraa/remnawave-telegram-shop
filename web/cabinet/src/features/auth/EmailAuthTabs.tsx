import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, Mail, Eye, EyeOff } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { api, ApiError } from '@/lib/api'
import { getTurnstileToken } from '@/lib/turnstile'
import { useAuthStore } from '@/store/auth'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { Link, useNavigate } from 'react-router-dom'

type Tab = 'login' | 'register'

export type EmailAuthTabsProps = {
  defaultOpen: boolean
  defaultTab: Tab
  /**
   * Маршрут, куда уводить после успешного login (оставляем прежнюю логику LoginPage).
   */
  from: string
  /**
   * Для сценария реферальной регистрации / Google-OAuth state.
   */
  referralCode?: string
  /**
   * При необходимости показать / скрыть блок подтверждения email от google pending.
   * (он специфичен для login-страницы; для register можно просто передать пустые значения)
   */
  googleLinkPending?: boolean
  /** Название провайдера в тексте «проверьте почту» (Google, Яндекс ID, VK ID). */
  googleLinkProvider?: string
  googleMaskedEmail?: string
}

export function EmailAuthTabs({
  defaultOpen,
  defaultTab,
  from,
  referralCode,
  googleLinkPending = false,
  googleLinkProvider = 'Google',
  googleMaskedEmail = '',
}: EmailAuthTabsProps) {
  const { t } = useTranslation()
  const { setToken, fetchMe } = useAuthStore()
  const { data: bootstrap } = useAuthBootstrap()
  const navigate = useNavigate()

  const [open, setOpen] = useState(defaultOpen)
  const [tab, setTab] = useState<Tab>(defaultTab)

  // Login state
  const [loginEmail, setLoginEmail] = useState('')
  const [loginPassword, setLoginPassword] = useState('')
  const [loginShowPw, setLoginShowPw] = useState(false)
  const [loginLoading, setLoginLoading] = useState(false)
  const [loginSecurityChecking, setLoginSecurityChecking] = useState(false)
  const [loginError, setLoginError] = useState<string | null>(null)

  // Register state
  const [registerEmail, setRegisterEmail] = useState('')
  const [registerPassword, setRegisterPassword] = useState('')
  const [registerConfirm, setRegisterConfirm] = useState('')
  const [registerShowPw, setRegisterShowPw] = useState(false)
  const [registerLoading, setRegisterLoading] = useState(false)
  const [registerSecurityChecking, setRegisterSecurityChecking] = useState(false)
  const [registerError, setRegisterError] = useState<string | null>(null)

  // Поддерживаем прежнюю логику LoginPage: если пришли google_link pending данные, перенаправления и alert-и остаются на уровне страницы,
  // а здесь показываем сам alert по флагам.
  useEffect(() => {
    setLoginError(null)
    setRegisterError(null)
  }, [tab])

  const loginDisabled = useMemo(() => loginLoading || loginSecurityChecking, [loginLoading, loginSecurityChecking])
  const registerDisabled = useMemo(
    () => registerLoading || registerSecurityChecking,
    [registerLoading, registerSecurityChecking],
  )

  async function handleLoginSubmit(e: FormEvent) {
    e.preventDefault()
    setLoginError(null)
    if (!loginEmail || !loginPassword) {
      setLoginError(t('errors.required'))
      return
    }
    setLoginLoading(true)
    try {
      let turnstileToken: string | undefined
      if (bootstrap?.turnstile_enabled) {
        const siteKey = (bootstrap.turnstile_site_key ?? '').trim()
        if (!siteKey) throw new Error('turnstile site key is missing')
        setLoginSecurityChecking(true)
        turnstileToken = await getTurnstileToken(siteKey, 'login')
        setLoginSecurityChecking(false)
      }
      const data = await api.login(loginEmail, loginPassword, turnstileToken)
      setToken(data.access_token)
      await fetchMe()
      // LoginPage сам делает navigate в своём useEffect — тут оставляем единый сценарий:
      navigate(from, { replace: true })
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          const body = err.body.toLowerCase()
          if (body.includes('not verified') || body.includes('email')) {
            navigate('/verify-email', { state: { email: loginEmail }, replace: true })
            return
          }
          setLoginError(t('errors.invalidCredentials'))
        } else if (err.status === 429) {
          setLoginError(t('errors.tooManyRequests'))
        } else {
          setLoginError(t('errors.unknown'))
        }
      } else {
        setLoginError(t('errors.unknown'))
      }
    } finally {
      setLoginSecurityChecking(false)
      setLoginLoading(false)
    }
  }

  function validateRegister(): string | null {
    if (!registerEmail) return t('errors.required')
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(registerEmail)) return t('errors.invalidEmail')
    if (registerPassword.length < 8) return t('errors.passwordMin')
    if (registerPassword !== registerConfirm) return t('errors.passwordMismatch')
    return null
  }

  async function handleRegisterSubmit(e: FormEvent) {
    e.preventDefault()
    const validErr = validateRegister()
    if (validErr) {
      setRegisterError(validErr)
      return
    }
    setRegisterError(null)
    setRegisterLoading(true)
    try {
      let turnstileToken: string | undefined
      if (bootstrap?.turnstile_enabled) {
        const siteKey = (bootstrap.turnstile_site_key ?? '').trim()
        if (!siteKey) throw new Error('turnstile site key is missing')
        setRegisterSecurityChecking(true)
        turnstileToken = await getTurnstileToken(siteKey, 'register')
        setRegisterSecurityChecking(false)
      }
      await api.register(registerEmail, registerPassword, referralCode || undefined, turnstileToken)
      navigate('/verify-email', { state: { email: registerEmail }, replace: true })
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 409) {
          navigate('/verify-email', { state: { email: registerEmail }, replace: true })
        } else if (err.status === 429) {
          setRegisterError(t('errors.tooManyRequests'))
        } else {
          setRegisterError(t('errors.unknown'))
        }
      } else {
        setRegisterError(t('errors.unknown'))
      }
    } finally {
      setRegisterSecurityChecking(false)
      setRegisterLoading(false)
    }
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-3">
        <span className="h-px flex-1 bg-border/70" />
        <Button
          type="button"
          variant="outline"
          className="h-auto min-w-[168px] rounded-full border-border/80 bg-card/70 px-3.5 py-1.5 text-[0.75rem] font-semibold text-[rgb(100,116,139)] hover:bg-muted/55 dark:text-[#e5e7eb]"
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
        >
          <span className="inline-flex items-center gap-2">
            <Mail className="size-[0.95rem] opacity-80" />
            <span>{t('auth.emailLoginToggle')}</span>
          </span>
          <ChevronDown className={`size-[0.95rem] opacity-70 transition-transform ${open ? 'rotate-180' : ''}`} />
        </Button>
        <span className="h-px flex-1 bg-border/70" />
      </div>

      <div
        className={`grid overflow-hidden transition-[grid-template-rows,opacity,transform] duration-[400ms] ease-out ${
          open
            ? 'visible mt-1 grid-rows-[1fr] opacity-100 translate-y-0'
            : 'invisible grid-rows-[0fr] opacity-0 -translate-y-1 pointer-events-none'
        }`}
        aria-hidden={!open}
      >
        <div className="space-y-4 overflow-hidden">
          <div className="flex gap-2">
            <Button
              type="button"
              variant={tab === 'login' ? 'default' : 'outline'}
              className="flex-1"
              onClick={() => setTab('login')}
            >
              {t('auth.emailLoginTab')}
            </Button>
            <Button
              type="button"
              variant={tab === 'register' ? 'default' : 'outline'}
              className="flex-1"
              onClick={() => setTab('register')}
            >
              {t('auth.emailRegisterTab')}
            </Button>
          </div>

          {tab === 'login' ? (
            <form onSubmit={handleLoginSubmit} className="space-y-4" noValidate>
              {googleLinkPending && (
                <Alert>
                  <AlertDescription>
                    {t('auth.googleLinkPending', { email: googleMaskedEmail || '—', provider: googleLinkProvider })}
                  </AlertDescription>
                </Alert>
              )}
              {loginError && (
                <Alert variant="destructive">
                  <AlertDescription>{loginError}</AlertDescription>
                </Alert>
              )}
              {loginSecurityChecking && (
                <Alert>
                  <AlertDescription>{t('auth.securityCheckInProgress')}</AlertDescription>
                </Alert>
              )}

              <div className="space-y-1.5">
                <Label htmlFor="email-login">{t('auth.email')}</Label>
                <Input
                  id="email-login"
                  type="email"
                  autoComplete="email"
                  placeholder={t('auth.emailPlaceholder')}
                  value={loginEmail}
                  onChange={(e) => setLoginEmail(e.target.value)}
                  error={!!loginError}
                  required
                />
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="password-login">{t('auth.password')}</Label>
                <div className="relative">
                  <Input
                    id="password-login"
                    type={loginShowPw ? 'text' : 'password'}
                    autoComplete="current-password"
                    placeholder={t('auth.passwordPlaceholder')}
                    value={loginPassword}
                    onChange={(e) => setLoginPassword(e.target.value)}
                    error={!!loginError}
                    required
                  />
                  <button
                    type="button"
                    onClick={() => setLoginShowPw((v) => !v)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                    tabIndex={-1}
                    aria-label={loginShowPw ? 'Hide password' : 'Show password'}
                  >
                    {loginShowPw ? <EyeOff size={16} /> : <Eye size={16} />}
                  </button>
                </div>
              </div>

              <Button type="submit" className="w-full" loading={loginDisabled}>
                {t('auth.emailLoginSubmit')}
              </Button>

              <Link
                to="/password/forgot"
                className="block text-center text-xs text-muted-foreground hover:text-primary transition-colors"
              >
                {t('auth.forgotPassword')}
              </Link>
            </form>
          ) : (
            <form onSubmit={handleRegisterSubmit} className="space-y-4" noValidate>
              {registerError && (
                <Alert variant="destructive">
                  <AlertDescription>{registerError}</AlertDescription>
                </Alert>
              )}
              {registerSecurityChecking && (
                <Alert>
                  <AlertDescription>{t('auth.securityCheckInProgress')}</AlertDescription>
                </Alert>
              )}

              <div className="space-y-1.5">
                <Label htmlFor="email-register">{t('auth.email')}</Label>
                <Input
                  id="email-register"
                  type="email"
                  autoComplete="email"
                  placeholder={t('auth.emailPlaceholder')}
                  value={registerEmail}
                  onChange={(e) => setRegisterEmail(e.target.value)}
                  error={!!registerError}
                  required
                />
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="password-register">{t('auth.password')}</Label>
                <div className="relative">
                  <Input
                    id="password-register"
                    type={registerShowPw ? 'text' : 'password'}
                    autoComplete="new-password"
                    placeholder={t('auth.passwordPlaceholder')}
                    value={registerPassword}
                    onChange={(e) => setRegisterPassword(e.target.value)}
                    error={!!registerError}
                    required
                  />
                  <button
                    type="button"
                    onClick={() => setRegisterShowPw((v) => !v)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    tabIndex={-1}
                  >
                    {registerShowPw ? <EyeOff size={16} /> : <Eye size={16} />}
                  </button>
                </div>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="confirm-register">{t('auth.passwordConfirm')}</Label>
                <Input
                  id="confirm-register"
                  type={registerShowPw ? 'text' : 'password'}
                  autoComplete="new-password"
                  placeholder={t('auth.passwordPlaceholder')}
                  value={registerConfirm}
                  onChange={(e) => setRegisterConfirm(e.target.value)}
                  error={registerPassword !== registerConfirm && registerConfirm.length > 0}
                  required
                />
              </div>

              <Button type="submit" className="w-full" loading={registerDisabled}>
                {t('auth.emailRegisterSubmit')}
              </Button>
            </form>
          )}
        </div>
      </div>
    </div>
  )
}


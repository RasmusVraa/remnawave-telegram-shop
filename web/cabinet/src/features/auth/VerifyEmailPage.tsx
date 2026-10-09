import { useState, useEffect, type FormEvent } from 'react'
import { Link, useLocation, useSearchParams, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { MailCheck } from 'lucide-react'

import { AuthLayout } from '@/components/AuthLayout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { api, ApiError } from '@/lib/api'
import { getTurnstileToken } from '@/lib/turnstile'
import { maskEmail } from '@/lib/utils'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { useAuthStore } from '@/store/auth'

const RESEND_COOLDOWN = 60

function isLegacyVerifyToken(raw: string | null): boolean {
  const t = (raw ?? '').trim()
  if (!t) return false
  if (/^\d{6}$/.test(t)) return false
  return true
}

export default function VerifyEmailPage() {
  const { t } = useTranslation()
  const location = useLocation()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const setToken = useAuthStore((s) => s.setToken)
  const fetchMe = useAuthStore((s) => s.fetchMe)
  const accessToken = useAuthStore((s) => s.accessToken)
  const user = useAuthStore((s) => s.user)
  const { data: bootstrap } = useAuthBootstrap()

  const emailFromState = (location.state as { email?: string })?.email ?? ''
  const tokenFromURL = searchParams.get('token')

  const [resendCooldown, setResendCooldown] = useState(0)
  const [resendLoading, setResendLoading] = useState(false)
  const [resendSecurity, setResendSecurity] = useState(false)
  const [message, setMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [code, setCode] = useState('')
  const [confirmLoading, setConfirmLoading] = useState(false)
  const [changeLoading, setChangeLoading] = useState(false)
  const [legacyLoading, setLegacyLoading] = useState(!!tokenFromURL && isLegacyVerifyToken(tokenFromURL))

  // Старые письма со ссылкой ?token=base64… — подтверждаем автоматически.
  useEffect(() => {
    if (!tokenFromURL || !isLegacyVerifyToken(tokenFromURL)) return
    const tok = tokenFromURL.trim()
    api
      .confirmEmail(tok)
      .then(async (data) => {
        setToken(data.access_token)
        await fetchMe()
        navigate('/dashboard', { replace: true })
      })
      .catch(() => {
        setLegacyLoading(false)
        setError(t('errors.unknown'))
        navigate('/verify-email', { replace: true, state: location.state })
      })
  }, [tokenFromURL, fetchMe, navigate, setToken, t])

  useEffect(() => {
    if (resendCooldown <= 0) return
    const id = setInterval(() => setResendCooldown((v) => v - 1), 1000)
    return () => clearInterval(id)
  }, [resendCooldown])

  async function handleResend() {
    if (!accessToken && !emailFromState.trim()) {
      setError(t('verifyEmail.resendNeedEmail'))
      return
    }
    setResendLoading(true)
    setResendSecurity(false)
    setMessage(null)
    setError(null)
    try {
      if (accessToken) {
        await api.resendVerifyEmail()
      } else {
        let turnstileToken: string | undefined
        if (bootstrap?.turnstile_enabled) {
          const siteKey = (bootstrap.turnstile_site_key ?? '').trim()
          if (!siteKey) throw new Error('turnstile site key is missing')
          setResendSecurity(true)
          turnstileToken = await getTurnstileToken(siteKey, 'register')
          setResendSecurity(false)
        }
        await api.resendVerifyEmailPublic(emailFromState.trim(), turnstileToken)
      }
      setResendCooldown(RESEND_COOLDOWN)
      setMessage(t('verifyEmail.resend'))
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) {
        setError(t('errors.tooManyRequests'))
      } else {
        setError(t('errors.unknown'))
      }
    } finally {
      setResendSecurity(false)
      setResendLoading(false)
    }
  }

  // Почта привязана к Telegram/OAuth-аккаунту как доп. способ входа, но не
  // подтверждена (например, опечатка в адресе). Без этой кнопки пользователь
  // заперт: requireVerified-роуты всегда ведут сюда, а код уходит не туда.
  const canChangeEmail =
    !!accessToken && !!user && !user.email_verified && user.providers.some((p) => p !== 'email')

  async function handleChangeEmail() {
    setChangeLoading(true)
    setMessage(null)
    setError(null)
    try {
      await api.identityUnlink('email')
      // В текущем JWT ещё email_verified=false — без refresh бэкенд вернёт 403
      // на подписку и платежи.
      const data = await api.refresh()
      setToken(data.access_token)
      await fetchMe()
      navigate('/accounts/email', { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 400) {
        setError(t('accounts.unlinkLastMethod'))
      } else {
        setError(t('errors.unknown'))
      }
    } finally {
      setChangeLoading(false)
    }
  }

  async function handleConfirm(e: FormEvent) {
    e.preventDefault()
    if (code.length !== 6) return
    setConfirmLoading(true)
    setError(null)
    try {
      const data = await api.confirmEmail(code)
      setToken(data.access_token)
      await fetchMe()
      navigate('/dashboard', { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) {
        setError(t('errors.tooManyRequests'))
      } else {
        setError(t('verifyEmail.invalidCode'))
      }
    } finally {
      setConfirmLoading(false)
    }
  }

  if (tokenFromURL && isLegacyVerifyToken(tokenFromURL)) {
    return (
      <AuthLayout>
        <Card>
          <CardContent className="pt-4 sm:pt-6 text-center space-y-3">
            {error ? (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : (
              <p className="text-muted-foreground">{legacyLoading ? t('common.loading') : null}</p>
            )}
          </CardContent>
        </Card>
      </AuthLayout>
    )
  }

  return (
    <AuthLayout>
      <Card>
        <CardHeader className="items-center text-center">
          <div className="flex h-14 w-14 items-center justify-center rounded-full bg-primary/10 mb-2">
            <MailCheck className="text-primary" size={28} strokeWidth={1.5} />
          </div>
          <CardTitle>{t('verifyEmail.title')}</CardTitle>
        </CardHeader>

        <CardContent className="space-y-4 text-center">
          {emailFromState && (
            <p className="text-sm text-muted-foreground">
              {t('verifyEmail.subtitle', { email: maskEmail(emailFromState) })}
            </p>
          )}

          {message && (
            <Alert variant="success">
              <AlertDescription>{message}</AlertDescription>
            </Alert>
          )}
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <form onSubmit={handleConfirm} className="space-y-3 text-left">
            <div className="space-y-1.5">
              <Label htmlFor="verify-code">{t('verifyEmail.codeLabel')}</Label>
              <Input
                id="verify-code"
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={6}
                placeholder={t('verifyEmail.codePlaceholder')}
                value={code}
                onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                className="text-center text-lg tracking-[0.25em] font-mono"
              />
            </div>
            <Button type="submit" className="w-full" loading={confirmLoading} disabled={code.length !== 6}>
              {t('verifyEmail.confirm')}
            </Button>
          </form>

          <p className="text-xs text-muted-foreground">{t('verifyEmail.checkSpam')}</p>

          <Button
            variant="outline"
            className="w-full"
            onClick={handleResend}
            loading={resendLoading || resendSecurity}
            disabled={resendCooldown > 0 || (!accessToken && !emailFromState.trim())}
          >
            {resendCooldown > 0
              ? t('verifyEmail.resendCooldown', { sec: resendCooldown })
              : t('verifyEmail.resend')}
          </Button>

          {canChangeEmail && (
            <Button variant="ghost" className="w-full" onClick={handleChangeEmail} loading={changeLoading}>
              {t('verifyEmail.changeEmail')}
            </Button>
          )}

          <Link
            to="/login"
            className="block text-xs text-muted-foreground hover:text-primary transition-colors"
          >
            {t('verifyEmail.backToLogin')}
          </Link>
        </CardContent>
      </Card>
    </AuthLayout>
  )
}

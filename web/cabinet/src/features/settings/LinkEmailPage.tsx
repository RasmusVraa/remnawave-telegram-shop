import { useEffect, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Eye, EyeOff } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal } from '@/components/PageReveal'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { api, ApiError } from '@/lib/api'
import { useErrorHaptic } from '@/lib/haptic'
import { useAuthStore } from '@/store/auth'

export default function LinkEmailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { user, fetchMe } = useAuthStore()

  useEffect(() => {
    void fetchMe()
  }, [fetchMe])

  useEffect(() => {
    if (user?.can_use_email_password_login) {
      navigate('/accounts', { replace: true })
    }
  }, [user?.can_use_email_password_login, navigate])

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [showPw, setShowPw] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)
  const [verifyCodeRequired, setVerifyCodeRequired] = useState(false)
  const [maskedEmail, setMaskedEmail] = useState<string>('')
  const [code, setCode] = useState('')

  function validate(): string | null {
    if (!email.trim()) return t('errors.required')
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) return t('errors.invalidEmail')
    if (password.length < 8) return t('errors.passwordMin')
    if (password !== confirm) return t('errors.passwordMismatch')
    return null
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const validErr = validate()
    if (validErr) {
      setError(validErr)
      return
    }
    setError(null)
    setLoading(true)
    try {
      const res = await api.linkEmail(email.trim(), password, confirm)
      await fetchMe()
      if (res.status === 'merge_verification_required') {
        setVerifyCodeRequired(true)
        setMaskedEmail(res.masked_email || email.trim())
        return
      }
      if (res.status === 'merge_required') {
        navigate('/link/merge?provider=email', { replace: true })
        return
      }
      navigate('/verify-email', { state: { email: email.trim() } })
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setError(t('errors.invalidCredentials'))
      } else if (err instanceof ApiError && err.status === 429) {
        setError(t('errors.tooManyRequests'))
      } else if (err instanceof ApiError && err.status === 400) {
        const body = (err.body || '').toLowerCase()
        if (body.includes('email is already used in') && body.includes('sign-in')) {
          setError(t('accounts.emailUsedBySocial'))
        } else {
          setError(err.body || t('errors.unknown'))
        }
      } else {
        setError(t('errors.unknown'))
      }
    } finally {
      setLoading(false)
    }
  }

  async function handleVerifyCodeSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      const res = await api.confirmEmailMergeCode(code.trim())
      if (res.status === 'merge_required') {
        navigate('/link/merge?provider=email', { replace: true })
        return
      }
      setError(t('errors.unknown'))
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setError(t('errors.invalidCredentials'))
      } else if (err instanceof ApiError && err.status === 400) {
        setError(err.body || t('errors.unknown'))
      } else {
        setError(t('errors.unknown'))
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <AppLayout>
      <PageReveal className="max-w-md mx-auto w-full py-4" wrapChildren>
        <Card>
          <CardHeader>
            <CardTitle>{t('accounts.linkEmailPageTitle')}</CardTitle>
            <CardDescription>{t('accounts.linkEmailPageSubtitle')}</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={verifyCodeRequired ? handleVerifyCodeSubmit : handleSubmit} className="space-y-4" noValidate>
              {error ? (
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              ) : null}
              {verifyCodeRequired ? (
                <Alert>
                  <AlertDescription>
                    Мы отправили код подтверждения на {maskedEmail}. Введите 6 цифр, чтобы продолжить объединение.
                  </AlertDescription>
                </Alert>
              ) : null}
              {!verifyCodeRequired ? (
                <>
              <div className="space-y-1.5">
                <Label htmlFor="link-email">{t('auth.email')}</Label>
                <Input
                  id="link-email"
                  type="email"
                  autoComplete="email"
                  placeholder={t('auth.emailPlaceholder')}
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                />
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="link-pw">{t('auth.password')}</Label>
                <div className="relative">
                  <Input
                    id="link-pw"
                    type={showPw ? 'text' : 'password'}
                    autoComplete="new-password"
                    placeholder={t('auth.passwordPlaceholder')}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    error={!!error}
                    required
                  />
                  <button
                    type="button"
                    onClick={() => setShowPw((v) => !v)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    tabIndex={-1}
                  >
                    {showPw ? <EyeOff size={16} /> : <Eye size={16} />}
                  </button>
                </div>
                <p className="text-xs text-muted-foreground">{t('errors.passwordMin')}</p>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="link-pw2">{t('auth.passwordConfirm')}</Label>
                <Input
                  id="link-pw2"
                  type={showPw ? 'text' : 'password'}
                  autoComplete="new-password"
                  placeholder={t('auth.passwordConfirm')}
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                  error={password !== confirm && confirm.length > 0}
                  required
                />
              </div>
                </>
              ) : (
                <div className="space-y-1.5">
                  <Label htmlFor="email-merge-code">Код подтверждения</Label>
                  <Input
                    id="email-merge-code"
                    inputMode="numeric"
                    pattern="[0-9]*"
                    maxLength={6}
                    value={code}
                    onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                    placeholder="000000"
                    required
                  />
                </div>
              )}

              <Button type="submit" className="w-full" loading={loading} disabled={verifyCodeRequired && code.length !== 6}>
                {t('accounts.linkEmailSubmit')}
              </Button>

              <p className="text-center text-sm">
                <Link to="/accounts" className="text-primary hover:underline">
                  {t('accounts.backToAccounts')}
                </Link>
              </p>
            </form>
          </CardContent>
        </Card>
      </PageReveal>
    </AppLayout>
  )
}

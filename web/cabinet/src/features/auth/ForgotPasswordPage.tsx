import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { CheckCircle2 } from 'lucide-react'

import { AuthLayout } from '@/components/AuthLayout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { api, ApiError } from '@/lib/api'
import { useErrorHaptic } from '@/lib/haptic'
import { getTurnstileToken } from '@/lib/turnstile'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'

export default function ForgotPasswordPage() {
  const { t } = useTranslation()
  const { data: bootstrap } = useAuthBootstrap()

  const [email, setEmail] = useState('')
  const [loading, setLoading] = useState(false)
  const [securityChecking, setSecurityChecking] = useState(false)
  const [sent, setSent] = useState(false)
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!email) { setError(t('errors.required')); return }
    setError(null)
    setLoading(true)
    try {
      let turnstileToken: string | undefined
      if (bootstrap?.turnstile_enabled) {
        const siteKey = (bootstrap.turnstile_site_key ?? '').trim()
        if (!siteKey) throw new Error('turnstile site key is missing')
        setSecurityChecking(true)
        turnstileToken = await getTurnstileToken(siteKey, 'forgot')
        setSecurityChecking(false)
      }
      await api.forgotPassword(email, turnstileToken)
      setSent(true)
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) {
        setError(t('errors.tooManyRequests'))
      } else {
        // Anti-enumeration: всегда показываем "успех"
        setSent(true)
      }
    } finally {
      setSecurityChecking(false)
      setLoading(false)
    }
  }

  return (
    <AuthLayout>
      <Card>
        <CardHeader>
          <CardTitle>{t('forgotPassword.title')}</CardTitle>
          <CardDescription>{t('forgotPassword.subtitle')}</CardDescription>
        </CardHeader>

        <CardContent className="space-y-4">
          {sent ? (
            <div className="space-y-4 text-center">
              <div className="flex justify-center">
                <CheckCircle2 className="text-primary" size={40} strokeWidth={1.5} />
              </div>
              <Alert variant="success">
                <AlertDescription>{t('forgotPassword.sent')}</AlertDescription>
              </Alert>
            </div>
          ) : (
            <form onSubmit={handleSubmit} className="space-y-4" noValidate>
              {error && (
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}
              {securityChecking && (
                <Alert>
                  <AlertDescription>{t('auth.securityCheckInProgress')}</AlertDescription>
                </Alert>
              )}
              <div className="space-y-1.5">
                <Label htmlFor="email">{t('auth.email')}</Label>
                <Input
                  id="email"
                  type="email"
                  autoComplete="email"
                  placeholder={t('auth.emailPlaceholder')}
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  error={!!error}
                />
              </div>
              <Button type="submit" className="w-full" loading={loading}>
                {t('forgotPassword.send')}
              </Button>
            </form>
          )}

          <div className="text-center">
            <Link
              to="/login"
              className="text-xs text-muted-foreground hover:text-primary transition-colors"
            >
              {t('forgotPassword.backToLogin')}
            </Link>
          </div>
        </CardContent>
      </Card>
    </AuthLayout>
  )
}

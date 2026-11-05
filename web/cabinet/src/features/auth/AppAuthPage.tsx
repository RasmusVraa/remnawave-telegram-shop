import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'

import { AuthLayout } from '@/components/AuthLayout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { api, ApiError } from '@/lib/api'
import { useAuthStore } from '@/store/auth'

/**
 * Страница подтверждения входа в десктоп/мобильное приложение.
 * Открывается по web_url из POST /auth/deeplink/request.
 */
export default function AppAuthPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const token = (params.get('t') || '').trim()
  const { accessToken, initialized } = useAuthStore()
  const [status, setStatus] = useState<'idle' | 'working' | 'ok' | 'error'>('idle')
  const [error, setError] = useState('')

  useEffect(() => {
    if (!initialized) return
    if (!token) {
      setStatus('error')
      setError(t('auth.appAuthMissingToken', { defaultValue: 'Нет токена авторизации.' }))
      return
    }
    if (!accessToken) {
      navigate('/login', { replace: true, state: { from: `/app-auth?t=${encodeURIComponent(token)}` } })
      return
    }
    let cancelled = false
    ;(async () => {
      setStatus('working')
      try {
        await api.webAuthComplete(token)
        if (!cancelled) setStatus('ok')
      } catch (e) {
        if (cancelled) return
        setStatus('error')
        if (e instanceof ApiError && e.status === 400) {
          setError(t('auth.appAuthInvalid', { defaultValue: 'Токен недействителен или уже использован.' }))
        } else {
          setError(t('auth.appAuthFailed', { defaultValue: 'Не удалось подтвердить вход.' }))
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [initialized, accessToken, token, navigate, t])

  return (
    <AuthLayout>
      <Card className="mx-auto w-full max-w-md">
        <CardHeader>
          <CardTitle>{t('auth.appAuthTitle', { defaultValue: 'Вход в приложение' })}</CardTitle>
          <CardDescription>
            {t('auth.appAuthHint', {
              defaultValue: 'Подтвердите вход в StarlitConnect / клиент VPN.',
            })}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {status === 'working' || status === 'idle' ? (
            <p className="text-sm text-muted-foreground">
              {t('common.loading', { defaultValue: 'Загрузка…' })}
            </p>
          ) : null}
          {status === 'ok' ? (
            <Alert>
              <AlertDescription>
                {t('auth.appAuthOk', {
                  defaultValue: 'Готово. Можно вернуться в приложение — оно подхватит подписку само.',
                })}
              </AlertDescription>
            </Alert>
          ) : null}
          {status === 'error' ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          <Button asChild variant="outline" className="w-full">
            <Link to="/dashboard">{t('nav.dashboard', { defaultValue: 'В кабинет' })}</Link>
          </Button>
        </CardContent>
      </Card>
    </AuthLayout>
  )
}

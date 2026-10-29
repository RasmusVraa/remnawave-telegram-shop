import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { ChevronDown, ChevronUp, Eye, EyeOff } from 'lucide-react'

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { api, ApiError } from '@/lib/api'
import { useAuthStore } from '@/store/auth'
import { cn } from '@/lib/utils'

/** Смена пароля: свёрнут по умолчанию. */
export function ChangePasswordCollapsible({ onSuccess }: { onSuccess: (token: string) => void }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [show, setShow] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [ok, setOk] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (next.length < 8) {
      setError(t('errors.passwordMin'))
      return
    }
    if (next !== confirm) {
      setError(t('errors.passwordMismatch'))
      return
    }
    setError(null)
    setLoading(true)
    try {
      const data = await api.changePassword(current, next)
      onSuccess(data.access_token)
      setOk(true)
      setCurrent('')
      setNext('')
      setConfirm('')
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) setError(t('errors.invalidCredentials'))
        else if (err.status === 400) setError(t('errors.passwordMin'))
        else if (err.status === 429) setError(t('errors.tooManyRequests'))
        else setError(t('errors.unknown'))
      } else setError(t('errors.unknown'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card className="overflow-hidden">
      <button
        type="button"
        className="flex w-full items-center justify-between gap-3 px-6 py-4 text-left outline-none transition-colors hover:bg-muted/35 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
        onClick={() => { setOpen((o) => !o); setError(null) }}
        aria-expanded={open}
      >
        <span className="text-base font-semibold leading-none">{t('settings.password.title')}</span>
        {open ? <ChevronUp className="size-4 shrink-0 text-muted-foreground" /> : <ChevronDown className="size-4 shrink-0 text-muted-foreground" />}
      </button>
      {open && (
        <CardContent className="border-t border-border/60 px-6 pb-6 pt-4">
          <form onSubmit={submit} className="space-y-3">
            {ok && (
              <Alert variant="success">
                <AlertDescription>{t('settings.password.success')}</AlertDescription>
              </Alert>
            )}
            {error && (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <div className="space-y-1.5">
              <Label>{t('settings.password.current')}</Label>
              <div className="relative">
                <Input
                  type={show ? 'text' : 'password'}
                  value={current}
                  onChange={(e) => setCurrent(e.target.value)}
                  autoComplete="current-password"
                />
                <button
                  type="button"
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground"
                  onClick={() => setShow((s) => !s)}
                >
                  {show ? <EyeOff size={14} /> : <Eye size={14} />}
                </button>
              </div>
            </div>
            <div className="space-y-1.5">
              <Label>{t('settings.password.new')}</Label>
              <Input
                type={show ? 'text' : 'password'}
                value={next}
                onChange={(e) => setNext(e.target.value)}
                autoComplete="new-password"
              />
            </div>
            <div className="space-y-1.5">
              <Label>{t('settings.password.confirm')}</Label>
              <Input
                type={show ? 'text' : 'password'}
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                autoComplete="new-password"
              />
            </div>
            <Button type="submit" loading={loading} size="sm">
              {t('settings.password.submit')}
            </Button>
          </form>
        </CardContent>
      )}
    </Card>
  )
}

export function DeleteAccountSection({ className }: { className?: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { logout } = useAuthStore()
  const [open, setOpen] = useState(false)
  const [confirmStep, setConfirmStep] = useState(false)
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  async function submit() {
    setErr(null)
    setLoading(true)
    try {
      await api.deleteAccount()
      logout()
      navigate('/login', { replace: true })
    } catch (e) {
      setErr(t('errors.deleteAccountFailed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card className={cn('border-destructive/40', className)}>
      <CardHeader>
        <CardTitle className="text-base text-destructive">{t('settings.deleteAccount.title')}</CardTitle>
        <CardDescription>{t('settings.deleteAccount.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {!open ? (
          <Button type="button" variant="destructive" size="sm" onClick={() => { setOpen(true); setConfirmStep(false); setErr(null) }}>
            {t('settings.deleteAccount.openButton')}
          </Button>
        ) : (
          <>
            <Alert variant="destructive">
              <AlertDescription>{t('settings.deleteAccount.warning')}</AlertDescription>
            </Alert>
            {err && (
              <Alert variant="destructive">
                <AlertDescription>{err}</AlertDescription>
              </Alert>
            )}
            {confirmStep ? (
              <p className="text-sm text-muted-foreground">{t('settings.deleteAccount.areYouSure')}</p>
            ) : null}
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => { setOpen(false); setConfirmStep(false); setErr(null) }}
                disabled={loading}
              >
                {t('settings.deleteAccount.cancel')}
              </Button>
              {!confirmStep ? (
                <Button
                  type="button"
                  variant="destructive"
                  size="sm"
                  onClick={() => { setConfirmStep(true); setErr(null) }}
                  disabled={loading}
                >
                  {t('settings.deleteAccount.submit')}
                </Button>
              ) : (
                <Button type="button" variant="destructive" size="sm" loading={loading} onClick={() => void submit()}>
                  {t('settings.deleteAccount.submitForever')}
                </Button>
              )}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}

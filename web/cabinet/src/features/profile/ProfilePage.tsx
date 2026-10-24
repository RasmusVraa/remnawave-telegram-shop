import { useEffect, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ArrowRight, ChevronRight, CreditCard, Gem, Gift, Link2, User } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuthStore } from '@/store/auth'
import { api } from '@/lib/api'
import { cn, formatDate, maskEmail } from '@/lib/utils'
import { shareLink } from '@/lib/share'
import { useTranslationWithLang } from '@/hooks/useTranslationWithLang'
import { ChangePasswordCollapsible, DeleteAccountSection } from '@/features/profile/account-security'
import { ProfileLoyaltySection } from '@/features/loyalty/LoyaltyProgramPage'
import { PaymentsHistoryCard } from '@/features/payments/PaymentsHistoryPage'
import { ReferralCopyRow } from '@/features/referral/ReferralCopyRow'
import { ProfileIdentityHeader, useProfileIdentity } from '@/features/profile/ProfileIdentity'

type ProfileTab = 'general' | 'bonuses' | 'history'

function tabFromHash(hash: string): ProfileTab {
  const h = hash.replace(/^#/, '')
  if (h === 'history' || h === 'payments') return 'history'
  if (h === 'bonuses' || h === 'loyalty') return 'bonuses'
  return 'general'
}

function hashForTab(tab: ProfileTab): string {
  if (tab === 'bonuses') return 'bonuses'
  if (tab === 'history') return 'history'
  return ''
}

export default function ProfilePage() {
  const { t } = useTranslation()
  const { lang } = useTranslationWithLang()
  const navigate = useNavigate()
  const location = useLocation()
  const { user, fetchMe, setUser } = useAuthStore()
  const [broadcastSaving, setBroadcastSaving] = useState(false)
  const [tab, setTab] = useState<ProfileTab>(() => tabFromHash(location.hash))
  const identity = useProfileIdentity()

  const { data: referrals, isPending: referralsPending } = useQuery({
    queryKey: ['referrals'],
    queryFn: () => api.referrals(),
    staleTime: 60_000,
    retry: 1,
  })

  useEffect(() => {
    void fetchMe()
  }, [fetchMe])

  useEffect(() => {
    setTab(tabFromHash(location.hash))
  }, [location.hash])

  const cabinetRefUrl =
    referrals?.cabinet_register_link ||
    (user?.telegram_id != null && Number.isFinite(user.telegram_id)
      ? `${window.location.origin}/cabinet/register?ref=ref_${user.telegram_id}`
      : null)
  const botRefUrl = referrals?.bot_start_link ?? null
  const hasReferralLinks = Boolean(botRefUrl || cabinetRefUrl)

  async function toggleBroadcast() {
    if (!user || broadcastSaving) return
    const next = !user.broadcast_opt_out
    setBroadcastSaving(true)
    try {
      await api.putBroadcast(next)
      setUser({ ...user, broadcast_opt_out: next })
    } catch {
      /* оставляем прежнее значение, если сервер не принял */
    } finally {
      setBroadcastSaving(false)
    }
  }

  function goTab(next: ProfileTab) {
    setTab(next)
    const h = hashForTab(next)
    if (h) {
      navigate({ pathname: '/profile', hash: h }, { replace: true })
    } else {
      navigate('/profile', { replace: true })
    }
  }

  /** Способ выбирает `shareLink`: Mini App, системный шит или вкладка t.me. */
  function shareRef(url: string): Promise<boolean> {
    return shareLink({ text: t('referralPage.shareInviteText'), url })
  }

  /** Способов входа меньше двух — подсветка CTA «Привязанные аккаунты» как у «Подключить устройство». */
  const linkedMethodCount = user?.providers?.length ?? 0
  const pulseLinkedAccounts = linkedMethodCount < 2

  const emailMethodLinked = Boolean(user?.can_use_email_password_login)
  const emailDisplay =
    emailMethodLinked && user?.email && String(user.email).trim() !== ''
      ? maskEmail(String(user.email))
      : '—'

  const tabs: { id: ProfileTab; labelKey: string; icon: typeof User }[] = [
    { id: 'general', labelKey: 'profile.tabGeneral', icon: User },
    { id: 'bonuses', labelKey: 'profile.tabBonuses', icon: Gem },
    { id: 'history', labelKey: 'profile.tabHistory', icon: CreditCard },
  ]

  return (
    <AppLayout>
      <PageReveal className="mx-auto w-full max-w-xl space-y-4">
        <RevealItem>
          <h1 className="text-2xl font-semibold">{t('profile.title')}</h1>
        </RevealItem>

        <RevealItem>
        <div
          role="tablist"
          aria-label={t('profile.title')}
          className="flex gap-0.5 rounded-lg border border-border/80 bg-muted/35 p-0.5"
        >
          {tabs.map(({ id, labelKey, icon: Icon }) => (
            <button
              key={id}
              type="button"
              role="tab"
              aria-selected={tab === id}
              className={cn(
                'flex min-w-0 flex-1 items-center justify-center gap-1 rounded-md px-1.5 py-1.5 text-[11px] font-medium transition-colors sm:text-xs',
                tab === id
                  ? 'bg-secondary text-primary shadow-sm'
                  : 'text-muted-foreground hover:text-foreground',
              )}
              onClick={() => goTab(id)}
            >
              <Icon
                className={cn('size-3.5 shrink-0', tab !== id && 'opacity-90')}
                strokeWidth={2}
                aria-hidden
              />
              <span className="truncate">{t(labelKey)}</span>
            </button>
          ))}
        </div>
        </RevealItem>

        {/* Один RevealItem на всю область вкладок: переключение таба не должно переигрывать анимацию. */}
        <RevealItem>
        {tab === 'general' && (
          <div className="space-y-4 pt-1">
            <Card>
              {/* Шапка с именем работает заголовком карточки — отдельный CardTitle не нужен. */}
              <CardContent className="divide-y divide-border pt-4 sm:pt-6">
                <ProfileIdentityHeader identity={identity} />
                <ProfileRow
                  label={t('profile.telegramId')}
                  value={
                    user?.has_telegram_link && user.telegram_id != null
                      ? String(user.telegram_id)
                      : '—'
                  }
                  hint={!user?.has_telegram_link ? t('profile.telegramIdUnlinkedHint') : undefined}
                />
                <ProfileRow label={t('profile.email')} value={emailDisplay} />
                <ProfileRow
                  label={t('profile.registeredAt')}
                  value={user?.registered_at ? formatDate(user.registered_at, lang) : '—'}
                />
                {user?.customer_id != null && user.customer_id > 0 && (
                  <div className="flex items-start justify-between gap-3 py-3">
                    <div className="min-w-0">
                      <p className="text-sm text-muted-foreground">{t('profile.broadcastOptOut')}</p>
                      <p className="mt-1 text-xs text-muted-foreground">{t('profile.broadcastOptOutHint')}</p>
                    </div>
                    <button
                      type="button"
                      role="switch"
                      aria-checked={Boolean(user.broadcast_opt_out)}
                      aria-label={t('profile.broadcastOptOut')}
                      disabled={broadcastSaving}
                      onClick={() => void toggleBroadcast()}
                      className={cn(
                        'relative mt-0.5 h-6 w-11 shrink-0 rounded-full transition-colors disabled:opacity-60',
                        user.broadcast_opt_out ? 'bg-primary' : 'bg-muted',
                      )}
                    >
                      <span
                        className={cn(
                          'absolute top-0.5 size-5 rounded-full bg-white shadow transition-transform',
                          user.broadcast_opt_out ? 'translate-x-5' : 'translate-x-0.5',
                        )}
                      />
                    </button>
                  </div>
                )}
              </CardContent>
            </Card>

            {pulseLinkedAccounts ? (
              <Link to="/accounts" className="connect-device-cta group block">
                <div className="connect-device-cta-inner profile-linked-accounts-inner flex items-center gap-3 px-4 py-3.5 text-card-foreground">
                  <span className="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
                    <Link2 size={16} aria-hidden />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="text-[16px] font-medium leading-snug sm:text-[18px]">{t('profile.linkedAccountsTitle')}</p>
                    <p className="mt-0.5 text-sm text-muted-foreground">{t('profile.linkedAccountsHint')}</p>
                  </div>
                  <ChevronRight className="size-5 shrink-0 text-muted-foreground" aria-hidden />
                </div>
              </Link>
            ) : (
              <Link to="/accounts" className="block">
                <Card>
                  <CardContent className="flex items-center gap-3 py-3.5">
                    <span className="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
                      <Link2 size={16} aria-hidden />
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="text-[16px] font-medium leading-snug sm:text-[18px]">{t('profile.linkedAccountsTitle')}</p>
                      <p className="mt-0.5 text-sm text-muted-foreground">{t('profile.linkedAccountsHint')}</p>
                    </div>
                    <ChevronRight className="size-5 shrink-0 text-muted-foreground" aria-hidden />
                  </CardContent>
                </Card>
              </Link>
            )}

            {user?.has_password ? (
              <ChangePasswordCollapsible
                onSuccess={async (token) => {
                  useAuthStore.getState().setToken(token)
                  await fetchMe()
                }}
              />
            ) : (
              <Card>
                <CardContent className="py-3 text-sm text-muted-foreground">
                  {t('settings.password.noPasswordHint')}
                </CardContent>
              </Card>
            )}

            {user?.can_delete_account_ui ? <DeleteAccountSection /> : null}
          </div>
        )}

        {tab === 'bonuses' && (
          <div className="space-y-4 pt-1">
            <Card>
              <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0 pb-2">
                <div className="min-w-0">
                  <CardTitle className="text-base">{t('profile.referralBlockTitle')}</CardTitle>
                  <CardDescription className="text-xs">{t('profile.referralBlockHint')}</CardDescription>
                </div>
                <Link
                  to="/referral"
                  className="inline-flex shrink-0 items-center gap-1 rounded-lg border border-border bg-card/70 px-2.5 py-1.5 text-xs font-medium text-primary shadow-sm"
                >
                  {t('profile.referralProgramLink')}
                  <ArrowRight className="size-3.5" strokeWidth={2} aria-hidden />
                </Link>
              </CardHeader>
              <CardContent className="space-y-4 pt-0">
                {referralsPending && !hasReferralLinks ? (
                  <div className="space-y-4" aria-hidden>
                    <Skeleton className="h-16 w-full rounded-lg" />
                    <Skeleton className="h-16 w-full rounded-lg" />
                  </div>
                ) : hasReferralLinks ? (
                  <>
                    {botRefUrl ? (
                      <ReferralCopyRow
                        label={t('referralPage.linkBot')}
                        value={botRefUrl}
                        onShare={shareRef}
                      />
                    ) : null}
                    {cabinetRefUrl ? (
                      <ReferralCopyRow
                        label={t('referralPage.linkCabinet')}
                        value={cabinetRefUrl}
                        onShare={shareRef}
                      />
                    ) : null}
                    <p className="text-[11px] leading-snug text-muted-foreground">{t('profile.referralFootnote')}</p>
                  </>
                ) : (
                  <p className="text-sm text-muted-foreground">{t('profile.referralNeedTelegram')}</p>
                )}
              </CardContent>
            </Card>

            <ProfileLoyaltySection />

            <Link
              to="/promocodes"
              className="subscription-feature-card cabinet-accent-violet cabinet-row flex w-full items-center gap-3 p-4 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <div className="cabinet-icon-box flex h-9 w-9 shrink-0 items-center justify-center rounded-xl">
                <Gift size={16} strokeWidth={1.75} aria-hidden />
              </div>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium text-foreground">{t('profile.promocodesCardTitle')}</p>
                <p className="mt-1 text-xs text-muted-foreground">{t('profile.promocodesCardHint')}</p>
              </div>
              <ChevronRight className="cabinet-row-chevron size-5 shrink-0 text-muted-foreground" aria-hidden />
            </Link>
          </div>
        )}

        {tab === 'history' && (
          <div className="pt-1">
            <PaymentsHistoryCard />
          </div>
        )}
        </RevealItem>
      </PageReveal>
    </AppLayout>
  )
}

function ProfileRow({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="space-y-1 py-3 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-sm text-muted-foreground">{label}</span>
        <span className="text-right text-sm font-medium break-all">{value}</span>
      </div>
      {hint ? <p className="text-xs text-muted-foreground sm:text-left text-right">{hint}</p> : null}
    </div>
  )
}

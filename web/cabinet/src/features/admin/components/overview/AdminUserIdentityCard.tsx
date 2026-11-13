import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, CircleAlert, CircleCheck, Copy, Share2 } from 'lucide-react'

import { Card } from '@/components/ui/card'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { cn } from '@/lib/utils'
import { formatNumber } from '@/lib/format'
import type { AdminCustomerDTO } from '@/lib/types/admin'
import { accountStatusBadgeClasses } from '../../utils/accountStatus'
import type { UnifiedAccountStatus } from '../../utils/accountStatus'
import { formatAdminDateTime } from '../../utils/datetime'
import { AdminUserAvatar } from './AdminUserAvatar'

interface Props {
  user: AdminCustomerDTO
  displayName: string
  status: UnifiedAccountStatus
  /** Бейдж тарифа: кликабельный, когда режим продаж это позволяет. */
  tariffBadge?: ReactNode
  hasRwUser: boolean
  /** Итог по платежам — та же цифра, что в карточке «Платежи» ниже. */
  payments?: { rubSum: number; count: number } | null
  dateLocale: string
  /**
   * Копирование ссылки. На телефоне колонки действий нет, поэтому кнопка
   * живёт прямо в шапке карточки — как в макете.
   */
  copy: { available: boolean; copied: boolean; copy: () => void }
}

/**
 * Колонка личности: кто этот человек и в каком он состоянии.
 *
 * Отделена от данных подписки намеренно: на широком экране колонка липнет к
 * верху и не уезжает, пока админ листает платежи, — «на кого я смотрю»
 * остаётся перед глазами.
 */
export function AdminUserIdentityCard({
  user,
  displayName,
  status,
  tariffBadge,
  hasRwUser,
  payments,
  dateLocale,
  copy,
}: Props) {
  const { t } = useTranslation()

  return (
    <Card className="cabinet-elevated-card p-4">
      <div className="flex items-center gap-3">
        <AdminUserAvatar url={user.avatar_url} name={displayName} tone={status.tone} />
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-lg font-semibold leading-tight">{displayName}</h1>
          <p className="mt-0.5 truncate text-xs tabular-nums text-muted-foreground">
            {/* У веб-клиента telegram_id подставной (70000…) — админу он ничего не говорит. */}
            {user.is_web_only
              ? t('admin.users.overview.idLineWeb', { lang: user.language.toUpperCase() })
              : t('admin.users.overview.idLine', {
                  id: user.telegram_id,
                  lang: user.language.toUpperCase(),
                })}
          </p>
        </div>
        {copy.available && (
          <button
            type="button"
            onClick={copy.copy}
            aria-label={t('admin.users.copySubscriptionLink')}
            title={t('admin.users.copySubscriptionLink')}
            className="inline-flex size-9 shrink-0 items-center justify-center rounded-btn border border-border bg-secondary text-muted-foreground transition-colors hover:bg-accent hover:text-foreground lg:hidden"
          >
            {copy.copied ? (
              <Check className="size-4 text-emerald-600 dark:text-emerald-400" />
            ) : (
              <Share2 className="size-4" />
            )}
          </button>
        )}
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <span className={accountStatusBadgeClasses(status.tone)}>{t(status.labelKey)}</span>
        {tariffBadge}
      </div>

      {!hasRwUser && (
        <p className="mt-3 rounded-lg border border-dashed border-border px-3 py-2 text-xs text-muted-foreground">
          {t('admin.users.subscription.noRwUser')}
        </p>
      )}

      <dl className="mt-3 space-y-1.5 border-t border-border/60 pt-3 text-xs">
        <CabinetRows cabinet={user.cabinet} />
        <div className="flex items-center justify-between gap-3">
          <dt className="text-muted-foreground">{t('admin.users.overview.customerSince')}</dt>
          <dd className="tabular-nums">{formatAdminDateTime(user.created_at, dateLocale)}</dd>
        </div>
        {payments && (
          <div className="flex items-center justify-between gap-3">
            <dt className="text-muted-foreground">{t('admin.users.overview.paidTotal')}</dt>
            <dd className={cn('tabular-nums', payments.count === 0 && 'text-muted-foreground')}>
              {t('admin.users.overview.paidTotalValue', {
                sum: formatNumber(payments.rubSum),
                count: payments.count,
              })}
            </dd>
          </div>
        )}
      </dl>
    </Card>
  )
}

/** Названия сервисов не переводим; «Почта» — через i18n. */
const PROVIDER_NAMES: Record<string, string> = {
  telegram: 'Telegram',
  google: 'Google',
  yandex: 'Yandex',
  vk: 'VK',
}

/**
 * Почта, её подтверждение и способы входа.
 *
 * Подтверждение здесь важнее, чем в профиле клиента: клиент попадает в
 * админку сразу после регистрации, ещё до кода из письма, а без кода войти
 * не может. «Не подтверждена» — ответ на «не могу зайти» в поддержке.
 */
function CabinetRows({ cabinet }: { cabinet: AdminCustomerDTO['cabinet'] }) {
  const { t } = useTranslation()
  const { state, copy } = useCopyToClipboard()

  if (!cabinet) {
    return (
      <div className="flex items-center justify-between gap-3">
        <dt className="text-muted-foreground">{t('admin.users.overview.loginVia')}</dt>
        <dd className="text-muted-foreground">{t('admin.users.overview.loginBotOnly')}</dd>
      </div>
    )
  }

  const email = cabinet.email
  const providers = cabinet.providers
    .map((p) => (p === 'email' ? t('admin.users.overview.loginEmail') : PROVIDER_NAMES[p] ?? p))
    .join(' · ')

  return (
    <>
      {/*
        Колонка узкая (270px): статус встаёт в строку с подписью «Почта», а сам
        адрес — отдельной строкой во всю ширину, иначе длинные адреса режутся
        до первых букв.
      */}
      <div>
        <div className="flex items-center justify-between gap-3">
          <dt className="text-muted-foreground">{t('admin.users.overview.email')}</dt>
          {email ? (
            <dd
              className={cn(
                'inline-flex items-center gap-1 whitespace-nowrap',
                cabinet.email_verified
                  ? 'text-emerald-600 dark:text-emerald-400'
                  : 'text-amber-600 dark:text-amber-400',
              )}
            >
              {cabinet.email_verified ? (
                <CircleCheck className="size-3.5" aria-hidden />
              ) : (
                <CircleAlert className="size-3.5" aria-hidden />
              )}
              {t(
                cabinet.email_verified
                  ? 'admin.users.overview.emailVerified'
                  : 'admin.users.overview.emailUnverified',
              )}
            </dd>
          ) : (
            <dd className="text-muted-foreground">—</dd>
          )}
        </div>
        {email && (
          <dd className="mt-1 flex min-w-0 items-center gap-1">
            <span className="min-w-0 truncate text-sm" title={email}>
              {email}
            </span>
            <button
              type="button"
              onClick={() => void copy(email)}
              aria-label={t('admin.users.overview.copyEmail')}
              title={t('admin.users.overview.copyEmail')}
              className="-my-1 inline-flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            >
              {state === 'done' ? (
                <Check className="size-3.5 text-emerald-600 dark:text-emerald-400" />
              ) : (
                <Copy className="size-3.5" />
              )}
            </button>
          </dd>
        )}
      </div>
      <div className="flex items-start justify-between gap-3">
        <dt className="shrink-0 text-muted-foreground">{t('admin.users.overview.loginVia')}</dt>
        <dd className={cn('min-w-0 text-right', !providers && 'text-muted-foreground')}>
          {providers || '—'}
        </dd>
      </div>
    </>
  )
}

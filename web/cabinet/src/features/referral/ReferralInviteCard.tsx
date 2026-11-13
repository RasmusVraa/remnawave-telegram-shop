import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Gift, Globe, Send, type LucideIcon } from 'lucide-react'

import { QrCode } from '@/components/QrCode'
import { Card, CardContent } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import { hapticSelection } from '@/lib/haptic'

import { ReferralCopyRow } from './ReferralCopyRow'

export interface ReferralLink {
  key: 'bot' | 'cabinet'
  label: string
  url: string
}

/** Иконка канала: самолётик — бот, глобус — веб-кабинет. */
const LINK_ICON: Record<ReferralLink['key'], LucideIcon> = {
  bot: Send,
  cabinet: Globe,
}

/**
 * Приглашение: QR, переключатель канала и одна строка ссылки.
 *
 * Задача страницы ровно одна — чтобы ссылкой поделились, поэтому карточка
 * стоит выше списков и сведена к четырём элементам.
 *
 * QR решает офлайн-сценарий, ради которого рефералкой и пользуются, — показать
 * телефон другу за столом. Ссылку в этот момент диктовать невозможно.
 *
 * Ссылок у пользователя две — в бота и на регистрацию в кабинете, — и они
 * ведут на один аккаунт. Показывать два QR рядом нельзя: это ровно тот вопрос
 * «а какой из них правильный», и на телефоне оба кода получились бы вдвое
 * мельче. Поэтому код один, а переключатель меняет и его, и строку, и кнопку
 * «Поделиться». Если ссылка настроена только одна, переключателя нет вовсе.
 *
 * Переключатель — иконка плюс подпись во всю ширину, а не два мелких таба:
 * «Телеграм» и «Кабинет» словами одинаковой длины и в одинаковом сером
 * различались только текстом, и в них приходилось вчитываться. Подпись при
 * иконке осталась: самолётик и глобус без слов угадывают не все.
 *
 * Отсюда убраны подпись «Обе ведут на ваш аккаунт» и лейбл над ссылкой — они
 * пересказывали то, что и так видно. Заголовок остался: без него карточка
 * читается как «какой-то код», и что с ним делать, приходится догадываться.
 */
export function ReferralInviteCard({
  links,
  refereeDays,
  onShare,
}: {
  links: ReferralLink[]
  /** Сколько дней получит приглашённый. 0 — в этом режиме подарка нет. */
  refereeDays: number
  onShare: (url: string) => Promise<boolean>
}) {
  const { t } = useTranslation()
  const [activeKey, setActiveKey] = useState<ReferralLink['key']>(links[0]?.key ?? 'bot')

  const active = links.find((l) => l.key === activeKey) ?? links[0]
  if (!active) return null

  const hasSwitch = links.length > 1

  return (
    <Card className="overflow-hidden border-primary/15 bg-gradient-to-br from-card via-card to-primary/5">
      <CardContent className="pt-4 sm:pt-6">
        {/*
         * На телефоне — стопка: заголовок, выбор канала, потом код, который от
         * него зависит. На sm+ QR уезжает в левую колонку и занимает все её
         * строки, поэтому управление стоит рядом с ним, а не под ним.
         *
         * Строки расставлены явно (row-start), а не автоматически: без этого
         * порядок в DOM, нужный телефону, ломал бы двухколоночную сетку.
         */}
        <div className="flex flex-col gap-4 sm:grid sm:grid-cols-[auto_minmax(0,1fr)] sm:items-center sm:gap-x-6 sm:gap-y-4">
          <h2 className="order-1 text-balance text-base font-bold tracking-tight sm:order-none sm:col-start-2 sm:row-start-1 sm:text-xl">
            {t('referralPage.invite.title')}
          </h2>

          {hasSwitch ? (
            <div
              role="group"
              aria-label={t('referralPage.invite.switchLabel')}
              className="order-2 flex gap-1 rounded-2xl border border-border bg-secondary p-1 sm:order-none sm:col-start-2 sm:row-start-2"
            >
              {links.map((link) => {
                const Icon = LINK_ICON[link.key]
                const isActive = link.key === active.key
                return (
                  <button
                    key={link.key}
                    type="button"
                    aria-pressed={isActive}
                    onClick={() => {
                      if (!isActive) hapticSelection()
                      setActiveKey(link.key)
                    }}
                    className={cn(
                      'flex h-11 flex-1 items-center justify-center gap-2 rounded-xl text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                      isActive
                        ? 'bg-card font-semibold text-foreground shadow-sm'
                        : 'text-muted-foreground hover:text-foreground',
                    )}
                  >
                    <Icon size={17} />
                    {link.label}
                  </button>
                )
              })}
            </div>
          ) : null}

          {/* Белое поле вокруг кода — требование читаемости, а не украшение:
              QR сканируют с чужого телефона под углом и при бликах. */}
          {/* Без внутреннего отступа: тихая зона по краям есть у самого кода,
              и внешний padding только удваивал белое поле. */}
          <div
            className={cn(
              'order-3 mx-auto w-full max-w-[196px] overflow-hidden rounded-xl bg-white shadow-sm sm:order-none sm:col-start-1 sm:row-start-1 sm:mx-0 sm:max-w-[168px] sm:self-center',
              hasSwitch ? 'sm:row-span-3' : 'sm:row-span-2',
            )}
          >
            <QrCode
              value={active.url}
              size={196}
              className="h-auto w-full"
              title={t('referralPage.invite.qrAlt')}
            />
          </div>

          <div
            className={cn(
              'order-4 flex min-w-0 flex-col gap-3 sm:order-none sm:col-start-2 sm:self-start',
              // Без переключателя строки на одну меньше, и подвал колонки
              // поднимается на его место.
              hasSwitch ? 'sm:row-start-3' : 'sm:row-start-2',
            )}
          >
            {refereeDays > 0 ? (
              <span className="inline-flex items-center gap-1.5 self-center rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2.5 py-1 text-xs font-semibold text-emerald-600 sm:self-start dark:text-emerald-400">
                <Gift size={13} />
                {t('referralPage.invite.refereeBadge', { count: refereeDays })}
              </span>
            ) : null}

            <ReferralCopyRow
              compact
              label={t('referralPage.invite.linkLabel')}
              value={active.url}
              onShare={onShare}
            />
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

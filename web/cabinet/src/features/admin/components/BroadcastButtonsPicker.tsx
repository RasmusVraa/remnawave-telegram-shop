import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, ChevronDown, ExternalLink, LayoutGrid } from 'lucide-react'

import { cn } from '@/lib/utils'
import { surface } from './Surface'
import {
  BROADCAST_LINK_KEYS,
  CABINET_LINK_KEYS,
  EXTERNAL_LINK_KEYS,
  broadcastLinkLabelKey,
  type BroadcastLinkKey,
} from '../utils/broadcastLinks'

/** Кнопки-действия бота: остаются в чате, кабинет не открывают. */
type BotActionKey = 'buy' | 'promo' | 'share_referral' | 'main_menu'

export interface BroadcastButtonsState {
  buy: boolean
  connect: boolean
  promo: boolean
  main_menu: boolean
  /** «Пригласить друга» — у каждого получателя своя реферальная ссылка. */
  share_referral: boolean
  links: BroadcastLinkKey[]
}

const BOT_ACTIONS: { key: BotActionKey; labelKey: string }[] = [
  { key: 'buy', labelKey: 'admin.broadcast.buttons.buy' },
  { key: 'promo', labelKey: 'admin.broadcast.buttons.promo' },
  { key: 'share_referral', labelKey: 'admin.broadcast.buttons.shareReferral' },
  { key: 'main_menu', labelKey: 'admin.broadcast.buttons.mainMenu' },
]

/**
 * Кнопки под сообщением, разложенные по тому, куда они ведут.
 *
 * Деление именно по этому признаку: раньше «Мой VPN» лежал рядом с «Купить»,
 * хотя открывает кабинет мини-приложением ровно как разделы, и по названию
 * группы нельзя было понять, уйдёт получатель в кабинет или останется в чате.
 *
 * Обе группы свёрнуты по умолчанию и показывают счётчик выбранного: кнопки
 * нужны не в каждой рассылке, а развёрнутыми они занимали пол-экрана.
 */
export function BroadcastButtonsPicker({
  buttons,
  onChange,
}: {
  buttons: BroadcastButtonsState
  onChange: (next: BroadcastButtonsState) => void
}) {
  const { t } = useTranslation()

  // Порядок ключей держим как в BROADCAST_LINK_KEYS: бэкенд всё равно приведёт
  // к своему, и расхождение сбивало бы предпросмотр.
  const toggleLink = (key: BroadcastLinkKey) => {
    const next = buttons.links.includes(key)
      ? buttons.links.filter((item) => item !== key)
      : BROADCAST_LINK_KEYS.filter((item) => item === key || buttons.links.includes(item))
    onChange({ ...buttons, links: [...next] })
  }

  const cabinetCount =
    (buttons.connect ? 1 : 0) + CABINET_LINK_KEYS.filter((k) => buttons.links.includes(k)).length
  const plainCount =
    BOT_ACTIONS.filter((a) => buttons[a.key]).length +
    EXTERNAL_LINK_KEYS.filter((k) => buttons.links.includes(k)).length

  return (
    <div className="space-y-2">
      <Group
        icon={<LayoutGrid className="size-4 shrink-0 text-muted-foreground" />}
        title={t('admin.broadcast.groupCabinetTitle')}
        hint={t('admin.broadcast.groupCabinetHint')}
        selected={cabinetCount}
        total={CABINET_LINK_KEYS.length + 1}
        onClear={() =>
          onChange({
            ...buttons,
            connect: false,
            links: buttons.links.filter((k) => !CABINET_LINK_KEYS.includes(k as never)),
          })
        }
      >
        {/* «Мой VPN» ведёт на главную кабинета — туда же, куда «Главная
            кабинета» из разделов. Стоит первым, чтобы дубль был виден. */}
        <Chip
          active={buttons.connect}
          label={t('admin.broadcast.buttons.connect')}
          onClick={() => onChange({ ...buttons, connect: !buttons.connect })}
        />
        {CABINET_LINK_KEYS.map((key) => (
          <Chip
            key={key}
            active={buttons.links.includes(key)}
            label={t(broadcastLinkLabelKey(key))}
            onClick={() => toggleLink(key)}
          />
        ))}
      </Group>

      <Group
        icon={<ExternalLink className="size-4 shrink-0 text-muted-foreground" />}
        title={t('admin.broadcast.groupPlainTitle')}
        hint={t('admin.broadcast.groupPlainHint')}
        selected={plainCount}
        total={BOT_ACTIONS.length + EXTERNAL_LINK_KEYS.length}
        onClear={() =>
          onChange({
            ...buttons,
            buy: false,
            promo: false,
            share_referral: false,
            main_menu: false,
            links: buttons.links.filter((k) => !EXTERNAL_LINK_KEYS.includes(k as never)),
          })
        }
      >
        {BOT_ACTIONS.map((action) => (
          <Chip
            key={action.key}
            active={buttons[action.key]}
            label={t(action.labelKey)}
            onClick={() => onChange({ ...buttons, [action.key]: !buttons[action.key] })}
          />
        ))}
        {EXTERNAL_LINK_KEYS.map((key) => (
          <Chip
            key={key}
            active={buttons.links.includes(key)}
            label={t(broadcastLinkLabelKey(key))}
            onClick={() => toggleLink(key)}
          />
        ))}
      </Group>

      <p className="px-1 text-[11px] leading-relaxed text-muted-foreground">
        {t('admin.broadcast.linksConfigHint')}
      </p>
    </div>
  )
}

function Group({
  icon,
  title,
  hint,
  selected,
  total,
  onClear,
  children,
}: {
  icon: ReactNode
  title: string
  hint: string
  selected: number
  total: number
  onClear: () => void
  children: ReactNode
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <div className={surface('raised', 'overflow-hidden rounded-lg')}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="flex min-h-12 w-full items-center gap-2.5 px-3 py-2.5 text-left transition-colors hover:bg-accent/40"
      >
        {icon}
        <span className="block min-w-0 flex-1">
          <span className="block text-sm font-medium">{title}</span>
          <span className="block truncate text-xs text-muted-foreground">{hint}</span>
        </span>
        <span
          className={cn(
            'ml-auto shrink-0 rounded-full px-2 py-0.5 text-xs tabular-nums',
            selected > 0 ? 'bg-primary/15 font-semibold text-primary' : 'bg-muted text-muted-foreground',
          )}
        >
          {selected} / {total}
        </span>
        <ChevronDown
          className={cn('size-4 shrink-0 text-muted-foreground transition-transform', open && 'rotate-180')}
        />
      </button>

      {open && (
        <div className="space-y-2.5 border-t border-border/50 p-3">
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">{children}</div>
          {selected > 0 && (
            <button
              type="button"
              onClick={onClear}
              className="text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
            >
              {t('admin.broadcast.linksClear')}
            </button>
          )}
        </div>
      )}
    </div>
  )
}

function Chip({ active, label, onClick }: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        'flex items-center gap-2 rounded-lg border px-2.5 py-2 text-left text-sm transition-colors',
        active
          ? 'border-primary/45 bg-primary/10 text-foreground'
          : 'border-border/60 bg-muted/40 text-muted-foreground hover:bg-accent/60 hover:text-foreground',
      )}
    >
      <span className="min-w-0 flex-1 truncate">{label}</span>
      <Check className={cn('size-3.5 shrink-0 text-primary', !active && 'opacity-0')} />
    </button>
  )
}

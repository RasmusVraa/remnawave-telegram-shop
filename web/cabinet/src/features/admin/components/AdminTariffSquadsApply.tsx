import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, Loader2, Server, Users } from 'lucide-react'

import { cn } from '@/lib/utils'
import { surface } from './Surface'
import { AdminModal } from './AdminModal'
import {
  adminSectionIconBoxClass,
  adminSectionIconAccentClassNames,
} from '../utils/adminSectionIconAccents'
import {
  useAdminTariffSquads,
  useAdminTariffSquadsApply,
  type AdminSquadItem,
} from '../hooks/useAdminTariffs'

/**
 * Режим применения к тем, кто уже на тарифе.
 *
 * Разница между режимами ровно одна — снимать ли лишнее, — и по названию
 * кнопки она не читается. Поэтому кнопка одна, а режим выбирается в окне,
 * где под каждым вариантом стоит строка про последствия.
 */
type ApplyMode = 'add' | 'strict'

function squadName(uuid: string, squads: AdminSquadItem[]): string {
  return squads.find((s) => s.uuid === uuid)?.name ?? uuid
}

/** Чипы серверов: зелёные — что появится, розовые — что снимется. */
function SquadChips({
  uuids,
  squads,
  tone,
}: {
  uuids: string[]
  squads: AdminSquadItem[]
  /** neutral — просто перечисление, без обещания что-то выдать или снять. */
  tone: 'add' | 'remove' | 'neutral'
}) {
  if (uuids.length === 0) return null
  return (
    <div className="flex flex-wrap gap-1.5">
      {uuids.map((uuid) => (
        <span
          key={uuid}
          className={cn(
            'rounded-md px-2 py-0.5 text-xs leading-5',
            tone === 'add' && 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
            tone === 'remove' && 'bg-rose-500/15 text-rose-600 dark:text-rose-400',
            tone === 'neutral' && 'bg-muted text-muted-foreground',
          )}
        >
          {tone === 'add' ? '+ ' : tone === 'remove' ? '− ' : ''}
          {squadName(uuid, squads)}
        </span>
      ))}
    </div>
  )
}

/** Строка «что будет» с подписью — из неё и складывается выбор в окнах. */
function ChoiceCard({
  selected,
  onSelect,
  title,
  description,
  children,
}: {
  selected: boolean
  onSelect: () => void
  title: string
  description: string
  children?: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className={cn(
        'flex w-full gap-3 rounded-xl px-3 py-3 text-left transition-colors',
        selected ? 'border border-primary/60 bg-primary/10' : surface('raised', 'hover:bg-accent/40'),
      )}
    >
      <span
        className={cn(
          'mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border transition-colors',
          selected ? 'border-primary bg-primary text-primary-foreground' : 'border-border',
        )}
      >
        {selected && <Check className="size-3" strokeWidth={3} />}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium">{title}</span>
        <span className="mt-0.5 block text-xs leading-relaxed text-muted-foreground">{description}</span>
        {children}
      </span>
    </button>
  )
}

/** Итог прогона: только ненулевые цифры, чтобы не читать «ошибок: 0». */
function RunStats({
  changed,
  already,
  notFound,
  failed,
}: {
  changed: number
  already: number
  notFound: number
  failed: number
}) {
  const { t } = useTranslation()
  const items: { label: string; value: number; tone?: 'warn' }[] = [
    { label: t('admin.tariffs.squadsApply.statChanged'), value: changed },
    { label: t('admin.tariffs.squadsApply.statAlready'), value: already },
  ]
  if (notFound > 0) items.push({ label: t('admin.tariffs.squadsApply.statNotFound'), value: notFound, tone: 'warn' })
  if (failed > 0) items.push({ label: t('admin.tariffs.squadsApply.statFailed'), value: failed, tone: 'warn' })

  return (
    <div className="flex flex-wrap gap-x-4 gap-y-1">
      {items.map((it) => (
        <span key={it.label} className="text-xs">
          <span className={cn('font-semibold tabular-nums', it.tone === 'warn' && 'text-amber-600 dark:text-amber-400')}>
            {it.value}
          </span>{' '}
          <span className="text-muted-foreground">{it.label}</span>
        </span>
      ))}
    </div>
  )
}

interface ApplyPanelProps {
  tariffId: number
  squads: AdminSquadItem[]
  /** Форма изменена: применять сохранённый список в этот момент — обманывать админа. */
  dirty: boolean
}

/**
 * Блок «серверы у тех, кто уже на тарифе» во вкладке «Серверы».
 *
 * Отдельная кнопка, а не только вопрос при сохранении: если применение упало
 * на середине (панель отвалилась), состояние чинится повторным запуском, а не
 * ожиданием, пока все продлятся.
 */
export function AdminTariffSquadsApplyPanel({ tariffId, squads, dirty }: ApplyPanelProps) {
  const { t } = useTranslation()
  const [modalOpen, setModalOpen] = useState(false)
  const [mode, setMode] = useState<ApplyMode>('add')

  const { data } = useAdminTariffSquads(tariffId, true)
  const apply = useAdminTariffSquadsApply()
  const run = data?.run ?? null
  const running = run?.status === 'running'
  const tariffSquads = data?.tariff_squads ?? []
  const strictRemove = squads.map((s) => s.uuid).filter((u) => !tariffSquads.includes(u))
  const { iconClassName } = adminSectionIconAccentClassNames('indigo')

  const busy = running || apply.isPending
  const disabled = dirty || busy || tariffSquads.length === 0
  const progress = run && run.total > 0 ? Math.round((run.processed / run.total) * 100) : 0

  const submit = () => {
    apply.mutate({
      id: tariffId,
      add: tariffSquads,
      remove: mode === 'strict' ? strictRemove : [],
    })
    setModalOpen(false)
  }

  return (
    <div className={surface('raised', 'mt-4 rounded-xl p-4')}>
      <div className="flex items-start gap-3">
        <span className={adminSectionIconBoxClass('indigo', 'size-8 shrink-0')}>
          <Users className={cn('size-4', iconClassName)} />
        </span>
        <div className="min-w-0 flex-1">
          <h4 className="text-sm font-semibold leading-tight">{t('admin.tariffs.squadsApply.title')}</h4>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {data
              ? data.active_count > 0
                ? t('admin.tariffs.squadsApply.activeCount', { count: data.active_count })
                : t('admin.tariffs.squadsApply.noActive')
              : t('admin.loading')}
          </p>
        </div>
      </div>

      <button
        type="button"
        disabled={disabled}
        onClick={() => setModalOpen(true)}
        className={cn(
          'mt-3 inline-flex w-full items-center justify-center gap-2 rounded-lg border border-border bg-background/40 px-4 py-2 text-sm font-medium transition-colors hover:bg-accent hover:text-foreground disabled:opacity-50 sm:w-auto',
          busy && '!opacity-100',
        )}
      >
        {busy ? <Loader2 className="size-4 animate-spin" /> : <Server className="size-4" />}
        {t('admin.tariffs.squadsApply.openButton')}
      </button>

      {dirty && (
        <p className="mt-2 text-xs text-amber-600 dark:text-amber-400">
          {t('admin.tariffs.squadsApply.dirtyHint')}
        </p>
      )}

      {apply.isError && (
        <p className="mt-2 text-xs text-rose-600 dark:text-rose-400">{(apply.error as Error).message}</p>
      )}

      {run && (
        <div className="mt-3 space-y-2 border-t border-border/50 pt-3">
          {running ? (
            <>
              <div className="flex items-baseline justify-between gap-2 text-xs">
                <span className="text-muted-foreground">{t('admin.tariffs.squadsApply.running')}</span>
                <span className="font-semibold tabular-nums">
                  {run.processed} / {run.total}
                </span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                <div
                  className="h-full rounded-full bg-primary transition-[width] duration-500"
                  style={{ width: `${Math.max(progress, 3)}%` }}
                />
              </div>
            </>
          ) : run.status === 'failed' ? (
            <p className="text-xs text-rose-600 dark:text-rose-400">
              {t('admin.tariffs.squadsApply.runFailed', { error: run.error ?? '' })}
            </p>
          ) : (
            <RunStats
              changed={run.changed}
              already={run.already_ok}
              notFound={run.not_found}
              failed={run.failed}
            />
          )}
        </div>
      )}

      <AdminModal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        title={t('admin.tariffs.squadsApply.modalTitle')}
        description={t('admin.tariffs.squadsApply.modalSubtitle', { count: data?.active_count ?? 0 })}
        icon={Users}
        iconAccent="indigo"
        size="md"
        footer={
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={() => setModalOpen(false)}
              className="rounded-lg border border-border px-4 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              {t('admin.cancel')}
            </button>
            <button
              type="button"
              onClick={submit}
              className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90"
            >
              {t('admin.tariffs.squadsApply.confirmButton')}
            </button>
          </div>
        }
      >
        <div className="space-y-2">
          <ChoiceCard
            selected={mode === 'add'}
            onSelect={() => setMode('add')}
            title={t('admin.tariffs.squadsApply.modeAdd')}
            description={t('admin.tariffs.squadsApply.modeAddHint')}
          />
          <ChoiceCard
            selected={mode === 'strict'}
            onSelect={() => setMode('strict')}
            title={t('admin.tariffs.squadsApply.modeStrict')}
            description={t('admin.tariffs.squadsApply.modeStrictHint')}
          >
            {strictRemove.length > 0 && (
              <span className="mt-2 block">
                <span className="mb-1 block text-[11px] uppercase tracking-wide text-muted-foreground">
                  {t('admin.tariffs.squadsApply.willRemove')}
                </span>
                <SquadChips uuids={strictRemove} squads={squads} tone="remove" />
              </span>
            )}
          </ChoiceCard>

          <div className="pt-1">
            <span className="mb-1 block text-[11px] uppercase tracking-wide text-muted-foreground">
              {t('admin.tariffs.squadsApply.tariffServers')}
            </span>
            <SquadChips uuids={tariffSquads} squads={squads} tone="neutral" />
          </div>
        </div>
      </AdminModal>
    </div>
  )
}

interface SaveDialogProps {
  open: boolean
  onClose: () => void
  /** applyNow=false — тариф сохраняется, панель не трогаем. */
  onSubmit: (applyNow: boolean) => void
  added: string[]
  removed: string[]
  squads: AdminSquadItem[]
  activeCount: number
  loading: boolean
}

/**
 * Вопрос при сохранении тарифа с изменённым списком серверов.
 *
 * Выбор осознанно отдан админу: добавление сервера обычно хотят раздать сразу,
 * а снятие — оставить до конца оплаченного периода. Три равнозначные кнопки
 * («Отмена», «Только новым», «Применить всем») читались как три разных
 * сохранения, поэтому вариант выбирается строкой, а действие одно.
 */
export function AdminTariffSquadsSaveDialog({
  open,
  onClose,
  onSubmit,
  added,
  removed,
  squads,
  activeCount,
  loading,
}: SaveDialogProps) {
  const { t } = useTranslation()
  const [applyNow, setApplyNow] = useState(true)

  // Снятие сервера чаще хотят отложить до продления, добавление — раздать сразу.
  useEffect(() => {
    if (open) setApplyNow(added.length > 0 || removed.length === 0)
  }, [open, added.length, removed.length])

  return (
    <AdminModal
      open={open}
      onClose={onClose}
      title={t('admin.tariffs.squadsApply.saveTitle')}
      icon={Users}
      iconAccent="amber"
      size="md"
      footer={
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            disabled={loading}
            className="rounded-lg border border-border px-4 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
          >
            {t('admin.cancel')}
          </button>
          <button
            type="button"
            onClick={() => onSubmit(applyNow)}
            disabled={loading}
            className={cn(
              'inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-50',
              loading && '!opacity-100',
            )}
          >
            {loading && <Loader2 className="size-4 animate-spin" />}
            {t('admin.save')}
          </button>
        </div>
      }
    >
      <div className="space-y-4">
        <div className="space-y-2">
          {added.length > 0 && (
            <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
              <span className="text-xs text-muted-foreground">{t('admin.tariffs.squadsApply.addedLabel')}</span>
              <SquadChips uuids={added} squads={squads} tone="add" />
            </div>
          )}
          {removed.length > 0 && (
            <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
              <span className="text-xs text-muted-foreground">{t('admin.tariffs.squadsApply.removedLabel')}</span>
              <SquadChips uuids={removed} squads={squads} tone="remove" />
            </div>
          )}
        </div>

        <div>
          <p className="mb-2 text-sm">
            {t('admin.tariffs.squadsApply.saveQuestion', { count: activeCount })}
          </p>
          <div className="space-y-2">
            <ChoiceCard
              selected={applyNow}
              onSelect={() => setApplyNow(true)}
              title={t('admin.tariffs.squadsApply.saveApplyNow')}
              description={t('admin.tariffs.squadsApply.saveApplyNowHint', { count: activeCount })}
            />
            <ChoiceCard
              selected={!applyNow}
              onSelect={() => setApplyNow(false)}
              title={t('admin.tariffs.squadsApply.saveLater')}
              description={t('admin.tariffs.squadsApply.saveLaterHint')}
            />
          </div>
        </div>
      </div>
    </AdminModal>
  )
}

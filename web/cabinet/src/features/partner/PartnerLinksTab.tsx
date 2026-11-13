import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link2, Plus, Archive, ArchiveRestore, Trash2, Pencil } from 'lucide-react'

import { RevealItem } from '@/components/PageReveal'
import { ReferralCopyRow } from '@/features/referral/ReferralCopyRow'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { cn } from '@/lib/utils'
import { api, ApiError, type PartnerAccountDTO, type PartnerLinkDTO } from '@/lib/api'
import { hapticSuccess, useErrorHaptic } from '@/lib/haptic'
import { shareLink } from '@/lib/share'

import { PARTNER_STATE_KEY } from './partnerKeys'
import { formatMoney } from './format'
import { PARTNER_INPUT } from './layout'

/**
 * Партнёрскую ссылку заводят ровно затем, чтобы её распространять, поэтому
 * «Поделиться» здесь такое же, как в партнёрке, — способ выбирает `shareLink`.
 */
function useShareStreamLink() {
  const { t } = useTranslation()
  return (url: string) => shareLink({ text: t('referralPage.shareInviteText'), url })
}

export function PartnerLinksTab({ partner }: { partner: PartnerAccountDTO }) {
  const { t } = useTranslation()
  const share = useShareStreamLink()
  const qc = useQueryClient()
  const [newName, setNewName] = useState('')
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editingName, setEditingName] = useState('')

  const invalidate = () => qc.invalidateQueries({ queryKey: PARTNER_STATE_KEY })

  function reportError(e: unknown) {
    if (e instanceof ApiError) {
      const raw = e.body || ''
      if (raw.includes('links_limit_reached')) return setError(t('partnerPage.links.errors.limit'))
      if (raw.includes('link_has_history')) return setError(t('partnerPage.links.errors.hasHistory'))
      if (raw.includes('link_is_default')) return setError(t('partnerPage.links.errors.isDefault'))
      if (raw.includes('invalid_name')) return setError(t('partnerPage.links.errors.invalidName'))
    }
    setError(t('partnerPage.errors.generic'))
  }

  const create = useMutation({
    mutationFn: () => api.partnerCreateLink(newName.trim()),
    onSuccess: async () => {
      hapticSuccess()
      setNewName('')
      setError(null)
      await invalidate()
    },
    onError: reportError,
  })

  const rename = useMutation({
    mutationFn: (vars: { id: number; name: string }) => api.partnerUpdateLink(vars.id, { name: vars.name }),
    onSuccess: async () => {
      setEditingId(null)
      setError(null)
      await invalidate()
    },
    onError: reportError,
  })

  const archive = useMutation({
    mutationFn: (vars: { id: number; archived: boolean }) =>
      api.partnerUpdateLink(vars.id, { archived: vars.archived }),
    onSuccess: async () => {
      setError(null)
      await invalidate()
    },
    onError: reportError,
  })

  const remove = useMutation({
    mutationFn: (id: number) => api.partnerDeleteLink(id),
    onSuccess: async () => {
      setError(null)
      await invalidate()
    },
    onError: reportError,
  })

  function onCreate(e: FormEvent) {
    e.preventDefault()
    if (!newName.trim()) {
      setError(t('partnerPage.links.errors.invalidName'))
      return
    }
    setError(null)
    create.mutate()
  }

  const mainLink = partner.links.find((l) => l.is_default)
  const streams = partner.links.filter((l) => !l.is_default)
  const limitReached = partner.links_used >= partner.links_limit

  return (
    <>
      {mainLink ? (
        <RevealItem>
          <Card>
            <CardHeader className="flex flex-row items-center justify-between gap-2 pb-2">
              <CardTitle className="flex items-center gap-2 text-base font-medium">
                <Link2 size={18} className="text-primary" />
                {t('partnerPage.links.mainTitle')}
              </CardTitle>
              <Badge variant="secondary">{t('partnerPage.links.default')}</Badge>
            </CardHeader>
            <CardContent className="space-y-3">
              {mainLink.bot_link ? (
                <ReferralCopyRow
                  label={t('partnerPage.links.botLink')}
                  value={mainLink.bot_link}
                  onShare={share}
                />
              ) : null}
              {mainLink.web_link ? (
                <ReferralCopyRow
                  label={t('partnerPage.links.webLink')}
                  value={mainLink.web_link}
                  onShare={share}
                />
              ) : null}
              <p className="text-xs text-muted-foreground">{t('partnerPage.links.mainHint')}</p>
            </CardContent>
          </Card>
        </RevealItem>
      ) : null}

      <RevealItem>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-base font-medium">
              <Plus size={18} className="text-primary" />
              {t('partnerPage.links.newTitle')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <form className="flex flex-col gap-2 sm:flex-row" onSubmit={onCreate}>
              <Input
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                maxLength={64}
                placeholder={t('partnerPage.links.namePlaceholder')}
                disabled={limitReached}
                className={PARTNER_INPUT}
              />
              <Button type="submit" loading={create.isPending} disabled={limitReached} className="shrink-0">
                {t('partnerPage.links.create')}
              </Button>
            </form>
            <p className="mt-2 text-xs text-muted-foreground">
              {t('partnerPage.links.usage', { used: partner.links_used, limit: partner.links_limit })}
            </p>
            {error ? (
              <Alert variant="destructive" className="mt-3">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}
          </CardContent>
        </Card>
      </RevealItem>

      {streams.length === 0 ? (
        <RevealItem>
          <Card>
            <CardContent className="py-6 text-center text-sm text-muted-foreground">
              {t('partnerPage.links.empty')}
            </CardContent>
          </Card>
        </RevealItem>
      ) : (
        streams.map((link) => (
          <RevealItem key={link.id}>
            <StreamCard
              link={link}
              isEditing={editingId === link.id}
              editingName={editingName}
              onEditStart={() => {
                setEditingId(link.id)
                setEditingName(link.name)
              }}
              onEditCancel={() => setEditingId(null)}
              onEditChange={setEditingName}
              onEditSave={() => rename.mutate({ id: link.id, name: editingName.trim() })}
              onArchive={() => archive.mutate({ id: link.id, archived: !link.archived })}
              onDelete={() => remove.mutate(link.id)}
              busy={rename.isPending || archive.isPending || remove.isPending}
              pending={
                rename.isPending && rename.variables?.id === link.id
                  ? 'rename'
                  : archive.isPending && archive.variables?.id === link.id
                    ? 'archive'
                    : remove.isPending && remove.variables === link.id
                      ? 'delete'
                      : null
              }
            />
          </RevealItem>
        ))
      )}
    </>
  )
}

function StreamCard({
  link,
  isEditing,
  editingName,
  onEditStart,
  onEditCancel,
  onEditChange,
  onEditSave,
  onArchive,
  onDelete,
  busy,
  pending,
}: {
  link: PartnerLinkDTO
  isEditing: boolean
  editingName: string
  onEditStart: () => void
  onEditCancel: () => void
  onEditChange: (v: string) => void
  onEditSave: () => void
  onArchive: () => void
  onDelete: () => void
  busy: boolean
  /** Какой запрос по этой ссылке сейчас в работе — на его кнопке спиннер. */
  pending: 'rename' | 'archive' | 'delete' | null
}) {
  const { t } = useTranslation()
  const share = useShareStreamLink()

  return (
    <Card className={cn(link.archived && 'opacity-70')}>
      <CardContent className="space-y-3 pt-5">
        <div className="flex items-start justify-between gap-2">
          {isEditing ? (
            <div className="flex w-full flex-col gap-2 sm:flex-row">
              <Input
                value={editingName}
                onChange={(e) => onEditChange(e.target.value)}
                maxLength={64}
                autoFocus
                className={PARTNER_INPUT}
              />
              <div className="flex gap-2">
                <Button size="sm" onClick={onEditSave} loading={pending === 'rename'} disabled={busy || !editingName.trim()}>
                  {t('partnerPage.links.save')}
                </Button>
                <Button size="sm" variant="secondary" onClick={onEditCancel}>
                  {t('partnerPage.links.cancel')}
                </Button>
              </div>
            </div>
          ) : (
            <>
              <p className="font-medium">{link.name}</p>
              <Badge variant={link.archived ? 'default' : 'success'}>
                {link.archived ? t('partnerPage.links.archived') : t('partnerPage.links.active')}
              </Badge>
            </>
          )}
        </div>

        {link.bot_link ? (
          <ReferralCopyRow
            label={t('partnerPage.links.botLink')}
            value={link.bot_link}
            onShare={share}
          />
        ) : null}
        {link.web_link ? (
          <ReferralCopyRow
            label={t('partnerPage.links.webLink')}
            value={link.web_link}
            onShare={share}
          />
        ) : null}

        <div className="grid grid-cols-3 gap-2 rounded-lg border border-border p-2 text-center">
          <StreamStat label={t('partnerPage.links.statCustomers')} value={String(link.customers)} />
          <StreamStat label={t('partnerPage.links.statPaying')} value={String(link.paying)} />
          <StreamStat label={t('partnerPage.links.statEarned')} value={formatMoney(link.earned)} />
        </div>

        {!isEditing ? (
          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="secondary" className="gap-1" onClick={onEditStart} disabled={busy}>
              <Pencil size={14} />
              {t('partnerPage.links.rename')}
            </Button>
            {link.can_archive ? (
              <Button size="sm" variant="secondary" className="gap-1" onClick={onArchive} loading={pending === 'archive'} disabled={busy}>
                {link.archived ? <ArchiveRestore size={14} /> : <Archive size={14} />}
                {link.archived ? t('partnerPage.links.restore') : t('partnerPage.links.archive')}
              </Button>
            ) : null}
            {/*
              Вариант secondary, а не outline: у outline заливка задана ещё и
              под dark:, и тёмная тема перебивала бы красный фон обратно на
              нейтральный — правило с модификатором сильнее безусловного.
            */}
            {link.can_delete ? (
              <Button
                size="sm"
                variant="secondary"
                className="gap-1 border border-destructive/40 bg-destructive/10 text-destructive hover:bg-destructive/20"
                onClick={onDelete}
                loading={pending === 'delete'}
                disabled={busy}
              >
                <Trash2 size={14} />
                {t('partnerPage.links.delete')}
              </Button>
            ) : null}
          </div>
        ) : null}

        {/* Правило удаления объясняется прямо здесь: иначе исчезнувшая кнопка
            выглядит как баг, а не как защита истории начислений. */}
        {!link.can_delete && !link.archived ? (
          <p className="text-xs text-muted-foreground">{t('partnerPage.links.deleteBlockedHint')}</p>
        ) : null}
      </CardContent>
    </Card>
  )
}

function StreamStat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</p>
      <p className="text-sm font-semibold tabular-nums">{value}</p>
    </div>
  )
}

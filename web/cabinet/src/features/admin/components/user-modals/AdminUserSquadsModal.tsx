import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Layers, Search } from 'lucide-react'

import { AdminModal } from '../AdminModal'
import { AdminModalSaveFooter } from '../AdminModalSaveFooter'
import { AdminCheckbox } from '../AdminCheckbox'
import { cn } from '@/lib/utils'
import { surface } from '../Surface'
import { formatAdminApiError } from '../../utils/formatAdminApiError'
import { useErrorHaptic } from '@/lib/haptic'
import {
  useAdminUserSetSquads,
  type AdminSquadDTO,
  type AdminUserPanelResponse,
} from '../../hooks/useAdminUsers'

function normalizeSquads(squads?: AdminSquadDTO[] | null): AdminSquadDTO[] {
  return squads ?? []
}

interface Props {
  open: boolean
  onClose: () => void
  userId: number
  panel: AdminUserPanelResponse
  onSuccess?: (message: string) => void
  onError?: (message: string) => void
}

export function AdminUserSquadsModal({
  open,
  onClose,
  userId,
  panel,
  onSuccess,
  onError,
}: Props) {
  const { t } = useTranslation()
  const rw = panel.rw!
  const setSquads = useAdminUserSetSquads(userId)

  const initialUuids = normalizeSquads(rw.active_squads).map((s) => s.uuid)
  const [draftSquads, setDraftSquads] = useState<string[]>(initialUuids)
  const [query, setQuery] = useState('')
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)

  useEffect(() => {
    if (open) {
      setDraftSquads(normalizeSquads(rw.active_squads).map((s) => s.uuid))
      setQuery('')
      setError(null)
    }
  }, [open, rw.active_squads])

  // Поиск появляется, только когда список перестаёт читаться одним взглядом.
  const showSearch = panel.available_squads.length > 6
  const visibleSquads = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return panel.available_squads
    return panel.available_squads.filter((sq) => sq.name.toLowerCase().includes(q))
  }, [panel.available_squads, query])

  const toggleSquad = (uuid: string) => {
    setDraftSquads((prev) =>
      prev.includes(uuid) ? prev.filter((u) => u !== uuid) : [...prev, uuid],
    )
  }

  const handleClose = () => {
    setError(null)
    onClose()
  }

  const handleSave = () => {
    setError(null)
    const sorted = [...draftSquads].sort()
    const sortedInitial = [...initialUuids].sort()
    const unchanged = sorted.length === sortedInitial.length && sorted.every((u, i) => u === sortedInitial[i])
    if (unchanged) {
      handleClose()
      return
    }

    setSquads.mutate(draftSquads, {
      onSuccess: () => {
        onSuccess?.(t('admin.feedback.saved'))
        handleClose()
      },
      onError: (e) => {
        const msg = formatAdminApiError(e, t)
        setError(msg)
        onError?.(msg)
      },
    })
  }

  const hasChanges = (() => {
    const sorted = [...draftSquads].sort()
    const sortedInitial = [...initialUuids].sort()
    return sorted.length !== sortedInitial.length || sorted.some((u, i) => u !== sortedInitial[i])
  })()

  return (
    <AdminModal
      open={open}
      onClose={handleClose}
      title={t('admin.users.subscription.squads')}
      description={t('admin.users.subscription.squadsHint')}
      icon={Layers}
      iconAccent="indigo"
      size="md"
      footer={
        <AdminModalSaveFooter
          onCancel={handleClose}
          onSave={handleSave}
          isPending={setSquads.isPending}
          saveDisabled={!hasChanges}
          leading={
            <span className="text-sm text-muted-foreground">
              {t('admin.users.subscription.squadsSelected', {
                count: draftSquads.length,
                total: panel.available_squads.length,
              })}
            </span>
          }
        />
      }
    >
      <div className="space-y-4">
        {error && (
          <p className="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </p>
        )}
        {showSearch && (
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t('admin.users.subscription.squadsSearch')}
              className="admin-input w-full py-2 pe-3 ps-9"
            />
          </div>
        )}
        {panel.available_squads.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('admin.noData')}</p>
        ) : visibleSquads.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('admin.users.searchEmpty')}</p>
        ) : (
          <div className="grid gap-2 sm:grid-cols-2">
            {visibleSquads.map((sq: AdminSquadDTO) => (
              <label
                key={sq.uuid}
                className={cn(
                  'flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors',
                  draftSquads.includes(sq.uuid)
                    ? 'border border-primary/50 bg-primary/10'
                    : surface('raised', 'hover:bg-accent/50'),
                )}
              >
                <AdminCheckbox
                  checked={draftSquads.includes(sq.uuid)}
                  onChange={() => toggleSquad(sq.uuid)}
                  aria-label={sq.name}
                />
                <span className="truncate">{sq.name}</span>
              </label>
            ))}
          </div>
        )}
      </div>
    </AdminModal>
  )
}

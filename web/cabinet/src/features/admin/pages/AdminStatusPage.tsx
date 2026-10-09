import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Activity, ExternalLink, Loader2 } from 'lucide-react'

import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { AdminFeedback } from '../components/AdminFeedback'
import { AdminSettingsGroupEditor } from '../components/AdminSettingsGroupEditor'
import { useAdminSettingsDraft } from '../hooks/useAdminSettingsDraft'
import { ADMIN_STATUS_SETTINGS_GROUP } from '../utils/adminSettingsGroups'
import { AdminStatusTargets } from '../components/AdminStatusTargets'

/** Публичная страница /status: зонды, карта и подписи. Те же настройки, что в реестре бота. */
export default function AdminStatusPage() {
  const { t } = useTranslation()
  const {
    groups,
    isLoading,
    isError,
    draft,
    setDraftValue,
    togglingKey,
    isGroupSaving,
    handleToggle,
    handleInstantEnum,
    handleSaveSection,
    feedback,
    clearFeedback,
  } = useAdminSettingsDraft()

  const [expanded, setExpanded] = useState(true)
  const group = (groups ?? []).find((g) => g.id === ADMIN_STATUS_SETTINGS_GROUP)

  return (
    <AdminLayout>
      <div className="space-y-6">
        <AdminPageHeader
          icon={Activity}
          title={t('admin.statusPage.title')}
          subtitle={t('admin.statusPage.subtitle')}
          accent="cyan"
          actions={
            <a
              href="/status"
              target="_blank"
              rel="noreferrer"
              className="inline-flex min-h-9 items-center gap-1.5 rounded-lg border border-border px-3 text-sm font-medium hover:bg-accent"
            >
              {t('admin.statusPage.open')}
              <ExternalLink className="size-3.5" />
            </a>
          }
        />

        <p className="max-w-3xl text-sm text-muted-foreground">{t('admin.statusPage.note')}</p>

        <AdminStatusTargets />

        <AdminFeedback feedback={feedback} onDismiss={clearFeedback} autoDismissMs={4000} />

        {isError ? (
          <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
            {t('admin.settings.loadError')}
          </div>
        ) : isLoading ? (
          <div className="flex items-center justify-center py-10 text-muted-foreground">
            <Loader2 className="size-5 animate-spin" />
          </div>
        ) : group ? (
          <AdminSettingsGroupEditor
            group={group}
            draft={draft}
            searchQuery=""
            expanded={expanded}
            onToggleExpand={() => setExpanded((v) => !v)}
            onDraftChange={setDraftValue}
            onToggle={handleToggle}
            onInstantEnum={handleInstantEnum}
            onSave={(keys) => handleSaveSection(group.id, keys)}
            saving={isGroupSaving(group.id)}
            togglingKey={togglingKey}
          />
        ) : null}
      </div>
    </AdminLayout>
  )
}

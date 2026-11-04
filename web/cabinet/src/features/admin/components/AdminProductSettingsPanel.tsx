import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2 } from 'lucide-react'

import { AdminFeedback } from './AdminFeedback'
import { AdminSettingsGroupEditor } from './AdminSettingsGroupEditor'
import { useAdminSettingsDraft } from '../hooks/useAdminSettingsDraft'
import {
  ADMIN_PRODUCT_SETTINGS_GROUPS,
  sortSettingsGroupsByOrder,
  type AdminSettingsGroupId,
} from '../utils/adminSettingsGroups'

/**
 * Продуктовые настройки под списком тарифов: витрина цен, триал и трафик,
 * HWID и устройства, курс Telegram Stars.
 *
 * Раньше жили отдельной вкладкой «Продукт» на странице «Настройки бота», из-за
 * чего прайс был разорван: цена тарифа — здесь, цена доп. устройства и курс
 * звёзд — на другом экране. Группы те же самые, источник данных тот же
 * (`GET/PATCH /admin/settings`), изменилось только место показа.
 */
export function AdminProductSettingsPanel() {
  const { t } = useTranslation()
  const {
    groups,
    isLoading,
    isError,
    draft,
    setDraftValue,
    togglingKey,
    savingGroup,
    isGroupSaving,
    handleToggle,
    handleInstantEnum,
    handleSaveSection,
    feedback,
    clearFeedback,
  } = useAdminSettingsDraft()

  const [expanded, setExpanded] = useState<Set<string>>(() => new Set())

  const allowed = new Set<AdminSettingsGroupId>(ADMIN_PRODUCT_SETTINGS_GROUPS)
  const productGroups = sortSettingsGroupsByOrder(
    (groups ?? []).filter((g) => allowed.has(g.id as AdminSettingsGroupId)),
  )

  if (isError) {
    return (
      <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
        {t('admin.settings.loadError')}
      </div>
    )
  }

  return (
    <section className="space-y-3">
      <AdminFeedback feedback={feedback} onDismiss={clearFeedback} autoDismissMs={4000} />

      <div className="flex flex-col gap-0.5">
        <h2 className="text-base font-semibold tracking-tight">{t('admin.tariffs.settingsTitle')}</h2>
        <p className="text-sm text-muted-foreground">{t('admin.tariffs.settingsSubtitle')}</p>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-10 text-muted-foreground">
          <Loader2 className="size-5 animate-spin" />
        </div>
      ) : (
        <div className="space-y-3">
          {productGroups.map((group) => (
            <AdminSettingsGroupEditor
              key={group.id}
              group={group}
              draft={draft}
              searchQuery=""
              expanded={expanded.has(group.id)}
              onToggleExpand={() =>
                setExpanded((prev) => {
                  const next = new Set(prev)
                  if (next.has(group.id)) next.delete(group.id)
                  else next.add(group.id)
                  return next
                })
              }
              onDraftChange={setDraftValue}
              onToggle={handleToggle}
              onInstantEnum={handleInstantEnum}
              onSave={(keys) => handleSaveSection(group.id, keys)}
              saving={isGroupSaving(group.id)}
              sectionSaving={savingGroup === group.id}
              togglingKey={togglingKey}
            />
          ))}
        </div>
      )}
    </section>
  )
}

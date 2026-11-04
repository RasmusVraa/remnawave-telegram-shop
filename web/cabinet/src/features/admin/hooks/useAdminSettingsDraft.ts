import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useAdminBotSettings, useAdminBotSettingsPatch } from './useAdminBotSettings'
import { useAdminMutationFeedback } from './useAdminMutationFeedback'

/**
 * Черновик настроек бота + сохранение по секциям.
 *
 * Вынесено из `AdminSettingsPage`, потому что те же самые группы настроек
 * теперь редактируются ещё и на странице «Тарифы» (продуктовые группы —
 * триал, HWID, звёзды, витрина цен). Логика одна, страниц две.
 *
 * Поля с `instant: true` сохраняются сразу при переключении; остальные копятся
 * в черновике и уходят одним PATCH по кнопке «Сохранить секцию».
 */
export function useAdminSettingsDraft() {
  const { t } = useTranslation()
  const { data, isLoading, isError } = useAdminBotSettings()
  const patchMutation = useAdminBotSettingsPatch()
  const { feedback, clear, showSuccess, showError } = useAdminMutationFeedback()

  const [draft, setDraft] = useState<Record<string, string>>({})
  const [togglingKey, setTogglingKey] = useState<string | null>(null)
  const [savingGroup, setSavingGroup] = useState<string | null>(null)

  useEffect(() => {
    if (!data) return
    setDraft((prev) => {
      const next = { ...prev }
      for (const g of data.groups) {
        for (const f of g.fields) {
          // instant-поля всегда синхронизируем с сервером: их значение уже применено.
          if (f.instant || prev[f.key] === undefined) {
            next[f.key] = f.value
          }
        }
      }
      return next
    })
  }, [data])

  const setDraftValue = useCallback((key: string, value: string) => {
    setDraft((d) => ({ ...d, [key]: value }))
  }, [])

  const handleToggle = useCallback(
    async (key: string, checked: boolean) => {
      const prev = draft[key]
      setDraft((d) => ({ ...d, [key]: checked ? 'true' : 'false' }))
      setTogglingKey(key)
      try {
        await patchMutation.mutateAsync({ [key]: checked ? 'true' : 'false' })
        showSuccess(t('admin.settings.saved'))
      } catch (err) {
        setDraft((d) => ({ ...d, [key]: prev ?? '' }))
        showError(err)
      } finally {
        setTogglingKey(null)
      }
    },
    [draft, patchMutation, showError, showSuccess, t],
  )

  const handleInstantEnum = useCallback(
    async (key: string, value: string) => {
      const prev = draft[key]
      setDraft((d) => ({ ...d, [key]: value }))
      setTogglingKey(key)
      try {
        await patchMutation.mutateAsync({ [key]: value })
        showSuccess(t('admin.settings.saved'))
      } catch (err) {
        setDraft((d) => ({ ...d, [key]: prev ?? '' }))
        showError(err)
      } finally {
        setTogglingKey(null)
      }
    },
    [draft, patchMutation, showError, showSuccess, t],
  )

  const handleSaveSection = useCallback(
    async (groupId: string, keys: string[]) => {
      const payload: Record<string, string> = {}
      for (const key of keys) {
        payload[key] = draft[key] ?? ''
      }
      setSavingGroup(groupId)
      try {
        await patchMutation.mutateAsync(payload)
        showSuccess(t('admin.settings.saved'))
      } catch (err) {
        showError(err)
      } finally {
        setSavingGroup(null)
      }
    },
    [draft, patchMutation, showError, showSuccess, t],
  )

  const isGroupSaving = useCallback(
    (groupId: string) => savingGroup === groupId || patchMutation.isPending,
    [savingGroup, patchMutation.isPending],
  )

  return {
    groups: data?.groups,
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
    clearFeedback: clear,
  }
}

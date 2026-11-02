import { useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, Info, Loader2, SlidersHorizontal } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { AdminToggleRow } from './AdminToggleSwitch'
import { AdminSelect } from './AdminSelect'
import { AdminDecorScheduleEditor } from './AdminDecorScheduleEditor'
import { AdminHomeQuickActionsEditor } from './AdminHomeQuickActionsEditor'
import { AdminCheckoutDefaultMethodEditor, AdminYookassaMethodsEditor } from './AdminPaymentMethodsEditors'
import { SettingsSubsectionTitle } from './SettingsSubsectionTitle'
import {
  ADMIN_SETTINGS_GROUP_ICONS,
  adminSettingsGroupAnchor,
  adminSettingsGroupIconStyle,
  splitGroupIntoSubsections,
  type AdminSettingsGroupId,
} from '../utils/adminSettingsGroups'
import { groupSettingsFieldsForLayout, isTextareaSettingType } from '../utils/adminSettingsFieldLayout'
import { decorThemeOptionLabelStyle } from '@/features/decor/decorThemeAdmin'
import type { AdminSettingFieldDTO, AdminSettingGroupDTO } from '@/lib/types/admin'

export function parseBool(v: string): boolean {
  return v.trim().toLowerCase() === 'true'
}

/** Лейбл поля из i18n; если перевода нет — показываем сам ключ настройки. */
export function fieldLabel(t: (k: string) => string, key: string): string {
  const k = `admin.settings.fields.${key}.label`
  const translated = t(k)
  return translated === k ? key : translated
}

export function fieldHint(t: (k: string) => string, key: string): string | undefined {
  const k = `admin.settings.fields.${key}.hint`
  const translated = t(k)
  if (translated === k || translated.trim() === '') return undefined
  return translated
}

export function fieldHasHint(t: (k: string) => string, key: string): boolean {
  return fieldHint(t, key) !== undefined
}

export function settingsGroupIcon(id: string): LucideIcon {
  return ADMIN_SETTINGS_GROUP_ICONS[id as AdminSettingsGroupId] ?? SlidersHorizontal
}

export interface AdminSettingsGroupEditorProps {
  group: AdminSettingGroupDTO
  draft: Record<string, string>
  searchQuery: string
  expanded: boolean
  categoryBadge?: string
  onToggleExpand: () => void
  onDraftChange: (key: string, value: string) => void
  onToggle: (key: string, value: boolean) => void
  onInstantEnum: (key: string, value: string) => void
  onSave: (keys: string[]) => void
  saving: boolean
  togglingKey: string | null
}

/**
 * Раскрывающаяся секция одной группы настроек.
 *
 * Компонент полностью управляемый: черновик, флаги сохранения и обработчики
 * приходят снаружи (см. `useAdminSettingsDraft`). Благодаря этому одни и те же
 * блоки рендерятся и на «Настройках бота», и на странице «Тарифы», где живёт
 * продуктовая группа настроек.
 */
export function AdminSettingsGroupEditor({
  group,
  draft,
  searchQuery,
  expanded,
  categoryBadge,
  onToggleExpand,
  onDraftChange,
  onToggle,
  onInstantEnum,
  onSave,
  saving,
  togglingKey,
}: AdminSettingsGroupEditorProps) {
  const { t } = useTranslation()
  const Icon = settingsGroupIcon(group.id)
  const iconStyle = adminSettingsGroupIconStyle(group.id)
  const q = searchQuery.trim().toLowerCase()

  const fieldMatchesSearch = useCallback(
    (field: AdminSettingFieldDTO) => {
      if (!q) return true
      const label = fieldLabel(t, field.key).toLowerCase()
      return label.includes(q) || field.key.toLowerCase().includes(q)
    },
    [q, t],
  )

  const visibleFields = useMemo(
    () => group.fields.filter(fieldMatchesSearch),
    [group.fields, fieldMatchesSearch],
  )

  const nonInstantKeys = useMemo(
    () => visibleFields.filter((f) => !f.instant).map((f) => f.key),
    [visibleFields],
  )

  const fieldByKey = useMemo(() => {
    const m = new Map<string, AdminSettingFieldDTO>()
    for (const f of group.fields) m.set(f.key, f)
    return m
  }, [group.fields])

  const subsectionBlocks = useMemo(() => {
    const keys = visibleFields.map((f) => f.key)
    return splitGroupIntoSubsections(group.id, keys)
      .map((block) => ({
        def: block.def,
        fields: block.fields
          .map(({ key }) => fieldByKey.get(key))
          .filter(Boolean) as AdminSettingFieldDTO[],
      }))
      .filter((b) => b.fields.length > 0)
  }, [group.id, visibleFields, fieldByKey])

  if (visibleFields.length === 0) {
    return null
  }

  const renderField = (field: AdminSettingFieldDTO, compact = false) => {
    const label = fieldLabel(t, field.key)
    const hint = fieldHint(t, field.key)
    const value = draft[field.key] ?? field.value

    // Расписание авто-тем — не текстовое поле, а свой редактор окон:
    // в настройке лежит JSON, руками его никто редактировать не должен.
    if (field.key === 'CABINET_DECOR_SCHEDULE') {
      return (
        <div key={field.key} className="border-b border-border/50 py-3 last:border-0">
          <AdminDecorScheduleEditor
            value={value}
            autoEnabled={parseBool(draft.CABINET_DECOR_AUTO_ENABLED ?? 'false')}
            disabled={saving}
            onChange={(next) => onDraftChange(field.key, next)}
          />
        </div>
      )
    }

    if (field.key === 'YOOKASA_PAYMENT_METHODS') {
      return (
        <div key={field.key} className="border-b border-border/50 py-3 last:border-0">
          <AdminYookassaMethodsEditor
            value={value}
            disabled={saving}
            onChange={(next) => onDraftChange(field.key, next)}
          />
        </div>
      )
    }

    if (field.key === 'CABINET_CHECKOUT_DEFAULT_METHOD') {
      return (
        <div key={field.key} className="border-b border-border/50 py-3 last:border-0">
          <AdminCheckoutDefaultMethodEditor
            value={value}
            yookassaMethodsValue={
              draft.YOOKASA_PAYMENT_METHODS ?? fieldByKey.get('YOOKASA_PAYMENT_METHODS')?.value ?? ''
            }
            disabled={saving}
            onChange={(next) => onDraftChange(field.key, next)}
          />
        </div>
      )
    }

    if (field.key === 'CABINET_HOME_QUICK_ACTIONS') {
      return (
        <div key={field.key} className="border-b border-border/50 py-3 last:border-0">
          <AdminHomeQuickActionsEditor
            value={value}
            disabled={saving}
            onChange={(next) => onDraftChange(field.key, next)}
          />
        </div>
      )
    }

    if (field.type === 'bool') {
      return (
        <AdminToggleRow
          key={field.key}
          label={label}
          hint={hint}
          checked={parseBool(value)}
          disabled={togglingKey === field.key || saving}
          onChange={(checked) => onToggle(field.key, checked)}
        />
      )
    }

    const inputId = `setting-${field.key}`
    return (
      <div
        key={field.key}
        className={cn(
          'space-y-1.5',
          compact
            ? 'rounded-lg border border-border/60 bg-card p-2.5 shadow-sm'
            : 'border-b border-border/50 py-3 last:border-0',
        )}
      >
        <label htmlFor={inputId} className="block text-sm font-medium leading-snug text-foreground">
          {label}
        </label>
        {field.type === 'enum' && field.enum_values?.length ? (
          <AdminSelect
            id={inputId}
            value={value}
            options={field.enum_values.map((opt) => ({
              value: opt,
              label: t(`admin.settings.enum.${opt}`, { defaultValue: opt }),
              labelStyle:
                field.key === 'CABINET_DECOR_THEME' ? decorThemeOptionLabelStyle(opt) : undefined,
            }))}
            onChange={(next) => {
              if (next == null) return
              if (field.instant) {
                onInstantEnum(field.key, next)
              } else {
                onDraftChange(field.key, next)
              }
            }}
            placeholder={label}
            ariaLabel={label}
            disabled={togglingKey === field.key || saving}
          />
        ) : isTextareaSettingType(field.type) ? (
          <textarea
            id={inputId}
            rows={3}
            className="admin-input w-full resize-y rounded-lg border border-border bg-card px-3 py-2 font-mono text-sm"
            value={value}
            placeholder={t('admin.settings.csvPlaceholder')}
            onChange={(e) => onDraftChange(field.key, e.target.value)}
          />
        ) : (
          <input
            id={inputId}
            type={field.type === 'float' || field.type === 'int' ? 'number' : 'text'}
            step={field.type === 'float' ? 'any' : '1'}
            min={field.min_int}
            max={field.max_int}
            className="admin-input w-full rounded-lg border border-border bg-card px-3 py-2 text-sm"
            value={value}
            onChange={(e) => onDraftChange(field.key, e.target.value)}
          />
        )}
        {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
        {field.key === 'RUB_PER_STAR' && (
          <p className="text-xs font-medium text-muted-foreground">
            {t('admin.settings.fields.RUB_PER_STAR.envKey')}
          </p>
        )}
      </div>
    )
  }

  const renderFieldsLayout = (fields: AdminSettingFieldDTO[]) => {
    const groups = groupSettingsFieldsForLayout(fields, (key) => fieldHasHint(t, key))

    return groups.map((layoutGroup, groupIndex) => {
      if (layoutGroup.kind === 'compact') {
        return (
          <div
            key={`compact-${groupIndex}`}
            className="grid grid-cols-1 gap-2.5 border-b border-border/50 py-3 last:border-0 sm:grid-cols-2 lg:grid-cols-3"
          >
            {layoutGroup.fields.map((field) => renderField(field, true))}
          </div>
        )
      }

      return layoutGroup.fields.map((field) => renderField(field, false))
    })
  }

  const renderSubsection = (
    block: { def: (typeof subsectionBlocks)[0]['def']; fields: AdminSettingFieldDTO[] },
    index: number,
  ) => (
    <div key={block.def?.id ?? `flat-${index}`} className="mb-4 last:mb-0">
      {block.def && (
        <SettingsSubsectionTitle icon={block.def.icon}>{t(block.def.titleKey)}</SettingsSubsectionTitle>
      )}
      <div className="rounded-lg border border-border/60 bg-card/80 px-3 shadow-sm">
        {renderFieldsLayout(block.fields)}
      </div>
    </div>
  )

  const groupTitle = t(`admin.settings.groups.${group.id}`)
  // Пояснение ко всей группе — там, где по подписям полей легко понять не то.
  const groupIntro = t(`admin.settings.groupIntros.${group.id}`, { defaultValue: '' })

  return (
    <Card id={adminSettingsGroupAnchor(group.id)} className="cabinet-elevated-card scroll-mt-24 overflow-hidden">
      <button
        type="button"
        onClick={onToggleExpand}
        className="flex min-h-11 w-full items-center justify-between gap-3 px-4 py-3 text-left transition-colors hover:bg-accent/40 sm:px-5"
        aria-expanded={expanded}
      >
        <div className="flex min-w-0 items-center gap-3">
          <div className={cn('flex size-8 shrink-0 items-center justify-center rounded-lg', iconStyle.box)}>
            <Icon className={cn('size-4', iconStyle.icon)} aria-hidden />
          </div>
          <div className="flex min-w-0 flex-col gap-0.5">
            {categoryBadge && (
              <span className="truncate text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
                {categoryBadge}
              </span>
            )}
            <p className="truncate text-sm font-semibold uppercase tracking-wide text-foreground sm:text-base sm:normal-case sm:tracking-normal">
              {groupTitle}
            </p>
          </div>
        </div>
        <ChevronDown
          className={cn(
            'size-5 shrink-0 text-muted-foreground transition-transform duration-200',
            expanded && 'rotate-180',
          )}
        />
      </button>

      {expanded && (
        <div className="admin-reveal">
          <CardContent className="space-y-1 border-t border-border/60 pt-4">
            {groupIntro && (
              <div className="mb-4 flex gap-2.5 rounded-lg border border-primary/25 bg-primary/[0.06] px-3 py-2.5 text-[13px] leading-relaxed text-foreground/90">
                <Info className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden />
                <p>{groupIntro}</p>
              </div>
            )}
            {subsectionBlocks.length === 1 && !subsectionBlocks[0].def ? (
              <div className="rounded-lg border border-border/60 bg-card/80 px-3 shadow-sm">
                {renderFieldsLayout(subsectionBlocks[0].fields)}
              </div>
            ) : (
              subsectionBlocks.map(renderSubsection)
            )}

            {nonInstantKeys.length > 0 && (
              <div className="flex justify-end pt-3">
                <Button
                  type="button"
                  size="sm"
                  disabled={saving}
                  onClick={() => onSave(nonInstantKeys)}
                  className="min-w-[140px]"
                >
                  {saving ? (
                    <>
                      <Loader2 className="mr-2 size-4 animate-spin" />
                      {t('admin.settings.saving')}
                    </>
                  ) : (
                    t('admin.settings.saveSection')
                  )}
                </Button>
              </div>
            )}
          </CardContent>
        </div>
      )}
    </Card>
  )
}

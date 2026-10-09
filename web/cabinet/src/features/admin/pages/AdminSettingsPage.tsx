import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, Search, SlidersHorizontal, X } from 'lucide-react'

import { cn } from '@/lib/utils'
import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { AdminFeedback } from '../components/AdminFeedback'
import { AdminSettingsGroupEditor, fieldLabel } from '../components/AdminSettingsGroupEditor'
import { useAdminSettingsDraft } from '../hooks/useAdminSettingsDraft'
import {
  ADMIN_PRODUCT_SETTINGS_GROUPS,
  ADMIN_STATUS_SETTINGS_GROUP,
  ADMIN_SETTINGS_CATEGORIES,
  ADMIN_SETTINGS_DEFAULT_CATEGORY,
  ADMIN_SETTINGS_GROUPS_LIST_ANCHOR,
  adminSettingsCategoryDef,
  adminSettingsCategoryForGroup,
  scrollToSettingsGroupsList,
  sortSettingsGroupsByOrder,
  type AdminSettingsCategoryId,
  type AdminSettingsGroupId,
} from '../utils/adminSettingsGroups'
import type { AdminSettingGroupDTO } from '@/lib/types/admin'


interface CategoryNavProps {
  activeId: AdminSettingsCategoryId
  onSelect: (id: AdminSettingsCategoryId) => void
}

function settingsCategoryTabId(categoryId: AdminSettingsCategoryId): string {
  return `settings-category-tab-${categoryId}`
}

function SettingsCategoryNav({ activeId, onSelect }: CategoryNavProps) {
  const { t } = useTranslation()

  return (
    <div
      role="tablist"
      aria-label={t('admin.settings.categoryNav')}
      className="-mx-1 overflow-x-auto overscroll-x-contain px-1 pb-0.5 lg:overflow-visible"
    >
      <div className="inline-flex min-w-full gap-1 rounded-lg border border-border/50 bg-card/50 p-1 sm:min-w-0 sm:w-full">
        {ADMIN_SETTINGS_CATEGORIES.map((category) => {
          const Icon = category.icon
          const isActive = activeId === category.id
          return (
            <button
              key={category.id}
              type="button"
              role="tab"
              id={settingsCategoryTabId(category.id)}
              aria-selected={isActive}
              aria-controls={ADMIN_SETTINGS_GROUPS_LIST_ANCHOR}
              tabIndex={isActive ? 0 : -1}
              onClick={() => onSelect(category.id)}
              className={cn(
                'inline-flex min-h-9 shrink-0 items-center justify-center gap-1.5 rounded-md px-3 py-2 text-center text-xs font-medium transition-colors sm:flex-1',
                isActive
                  ? cn(category.iconStyle.box, category.iconStyle.icon)
                  : 'text-foreground/80 hover:bg-accent hover:text-foreground',
              )}
            >
              <Icon
                className={cn('size-3.5 shrink-0', isActive ? category.iconStyle.icon : undefined)}
                aria-hidden
              />
              <span className="truncate leading-tight">{t(category.titleKey)}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}

export default function AdminSettingsPage() {
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
  const [searchQuery, setSearchQuery] = useState('')
  const [activeCategory, setActiveCategory] = useState<AdminSettingsCategoryId>(ADMIN_SETTINGS_DEFAULT_CATEGORY)
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(() => new Set())

  // Продуктовые группы переехали на страницу «Тарифы» — прячем их здесь целиком,
  // включая поиск, иначе их можно было бы редактировать в двух местах сразу.
  const pageGroups = useMemo(() => {
    const moved = new Set<string>([...ADMIN_PRODUCT_SETTINGS_GROUPS, ADMIN_STATUS_SETTINGS_GROUP])
    return (groups ?? []).filter((g) => !moved.has(g.id))
  }, [groups])

  const searchFilteredGroups = useMemo(() => {
    const q = searchQuery.trim().toLowerCase()
    if (!q) return pageGroups

    return pageGroups
      .map((group) => {
        const groupTitle = t(`admin.settings.groups.${group.id}`).toLowerCase()
        if (groupTitle.includes(q)) return group
        const fields = group.fields.filter((f) => {
          const label = fieldLabel(t, f.key).toLowerCase()
          return label.includes(q) || f.key.toLowerCase().includes(q)
        })
        if (fields.length === 0) return null
        return { ...group, fields }
      })
      .filter(Boolean) as AdminSettingGroupDTO[]
  }, [pageGroups, searchQuery, t])

  const isSearching = searchQuery.trim().length > 0

  const visibleGroups = useMemo(() => {
    if (isSearching) return sortSettingsGroupsByOrder(searchFilteredGroups)

    const category = adminSettingsCategoryDef(activeCategory)
    if (!category) return []

    const allowed = new Set(category.groups)
    return sortSettingsGroupsByOrder(
      searchFilteredGroups.filter((group) => allowed.has(group.id as AdminSettingsGroupId)),
    )
  }, [activeCategory, isSearching, searchFilteredGroups])

  const categoryBadgeForGroup = useCallback(
    (groupId: string) => {
      if (!isSearching) return undefined
      const categoryId = adminSettingsCategoryForGroup(groupId)
      if (!categoryId) return undefined
      const category = adminSettingsCategoryDef(categoryId)
      return category ? t(category.titleKey) : undefined
    },
    [isSearching, t],
  )

  // Expand all matching groups while searching; collapse only when search ends.
  // Do not reset on visibleGroups/data refetch — that would close sections after saving.
  useEffect(() => {
    if (isSearching) {
      setExpandedGroups(new Set(visibleGroups.map((g) => g.id)))
    }
  }, [isSearching, visibleGroups])

  useEffect(() => {
    if (!isSearching) {
      setExpandedGroups(new Set())
    }
  }, [isSearching])

  const toggleGroupExpanded = useCallback((id: string) => {
    setExpandedGroups((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const handleCategoryNav = useCallback((categoryId: AdminSettingsCategoryId) => {
    setActiveCategory(categoryId)
    setExpandedGroups(new Set())
    window.requestAnimationFrame(() => {
      scrollToSettingsGroupsList()
    })
  }, [])

  return (
    <AdminLayout>
      <AdminFeedback feedback={feedback} onDismiss={clearFeedback} autoDismissMs={4000} />
      <div className="mx-auto max-w-2xl space-y-4 lg:max-w-5xl">
        <AdminPageHeader
          icon={SlidersHorizontal}
          title={t('admin.settings.title')}
          subtitle={t('admin.settings.subtitle')}
          accent="violet"
        />

        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="search"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={t('admin.settings.searchPlaceholder')}
            className="admin-input h-9 w-full rounded-md border border-input bg-background pl-9 pr-9 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          />
          {searchQuery && (
            <button
              type="button"
              aria-label={t('admin.settings.searchClear')}
              className="absolute right-2 top-1/2 -translate-y-1/2 rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
              onClick={() => setSearchQuery('')}
            >
              <X className="size-4" />
            </button>
          )}
        </div>

        {!isLoading && !isError && !isSearching && (
          <SettingsCategoryNav activeId={activeCategory} onSelect={handleCategoryNav} />
        )}

        {isLoading && (
          <div className="flex items-center justify-center py-16 text-muted-foreground">
            <Loader2 className="size-6 animate-spin" />
          </div>
        )}

        {isError && (
          <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
            {t('admin.settings.loadError')}
          </div>
        )}

        {!isLoading && !isError && searchQuery && visibleGroups.length === 0 && (
          <p className="py-8 text-center text-sm text-muted-foreground">{t('admin.settings.searchEmpty')}</p>
        )}

        {!isLoading && !isError && !isSearching && visibleGroups.length === 0 && (
          <p className="py-8 text-center text-sm text-muted-foreground">{t('admin.settings.categoryEmpty')}</p>
        )}

        <div
          id={ADMIN_SETTINGS_GROUPS_LIST_ANCHOR}
          role="tabpanel"
          aria-labelledby={settingsCategoryTabId(activeCategory)}
          className="scroll-mt-24 space-y-3"
        >
          {visibleGroups.map((group) => (
            <AdminSettingsGroupEditor
              key={group.id}
              group={group}
              draft={draft}
              searchQuery={searchQuery}
              categoryBadge={categoryBadgeForGroup(group.id)}
              expanded={isSearching || expandedGroups.has(group.id)}
              onToggleExpand={() => toggleGroupExpanded(group.id)}
              onDraftChange={setDraftValue}
              onToggle={handleToggle}
              onInstantEnum={handleInstantEnum}
              onSave={(keys) => handleSaveSection(group.id, keys)}
              saving={isGroupSaving(group.id)}
              togglingKey={togglingKey}
            />
          ))}
        </div>
      </div>
    </AdminLayout>
  )
}

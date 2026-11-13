import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Server, Plus, Trash2, Edit, Globe, Bell, History } from 'lucide-react'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
// Локальный Spinner ниже — загрузка вкладки; этот — спиннер кнопки.
import { Spinner as ButtonSpinner } from '@/components/ui/spinner'
import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { AdminModal } from '../components/AdminModal'
import { AdminSelect } from '../components/AdminSelect'
import { AdminCheckboxField } from '../components/AdminCheckbox'
import { AdminFeedback } from '../components/AdminFeedback'
import { useAdminMutationFeedback } from '../hooks/useAdminMutationFeedback'
import { faviconSrcFromUrl, normalizeHttpUrl } from '../utils/normalizeUrl'
import { CountryFlag } from '../components/CountryFlag'
import { Card } from '@/components/ui/card'

type TabKey = 'nodes' | 'providers' | 'history' | 'settings'

/** Отказ сервера во вкладке — общим уведомлением страницы: раньше ошибки тут не показывались вовсе. */
type TabProps = { onError: (err: unknown) => void }

function toDatetimeLocal(iso?: string | null): string {
  if (!iso) return ''
  try {
    return new Date(iso).toISOString().slice(0, 16)
  } catch {
    return ''
  }
}

export default function AdminInfraPage() {
  const { t } = useTranslation()
  const [tab, setTab] = useState<TabKey>('nodes')
  const { feedback, clear, showError } = useAdminMutationFeedback()

  const tabs: { key: TabKey; label: string; icon: typeof Server }[] = [
    { key: 'nodes', label: t('admin.infra.tabs.nodes'), icon: Server },
    { key: 'providers', label: t('admin.infra.tabs.providers'), icon: Globe },
    { key: 'history', label: t('admin.infra.tabs.history'), icon: History },
    { key: 'settings', label: t('admin.infra.tabs.settings'), icon: Bell },
  ]

  return (
    <AdminLayout>
      <AdminFeedback feedback={feedback} onDismiss={clear} />
      <div className="space-y-6">
        <AdminPageHeader
          icon={Server}
          title={t('admin.infra.title')}
          subtitle={t('admin.infra.subtitle')}
          accent="slate"
        />

        <div className="-mx-1 overflow-x-auto overscroll-x-contain px-1 pb-0.5">
          <div role="tablist" className="inline-flex min-w-full gap-1 rounded-lg border border-border/50 bg-card/50 p-1 sm:min-w-0 sm:w-full">
            {tabs.map(({ key, label, icon: Icon }) => (
              <button
                key={key}
                type="button"
                role="tab"
                aria-selected={tab === key}
                onClick={() => setTab(key)}
                className={cn(
                  'inline-flex min-h-9 shrink-0 items-center justify-center gap-1.5 rounded-md px-3 py-2 text-sm font-medium transition-colors sm:flex-1',
                  tab === key
                    ? 'bg-primary/10 text-primary dark:bg-primary/20'
                    : 'text-foreground/80 hover:bg-accent hover:text-foreground',
                )}
              >
                <Icon className="size-4 shrink-0" />
                {label}
              </button>
            ))}
          </div>
        </div>

        {tab === 'nodes' && <NodesTab onError={showError} />}
        {tab === 'providers' && <ProvidersTab onError={showError} />}
        {tab === 'history' && <HistoryTab onError={showError} />}
        {tab === 'settings' && <SettingsTab onError={showError} />}
      </div>
    </AdminLayout>
  )
}

function NodesTab({ onError }: TabProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [editNode, setEditNode] = useState<{ uuid: string; name: string; nextBillingAt: string } | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['admin-infra-nodes'],
    queryFn: () => api.adminInfraNodes(),
  })

  const { data: providersData } = useQuery({
    queryKey: ['admin-infra-providers'],
    queryFn: () => api.adminInfraProviders(),
    enabled: createOpen,
  })

  const deleteMutation = useMutation({
    meta: { haptics: true },
    onError,
    mutationFn: (uuid: string) => api.adminInfraDeleteNode(uuid),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin-infra-nodes'] }),
  })

  const createMutation = useMutation({
    meta: { haptics: true },
    onError,
    mutationFn: (body: { provider_uuid: string; node_uuid: string; next_billing_at?: string }) =>
      api.adminInfraCreateNode(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-infra-nodes'] })
      setCreateOpen(false)
    },
  })

  const patchMutation = useMutation({
    meta: { haptics: true },
    onError,
    mutationFn: (body: { uuid: string; next_billing_at: string }) => api.adminInfraPatchNode(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-infra-nodes'] })
      setEditNode(null)
    },
  })

  const [form, setForm] = useState({ provider_uuid: '', node_uuid: '', next_billing_at: '' })

  if (isLoading) return <Spinner />

  const available = data?.availableBillingNodes ?? []

  return (
    <div className="space-y-4">
      {data?.stats && (
        <div className="grid gap-3 sm:grid-cols-3">
          <StatCard label={t('admin.infra.upcomingNodes')} value={data.stats.upcomingNodesCount} />
          <StatCard label={t('admin.infra.monthPayments')} value={`$${data.stats.currentMonthPayments.toFixed(2)}`} />
          <StatCard label={t('admin.infra.totalSpent')} value={`$${data.stats.totalSpent.toFixed(2)}`} />
        </div>
      )}

      <div className="rounded-lg border border-border/50 bg-card">
        <div className="flex items-center justify-between border-b border-border/50 px-4 py-3">
          <h3 className="flex items-center gap-2 text-sm font-medium">
            <Server className="size-4" />
            {t('admin.infra.billingNodes')} ({data?.totalBillingNodes ?? 0})
          </h3>
          <button
            type="button"
            disabled={available.length === 0}
            onClick={() => {
              setForm({ provider_uuid: '', node_uuid: '', next_billing_at: '' })
              setCreateOpen(true)
            }}
            className="inline-flex items-center gap-1 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            <Plus className="size-3.5" />
            {t('admin.infra.addNode')}
          </button>
        </div>
        {!data?.billingNodes?.length ? (
          <p className="px-4 py-6 text-center text-sm text-muted-foreground">{t('admin.infra.noNodes')}</p>
        ) : (
          <div className="divide-y divide-border/50">
            {data.billingNodes.map((node: any) => (
              <div key={node.uuid} className="flex items-center justify-between px-4 py-3">
                <div className="flex min-w-0 items-center gap-2.5">
                  <CountryFlag code={node.node?.countryCode} />
                  <div className="min-w-0">
                  <div className="text-sm font-medium">{node.node?.name ?? node.nodeUuid}</div>
                  <div className="text-xs text-muted-foreground">
                    {node.provider?.name} &middot;{' '}
                    {t('admin.infra.nextBilling')}: {new Date(node.nextBillingAt).toLocaleDateString()}
                  </div>
                  </div>
                </div>
                <div className="flex items-center gap-1">
                  <button
                    type="button"
                    onClick={() => setEditNode({
                      uuid: node.uuid,
                      name: node.node?.name ?? node.nodeUuid,
                      nextBillingAt: toDatetimeLocal(node.nextBillingAt),
                    })}
                    className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                    title={t('admin.infra.editNode')}
                  >
                    <Edit className="size-4" />
                  </button>
                  <button
                    type="button"
                    disabled={deleteMutation.isPending && deleteMutation.variables === node.uuid}
                    onClick={() => {
                      if (window.confirm(t('admin.infra.confirmDeleteNode'))) {
                        deleteMutation.mutate(node.uuid)
                      }
                    }}
                    className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                  >
                    {deleteMutation.isPending && deleteMutation.variables === node.uuid ? <ButtonSpinner /> : <Trash2 className="size-4" />}
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <AdminModal open={createOpen} onClose={() => setCreateOpen(false)} title={t('admin.infra.createNode')} icon={Server} iconAccent="blue">
        {available.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('admin.infra.noAvailableNodes')}</p>
        ) : (
          <div className="space-y-3">
            <div>
              <label className="mb-1 block text-sm font-medium">{t('admin.infra.pickProvider')}</label>
              <AdminSelect
                value={form.provider_uuid || null}
                options={(providersData?.providers ?? []).map((p: any) => ({ value: p.uuid as string, label: p.name as string }))}
                onChange={(next) => setForm((f) => ({ ...f, provider_uuid: next ?? '' }))}
                allowEmpty
                emptyLabel="—"
                placeholder="—"
                ariaLabel={t('admin.infra.pickProvider')}
              />
            </div>
            <div>
              <label className="mb-1 block text-sm font-medium">{t('admin.infra.pickNode')}</label>
              <AdminSelect
                value={form.node_uuid || null}
                options={available.map((n: any) => ({ value: n.uuid as string, label: `${n.name}${n.countryCode ? ` (${n.countryCode})` : ''}` }))}
                onChange={(next) => setForm((f) => ({ ...f, node_uuid: next ?? '' }))}
                allowEmpty
                emptyLabel="—"
                placeholder="—"
                ariaLabel={t('admin.infra.pickNode')}
              />
            </div>
            <div>
              <label className="mb-1 block text-sm font-medium">{t('admin.infra.nextBillingAt')}</label>
              <input
                type="datetime-local"
                className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                value={form.next_billing_at}
                onChange={(e) => setForm((f) => ({ ...f, next_billing_at: e.target.value }))}
              />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <button type="button" className="rounded-md border px-4 py-2 text-sm hover:bg-accent" onClick={() => setCreateOpen(false)}>
                {t('admin.cancel')}
              </button>
              <button
                type="button"
                disabled={!form.provider_uuid || !form.node_uuid || createMutation.isPending}
                className={cn('inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90 disabled:opacity-50', createMutation.isPending && '!opacity-100')}
                onClick={() => createMutation.mutate({
                  provider_uuid: form.provider_uuid,
                  node_uuid: form.node_uuid,
                  next_billing_at: form.next_billing_at ? new Date(form.next_billing_at).toISOString() : undefined,
                })}
              >
                {createMutation.isPending && <ButtonSpinner />}
                {t('admin.create')}
              </button>
            </div>
          </div>
        )}
      </AdminModal>

      <AdminModal open={editNode != null} onClose={() => setEditNode(null)} title={t('admin.infra.editNode')} icon={Edit} iconAccent="blue">
        {editNode && (
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">{editNode.name}</p>
            <div>
              <label className="mb-1 block text-sm font-medium">{t('admin.infra.nextBillingAt')}</label>
              <input
                type="datetime-local"
                className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                value={editNode.nextBillingAt}
                onChange={(e) => setEditNode({ ...editNode, nextBillingAt: e.target.value })}
              />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <button type="button" className="rounded-md border px-4 py-2 text-sm hover:bg-accent" onClick={() => setEditNode(null)}>
                {t('admin.cancel')}
              </button>
              <button
                type="button"
                disabled={!editNode.nextBillingAt || patchMutation.isPending}
                className={cn('inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90 disabled:opacity-50', patchMutation.isPending && '!opacity-100')}
                onClick={() => patchMutation.mutate({
                  uuid: editNode.uuid,
                  next_billing_at: new Date(editNode.nextBillingAt).toISOString(),
                })}
              >
                {patchMutation.isPending && <ButtonSpinner />}
                {t('admin.save')}
              </button>
            </div>
          </div>
        )}
      </AdminModal>
    </div>
  )
}

function ProvidersTab({ onError }: TabProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [editProv, setEditProv] = useState<{ uuid: string; name: string; favicon_link: string; login_url: string } | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['admin-infra-providers'],
    queryFn: () => api.adminInfraProviders(),
  })

  const deleteMutation = useMutation({
    meta: { haptics: true },
    onError,
    mutationFn: (uuid: string) => api.adminInfraDeleteProvider(uuid),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin-infra-providers'] }),
  })

  const createMutation = useMutation({
    meta: { haptics: true },
    onError,
    mutationFn: (body: { name: string; favicon_link?: string; login_url?: string }) =>
      api.adminInfraCreateProvider(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-infra-providers'] })
      setCreateOpen(false)
    },
  })

  const patchMutation = useMutation({
    meta: { haptics: true },
    onError,
    mutationFn: (body: { uuid: string; name?: string; favicon_link?: string; login_url?: string }) =>
      api.adminInfraPatchProvider(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-infra-providers'] })
      setEditProv(null)
    },
  })

  const [form, setForm] = useState({ name: '', favicon_link: '', login_url: '' })

  if (isLoading) return <Spinner />

  return (
    <div className="rounded-lg border border-border/50 bg-card">
      <div className="flex items-center justify-between border-b border-border/50 px-4 py-3">
        <h3 className="flex items-center gap-2 text-sm font-medium">
          <Globe className="size-4" />
          {t('admin.infra.providers')} ({data?.total ?? 0})
        </h3>
        <button
          type="button"
          onClick={() => {
            setForm({ name: '', favicon_link: '', login_url: '' })
            setCreateOpen(true)
          }}
          className="inline-flex items-center gap-1 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
        >
          <Plus className="size-3.5" />
          {t('admin.infra.addProvider')}
        </button>
      </div>
      {!data?.providers?.length ? (
        <p className="px-4 py-6 text-center text-sm text-muted-foreground">{t('admin.infra.noProviders')}</p>
      ) : (
        <div className="divide-y divide-border/50">
          {data.providers.map((prov: any) => (
            <div key={prov.uuid} className="flex items-center justify-between px-4 py-3">
              <div className="flex items-center gap-3">
                {prov.faviconLink && (
                  <img
                    src={faviconSrcFromUrl(prov.faviconLink)}
                    alt=""
                    className="size-6 rounded object-cover"
                    onError={(e) => {
                      e.currentTarget.style.display = 'none'
                    }}
                  />
                )}
                <div>
                  <div className="text-sm font-medium">{prov.name}</div>
                  <div className="text-xs text-muted-foreground">
                    {prov.billingNodes?.length ?? 0} {t('admin.infra.nodes')} &middot; ${prov.billingHistory?.totalAmount?.toFixed(2) ?? '0.00'}
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-1">
                {prov.loginUrl && (
                  <a href={normalizeHttpUrl(prov.loginUrl)} target="_blank" rel="noopener noreferrer" className="rounded-md p-1.5 text-muted-foreground hover:text-foreground" title={t('admin.infra.openLogin')}>
                    <Globe className="size-4" />
                  </a>
                )}
                <button
                  type="button"
                  onClick={() => setEditProv({
                    uuid: prov.uuid,
                    name: prov.name,
                    favicon_link: normalizeHttpUrl(prov.faviconLink ?? ''),
                    login_url: normalizeHttpUrl(prov.loginUrl ?? ''),
                  })}
                  className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                >
                  <Edit className="size-4" />
                </button>
                <button
                  type="button"
                  disabled={deleteMutation.isPending && deleteMutation.variables === prov.uuid}
                  onClick={() => {
                    if (window.confirm(t('admin.infra.confirmDeleteProvider'))) {
                      deleteMutation.mutate(prov.uuid)
                    }
                  }}
                  className="rounded-md p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                >
                  {deleteMutation.isPending && deleteMutation.variables === prov.uuid ? <ButtonSpinner /> : <Trash2 className="size-4" />}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      <AdminModal open={createOpen} onClose={() => setCreateOpen(false)} title={t('admin.infra.createProvider')} icon={Globe} iconAccent="indigo">
        <ProviderForm
          form={form}
          setForm={setForm}
          onCancel={() => setCreateOpen(false)}
          onSave={() => createMutation.mutate({
            name: form.name.trim(),
            favicon_link: form.favicon_link.trim() ? normalizeHttpUrl(form.favicon_link) : undefined,
            login_url: form.login_url.trim() ? normalizeHttpUrl(form.login_url) : undefined,
          })}
          saving={createMutation.isPending}
          saveLabel={t('admin.create')}
        />
      </AdminModal>

      <AdminModal open={editProv != null} onClose={() => setEditProv(null)} title={t('admin.infra.editProvider')} icon={Edit} iconAccent="indigo">
        {editProv && (
          <ProviderForm
            form={{ name: editProv.name, favicon_link: editProv.favicon_link, login_url: editProv.login_url }}
            setForm={(f) => setEditProv({ ...editProv, ...f })}
            onCancel={() => setEditProv(null)}
            onSave={() => patchMutation.mutate({
              uuid: editProv.uuid,
              name: editProv.name.trim(),
              favicon_link: editProv.favicon_link.trim() ? normalizeHttpUrl(editProv.favicon_link) : undefined,
              login_url: editProv.login_url.trim() ? normalizeHttpUrl(editProv.login_url) : undefined,
            })}
            saving={patchMutation.isPending}
            saveLabel={t('admin.save')}
          />
        )}
      </AdminModal>
    </div>
  )
}

function ProviderForm({
  form,
  setForm,
  onCancel,
  onSave,
  saving,
  saveLabel,
}: {
  form: { name: string; favicon_link: string; login_url: string }
  setForm: (f: { name: string; favicon_link: string; login_url: string }) => void
  onCancel: () => void
  onSave: () => void
  saving: boolean
  saveLabel: string
}) {
  const { t } = useTranslation()
  return (
    <div className="space-y-3">
      <div>
        <label className="mb-1 block text-sm font-medium">{t('admin.infra.providerName')}</label>
        <input
          className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
        />
      </div>
      <div>
        <label className="mb-1 block text-sm font-medium">{t('admin.infra.faviconLink')}</label>
        <input
          type="text"
          inputMode="url"
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
          className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
          value={form.favicon_link}
          onChange={(e) => setForm({ ...form, favicon_link: e.target.value })}
          onBlur={(e) => {
            const v = e.target.value.trim()
            if (v) setForm({ ...form, favicon_link: normalizeHttpUrl(v) })
          }}
        />
      </div>
      <div>
        <label className="mb-1 block text-sm font-medium">{t('admin.infra.loginUrl')}</label>
        <input
          type="text"
          inputMode="url"
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
          className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
          value={form.login_url}
          onChange={(e) => setForm({ ...form, login_url: e.target.value })}
          onBlur={(e) => {
            const v = e.target.value.trim()
            if (v) setForm({ ...form, login_url: normalizeHttpUrl(v) })
          }}
        />
      </div>
      <div className="flex justify-end gap-2 pt-2">
        <button type="button" className="rounded-md border px-4 py-2 text-sm hover:bg-accent" onClick={onCancel}>
          {t('admin.cancel')}
        </button>
        <button
          type="button"
          disabled={form.name.trim().length < 2 || saving}
          className={cn('inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90 disabled:opacity-50', saving && '!opacity-100')}
          onClick={onSave}
        >
          {saving && <ButtonSpinner />}
          {saveLabel}
        </button>
      </div>
    </div>
  )
}

function HistoryTab({ onError }: TabProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [start, setStart] = useState(0)
  const [createOpen, setCreateOpen] = useState(false)
  const size = 20

  const { data, isLoading } = useQuery({
    queryKey: ['admin-infra-history', start],
    queryFn: () => api.adminInfraHistory(start, size),
  })

  const { data: providersData } = useQuery({
    queryKey: ['admin-infra-providers'],
    queryFn: () => api.adminInfraProviders(),
    enabled: createOpen,
  })

  const deleteMutation = useMutation({
    meta: { haptics: true },
    onError,
    mutationFn: (uuid: string) => api.adminInfraDeleteHistory(uuid),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin-infra-history'] }),
  })

  const createMutation = useMutation({
    meta: { haptics: true },
    onError,
    mutationFn: (body: { provider_uuid: string; amount: number; billed_at: string }) =>
      api.adminInfraCreateHistory(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-infra-history'] })
      setCreateOpen(false)
    },
  })

  const [form, setForm] = useState({ provider_uuid: '', amount: '', billed_at: '' })

  if (isLoading) return <Spinner />

  const records = data?.records ?? []
  const total = data?.total ?? 0

  return (
    <div className="space-y-4">
      <div className="rounded-lg border border-border/50 bg-card">
        <div className="flex items-center justify-between border-b border-border/50 px-4 py-3">
          <h3 className="flex items-center gap-2 text-sm font-medium">
            <History className="size-4" />
            {t('admin.infra.billingHistory')} ({total})
          </h3>
          <button
            type="button"
            onClick={() => {
              setForm({ provider_uuid: '', amount: '', billed_at: '' })
              setCreateOpen(true)
            }}
            className="inline-flex items-center gap-1 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
          >
            <Plus className="size-3.5" />
            {t('admin.infra.addHistory')}
          </button>
        </div>
        {!records.length ? (
          <p className="px-4 py-6 text-center text-sm text-muted-foreground">{t('admin.infra.noHistory')}</p>
        ) : (
          <div className="divide-y divide-border/50">
            {records.map((rec: any) => (
              <div key={rec.uuid} className="flex items-center justify-between px-4 py-3">
                <div className="flex min-w-0 items-center gap-3">
                  {rec.provider?.faviconLink && (
                    <img
                      src={faviconSrcFromUrl(rec.provider.faviconLink)}
                      alt=""
                      loading="lazy"
                      className="size-6 shrink-0 rounded object-cover"
                      onError={(e) => {
                        e.currentTarget.style.display = 'none'
                      }}
                    />
                  )}
                  <div className="min-w-0">
                  <div className="text-sm font-medium">${rec.amount.toFixed(2)}</div>
                  <div className="text-xs text-muted-foreground">
                    {rec.provider?.name ?? rec.providerUuid} &middot;{' '}
                    {new Date(rec.billedAt).toLocaleDateString()}
                  </div>
                  </div>
                </div>
                <button
                  type="button"
                  disabled={deleteMutation.isPending && deleteMutation.variables === rec.uuid}
                  onClick={() => {
                    if (window.confirm(t('admin.infra.confirmDeleteHistory'))) {
                      deleteMutation.mutate(rec.uuid)
                    }
                  }}
                  className="rounded-md p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                >
                  {deleteMutation.isPending && deleteMutation.variables === rec.uuid ? <ButtonSpinner /> : <Trash2 className="size-4" />}
                </button>
              </div>
            ))}
          </div>
        )}
      </div>

      {total > size && (
        <div className="flex items-center justify-center gap-2">
          <button
            type="button"
            onClick={() => setStart(Math.max(0, start - size))}
            disabled={start === 0}
            className="rounded-md px-3 py-1 text-sm disabled:opacity-50"
          >
            {t('admin.prev')}
          </button>
          <span className="text-sm text-muted-foreground">
            {start + 1}–{Math.min(start + size, total)} / {total}
          </span>
          <button
            type="button"
            onClick={() => setStart(start + size)}
            disabled={start + size >= total}
            className="rounded-md px-3 py-1 text-sm disabled:opacity-50"
          >
            {t('admin.next')}
          </button>
        </div>
      )}

      <AdminModal open={createOpen} onClose={() => setCreateOpen(false)} title={t('admin.infra.createHistory')} icon={History} iconAccent="amber">
        <div className="space-y-3">
          <div>
            <label className="mb-1 block text-sm font-medium">{t('admin.infra.pickProvider')}</label>
            <AdminSelect
              value={form.provider_uuid || null}
              options={(providersData?.providers ?? []).map((p: any) => ({ value: p.uuid as string, label: p.name as string }))}
              onChange={(next) => setForm((f) => ({ ...f, provider_uuid: next ?? '' }))}
              allowEmpty
              emptyLabel="—"
              placeholder="—"
              ariaLabel={t('admin.infra.pickProvider')}
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium">{t('admin.infra.amount')}</label>
            <input
              type="number"
              step="0.01"
              min="0.01"
              className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
              value={form.amount}
              onChange={(e) => setForm((f) => ({ ...f, amount: e.target.value }))}
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium">{t('admin.infra.billedAt')}</label>
            <input
              type="datetime-local"
              className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
              value={form.billed_at}
              onChange={(e) => setForm((f) => ({ ...f, billed_at: e.target.value }))}
            />
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button type="button" className="rounded-md border px-4 py-2 text-sm hover:bg-accent" onClick={() => setCreateOpen(false)}>
              {t('admin.cancel')}
            </button>
            <button
              type="button"
              disabled={!form.provider_uuid || !form.amount || !form.billed_at || createMutation.isPending}
              className={cn('inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90 disabled:opacity-50', createMutation.isPending && '!opacity-100')}
              onClick={() => createMutation.mutate({
                provider_uuid: form.provider_uuid,
                amount: Number(form.amount),
                billed_at: new Date(form.billed_at).toISOString(),
              })}
            >
              {createMutation.isPending && <ButtonSpinner />}
              {t('admin.create')}
            </button>
          </div>
        </div>
      </AdminModal>
    </div>
  )
}

function SettingsTab({ onError }: TabProps) {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['admin-infra-settings'],
    queryFn: () => api.adminInfraSettings(),
  })
  const queryClient = useQueryClient()
  const toggleMutation = useMutation({
    meta: { haptics: true },
    onError,
    mutationFn: (params: { days: number; enabled: boolean }) => api.adminInfraUpdateSettings(params),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin-infra-settings'] }),
  })

  if (isLoading) return <Spinner />

  const thresholds = [
    { days: 1, key: 'notify_before_1' },
    { days: 3, key: 'notify_before_3' },
    { days: 7, key: 'notify_before_7' },
    { days: 14, key: 'notify_before_14' },
  ] as const

  return (
    <div className="rounded-lg border border-border/50 bg-card p-4">
      <h3 className="mb-3 flex items-center gap-2 text-sm font-medium">
        <Bell className="size-4" />
        {t('admin.infra.notifySettings')}
      </h3>
      <div className="space-y-2">
        {thresholds.map(({ days, key }) => {
          const enabled = data?.[key] ?? false
          const saving = toggleMutation.isPending && toggleMutation.variables?.days === days
          return (
            <AdminCheckboxField
              key={days}
              checked={enabled}
              busy={saving}
              onChange={() => toggleMutation.mutate({ days, enabled: !enabled })}
              label={t('admin.infra.notifyBefore', { days })}
              className="px-3 py-2"
            />
          )
        })}
      </div>
    </div>
  )
}

function StatCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg border border-border/50 bg-card p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 text-lg font-semibold">{value}</div>
    </div>
  )
}

function Spinner() {
  return (
    <div className="flex justify-center py-8">
      <span className="size-6 rounded-full border-2 border-primary border-t-transparent animate-spin" />
    </div>
  )
}

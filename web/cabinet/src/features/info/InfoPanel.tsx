import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ExternalLink, FileText, HelpCircle, ShieldCheck } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { useCabinetContentConfig } from './contentConfig'
import { ServerStatusSummary, useServerStatus } from './ServerStatusSummary'

type TabId = 'faq' | 'documents' | 'status' | 'useful_links'

export type InfoPanelProps = {
  /** Скрыть верхний H1 (когда заголовок секции снаружи, напр. на /support) */
  hideTitle?: boolean
}

export function InfoPanel({ hideTitle }: InfoPanelProps) {
  const { t } = useTranslation()
  const { data, isLoading } = useAuthBootstrap()
  const { data: content, isLoading: contentLoading } = useCabinetContentConfig()
  const siteLinks = data?.site_links
  const [tab, setTab] = useState<TabId>('faq')

  const tabsBase = content?.info.tabs ?? [
    { id: 'faq' as const, label: 'FAQ' },
    { id: 'documents' as const, label: 'Документы' },
    { id: 'status' as const, label: 'Статус' },
  ]
  const statusQuery = useServerStatus()
  const statusUrl = siteLinks?.server_status
  // Вкладка «Статус» нужна, если есть что показать: наш список серверов или внешняя ссылка.
  const hasStatus = (statusQuery.data?.nodes.length ?? 0) > 0 || Boolean(statusUrl)
  const hideStatusTab = !hasStatus && !statusQuery.isLoading
  const usefulLinks = (content?.info.useful_links ?? []).filter((x) => siteLinks?.[x.link_key])
  const tabs: { id: TabId; label: string }[] = [
    ...tabsBase.filter((t) => !(hideStatusTab && t.id === 'status')).map((t) => ({ id: t.id as TabId, label: t.label })),
    ...(usefulLinks.length > 0 ? [{ id: 'useful_links' as const, label: 'Полезные ссылки' }] : []),
  ]
  const faq = content?.faq ?? []
  const docs = (content?.info.documents ?? []).filter((x) => siteLinks?.[x.link_key])
  const loading = isLoading || contentLoading
  const hasFaq = faq.length > 0
  const hasDocs = docs.length > 0

  const visibleFaq = useMemo(() => faq, [faq])

  return (
    <div className="space-y-6">
      {!hideTitle && (
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">{content?.info.title ?? 'Информация'}</h1>
        </div>
      )}

      <div className="flex flex-wrap gap-2">
        {loading && !tabs.length
          ? [0, 1, 2].map((i) => <Skeleton key={`tab-${i}`} className="h-9 w-28 rounded-md" />)
          : null}
        {tabs.map((item) => (
          <Button
            key={item.id}
            type="button"
            size="sm"
            variant={tab === item.id ? 'default' : 'outline'}
            className={
              tab === item.id ? 'shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)]' : ''
            }
            onClick={() => setTab(item.id)}
          >
            {item.label}
          </Button>
        ))}
      </div>

      {loading ? (
        <div className="space-y-3" aria-hidden>
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-[3.75rem] w-full rounded-xl" />
          ))}
        </div>
      ) : (
        <>
          {tab === 'faq' && (
            <div className="space-y-3">
              {!hasFaq ? (
                <p className="text-sm text-muted-foreground">FAQ пока не заполнен</p>
              ) : (
                visibleFaq.map((item) => (
                  <details
                    key={item.id}
                    className="group rounded-xl border border-border/70 bg-card/70 p-4 open:border-primary/35"
                  >
                    <summary className="cursor-pointer list-none text-base font-medium">
                      <div className="flex items-center justify-between gap-3">
                        <span className="flex items-center gap-2">
                          <HelpCircle className="size-4 text-primary/85" />
                          {item.question}
                        </span>
                        <span className="text-muted-foreground transition-transform group-open:rotate-180">⌄</span>
                      </div>
                    </summary>
                    <div className="mt-3 space-y-3 text-sm text-muted-foreground">
                      <ReactMarkdown
                        remarkPlugins={[remarkGfm]}
                        components={{
                          p: ({ children }) => <p className="mb-2 last:mb-0">{children}</p>,
                          ul: ({ children }) => <ul className="ml-4 list-disc space-y-1">{children}</ul>,
                          ol: ({ children }) => <ol className="ml-4 list-decimal space-y-1">{children}</ol>,
                          code: ({ children }) => (
                            <code className="rounded bg-muted px-1 py-0.5 text-xs">{children}</code>
                          ),
                        }}
                      >
                        {item.answer_md}
                      </ReactMarkdown>
                      {item.link_key && siteLinks?.[item.link_key] && (
                        <a
                          href={siteLinks[item.link_key]}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex items-center gap-2 text-sm font-medium text-primary hover:underline"
                        >
                          Открыть материал
                          <ExternalLink className="size-3.5 shrink-0 opacity-70" aria-hidden />
                        </a>
                      )}
                    </div>
                  </details>
                ))
              )}
            </div>
          )}

          {tab === 'documents' && (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Документы</CardTitle>
                <CardDescription>Юридические документы и условия использования</CardDescription>
              </CardHeader>
              <CardContent>
                {!hasDocs ? (
                  <p className="text-sm text-muted-foreground">Документы не настроены</p>
                ) : (
                  <ul className="space-y-2">
                    {docs.map((doc) => (
                      <li key={doc.link_key}>
                        <a
                          href={siteLinks?.[doc.link_key]}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex items-center gap-2 text-sm font-medium text-primary hover:underline"
                        >
                          <FileText className="size-4 text-primary/80" />
                          {doc.label}
                          <ExternalLink className="size-3.5 shrink-0 opacity-70" aria-hidden />
                        </a>
                      </li>
                    ))}
                  </ul>
                )}
              </CardContent>
            </Card>
          )}

          {tab === 'status' && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-base">
                  <ShieldCheck className="size-5 text-emerald-500" />
                  {t('infoStatus.title')}
                </CardTitle>
                <CardDescription>
                  {t('infoStatus.description')}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <ServerStatusSummary externalUrl={statusUrl} />
              </CardContent>
            </Card>
          )}

          {tab === 'useful_links' && (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Полезные ссылки</CardTitle>
                <CardDescription>Новости, отзывы и другие полезные материалы</CardDescription>
              </CardHeader>
              <CardContent>
                {usefulLinks.length === 0 ? (
                  <p className="text-sm text-muted-foreground">Ссылки не настроены</p>
                ) : (
                  <ul className="space-y-2">
                    {usefulLinks.map((l) => (
                      <li key={l.link_key}>
                        <a
                          href={siteLinks?.[l.link_key]}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex items-center gap-2 text-sm font-medium text-primary hover:underline"
                        >
                          <ExternalLink className="size-3.5 opacity-70" aria-hidden />
                          {l.label}
                        </a>
                      </li>
                    ))}
                  </ul>
                )}
              </CardContent>
            </Card>
          )}
        </>
      )}
    </div>
  )
}

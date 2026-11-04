import { useTranslation } from 'react-i18next'
import { Activity, ExternalLink } from 'lucide-react'

import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { AdminStatusTargets } from '../components/AdminStatusTargets'
import { AdminStatusChecks } from '../components/AdminStatusChecks'
import { AdminStatusSettings } from '../components/AdminStatusSettings'

/** Страница «Статус серверов»: список серверов, откуда берутся данные и вид публичной страницы. */
export default function AdminStatusPage() {
  const { t } = useTranslation()

  return (
    <AdminLayout>
      <div className="space-y-4">
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
        <AdminStatusTargets />
        <AdminStatusChecks />
        <AdminStatusSettings />
      </div>
    </AdminLayout>
  )
}

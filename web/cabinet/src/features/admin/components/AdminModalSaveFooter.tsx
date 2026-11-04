import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2 } from 'lucide-react'

import { cn } from '@/lib/utils'

interface Props {
  onCancel: () => void
  onSave: () => void
  isPending?: boolean
  saveDisabled?: boolean
  /**
   * Левый слот футера: необратимое действие («Сбросить счётчик»,
   * «Отвязать все») или счётчик выбранного. Всё, что не «Отмена» и не
   * «Сохранить», живёт здесь — так главная кнопка всегда стоит в одном месте.
   */
  leading?: ReactNode
}

export function AdminModalSaveFooter({ onCancel, onSave, isPending, saveDisabled, leading }: Props) {
  const { t } = useTranslation()

  return (
    <div className="flex flex-wrap items-center gap-2">
      {leading && <div className="me-auto min-w-0">{leading}</div>}
      <div className="ms-auto flex gap-2">
        <button
          type="button"
          onClick={onCancel}
          disabled={isPending}
          className="rounded-btn border px-4 py-2 text-sm hover:bg-accent disabled:opacity-50"
        >
          {t('admin.cancel')}
        </button>
        <button
          type="button"
          onClick={onSave}
          disabled={isPending || saveDisabled}
          className={cn(
            'inline-flex items-center gap-2 rounded-btn bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50',
            isPending && '!opacity-100',
          )}
        >
          {isPending ? <Loader2 className="size-4 animate-spin" /> : null}
          {t('admin.save')}
        </button>
      </div>
    </div>
  )
}

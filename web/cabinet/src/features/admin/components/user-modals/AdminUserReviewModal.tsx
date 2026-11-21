import { useTranslation } from 'react-i18next'

import { Spinner } from '@/components/ui/spinner'
import { useAdminReviewByCustomer } from '../../hooks/useAdminReviews'
import { AdminModal } from '../AdminModal'
import { AdminReviewModal } from '../AdminReviewModal'

interface Props {
  customerId: number
  open: boolean
  onClose: () => void
  onSaved?: (message: string) => void
}

/**
 * «Отзыв клиента» из карточки пользователя: показывает существующий отзыв на
 * правку либо создаёт новый от имени клиента. Клиенту без аккаунта в кабинете
 * отзыв не создать — об этом окно и говорит.
 */
export function AdminUserReviewModal({ customerId, open, onClose, onSaved }: Props) {
  const { t } = useTranslation()
  const q = useAdminReviewByCustomer(open ? customerId : null)

  if (open && q.data?.customer_found) {
    return <AdminReviewModal open review={q.data.review} customerId={customerId} onClose={onClose} onSaved={onSaved} />
  }
  return (
    <AdminModal open={open} onClose={onClose} title={t('admin.reviews.modal.userAction')} size="sm">
      {q.isLoading ? (
        <div className="flex justify-center py-6">
          <Spinner />
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">{t('admin.reviews.modal.noCustomer')}</p>
      )}
    </AdminModal>
  )
}

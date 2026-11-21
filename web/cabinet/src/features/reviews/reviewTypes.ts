export type ReviewVisibility = 'anonymous' | 'name' | 'name_avatar'
export type ReviewStatus = 'pending' | 'published' | 'hidden'

export interface ReviewReply {
  body: string
  at: string
}

/** Публичный отзыв: то, что видят все. */
export interface ReviewItem {
  id: number
  rating: number
  body: string
  author_name: string | null
  author_avatar_url: string | null
  /** YYYY-MM-DD: дата регистрации клиента. */
  client_since: string | null
  tariff_label: string
  is_trial: boolean
  created_at: string
  reply?: ReviewReply
}

export interface MyReview extends ReviewItem {
  status: ReviewStatus
  visibility: ReviewVisibility
  display_name: string
}

export type ReviewBlockReason = 'verify' | 'need_paid' | 'need_sub' | 'no_customer'

export interface MyReviewResponse {
  enabled: boolean
  moderated: boolean
  can_write: boolean
  reason?: ReviewBlockReason
  policy: string
  /** Имя и аватар клиента для предпросмотра формы. */
  author?: { name: string; avatar_url?: string }
  review: MyReview | null
}

export interface ReviewsListResponse {
  enabled?: boolean
  items: ReviewItem[]
}

export interface SaveReviewInput {
  rating: number
  body: string
  visibility: ReviewVisibility
  display_name: string
}

export interface AdminReview extends ReviewItem {
  status: ReviewStatus
  visibility: ReviewVisibility
  display_name: string
  original_body?: string
  by_admin: boolean
  customer_id: number | null
  telegram_id: number | null
  updated_at: string
  /** Дата публикации (RFC3339); null — не опубликован. */
  published_at: string | null
}

export interface AdminReviewCounts {
  all: number
  pending: number
  published: number
  hidden: number
}

export interface AdminReviewsResponse {
  enabled: boolean
  items: AdminReview[]
  total: number
  counts: AdminReviewCounts
}

export interface AdminReviewUpdateInput {
  rating?: number
  body?: string
  visibility?: ReviewVisibility
  display_name?: string
  status?: ReviewStatus
  reply?: string
  /** «ГГГГ-ММ-ДД»; пусто — не менять. */
  published_at?: string
}

export interface AdminReviewCreateInput {
  customer_id: number
  rating: number
  body: string
  visibility: ReviewVisibility
  display_name: string
  status: ReviewStatus
  /** «ГГГГ-ММ-ДД»; пусто — сегодня. */
  published_at?: string
}

export interface AdminReviewByCustomerResponse {
  customer_found: boolean
  has_account: boolean
  review: AdminReview | null
}

export function visibilityFromToggles(showName: boolean, showAvatar: boolean): ReviewVisibility {
  if (!showName) return 'anonymous'
  return showAvatar ? 'name_avatar' : 'name'
}

export function togglesFromVisibility(v: ReviewVisibility | undefined): { showName: boolean; showAvatar: boolean } {
  return { showName: v === 'name' || v === 'name_avatar', showAvatar: v === 'name_avatar' }
}

/** Подсказка клиента в окне «от имени клиента». */
export interface AdminReviewCustomerHit {
  id: number
  telegram_username?: string
  telegram_id?: string
  email?: string
  has_account: boolean
}

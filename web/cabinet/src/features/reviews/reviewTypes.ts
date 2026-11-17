export type ReviewVisibility = 'anonymous' | 'name' | 'name_avatar'

export type ReviewItem = {
  id: number
  rating: number
  body: string
  author_name?: string | null
  author_avatar_url?: string | null
  client_since?: string | null
  tariff_name: string
  created_at: string
  edited_at?: string | null
  has_original?: boolean
}

export type ReviewsListResponse = {
  items: ReviewItem[]
}

export type MyReviewResponse = ReviewItem & {
  visibility?: ReviewVisibility
  display_name?: string
  has_review: boolean
}

export function visibilityFromToggles(showName: boolean, showAvatar: boolean): ReviewVisibility {
  if (!showName) return 'anonymous'
  if (showAvatar) return 'name_avatar'
  return 'name'
}

export function togglesFromVisibility(visibility: ReviewVisibility | undefined): {
  showName: boolean
  showAvatar: boolean
} {
  if (visibility === 'name_avatar') return { showName: true, showAvatar: true }
  if (visibility === 'name') return { showName: true, showAvatar: false }
  return { showName: false, showAvatar: false }
}

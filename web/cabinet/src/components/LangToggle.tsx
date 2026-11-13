import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { setLanguage } from '@/i18n'
import { cn } from '@/lib/utils'
import { hapticSelection } from '@/lib/haptic'

interface LangToggleProps {
  className?: string
}

export function LangToggle({ className }: LangToggleProps) {
  const { i18n } = useTranslation()
  const isRu = i18n.language === 'ru'

  const toggle = () => {
    const next = isRu ? 'en' : 'ru'
    setLanguage(next)
  }

  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={() => {
        hapticSelection()
        toggle()
      }}
      className={cn('font-mono text-xs', className)}
      aria-label="Switch language"
    >
      {isRu ? 'EN' : 'RU'}
    </Button>
  )
}

type BrandIconProps = {
  className?: string
}

export function GoogleBrandIcon({ className }: BrandIconProps) {
  return (
    <svg viewBox="0 0 24 24" className={className} aria-hidden>
      <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" />
      <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
      <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" />
      <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" />
    </svg>
  )
}

export function TelegramBrandIcon({ className }: BrandIconProps) {
  return (
    <svg viewBox="0 0 24 24" className={className} aria-hidden>
      <circle cx="12" cy="12" r="12" fill="#229ED9" />
      <path
        fill="#fff"
        transform="translate(-0.55 -0.55) scale(1.08)"
        d="M17.85 7.2c.2-.08.42.1.36.32l-1.95 9.18a.35.35 0 0 1-.54.2l-2.88-2.15-1.47 1.4c-.14.14-.4.06-.43-.14l-.34-2.74 5.69-5.12c.12-.1-.03-.28-.16-.2L8.9 12.4 6.3 11.5a.35.35 0 0 1-.01-.66l11.56-3.64z"
      />
    </svg>
  )
}

export function YandexBrandIcon({ className }: BrandIconProps) {
  return (
    <svg viewBox="0 0 24 24" className={className} fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden>
      <path d="M2.04 12c0-5.523 4.476-10 10-10 5.522 0 10 4.477 10 10s-4.478 10-10 10c-5.524 0-10-4.477-10-10z" fill="#FC3F1D" />
      <path d="M13.32 7.666h-.924c-1.694 0-2.585.858-2.585 2.123 0 1.43.616 2.1 1.881 2.959l1.045.704-3.003 4.487H7.49l2.695-4.014c-1.55-1.111-2.42-2.19-2.42-4.015 0-2.288 1.595-3.85 4.62-3.85h3.003v11.868H13.32V7.666z" fill="#fff" />
    </svg>
  )
}

/** Фирменный знак Heleket. В интерфейсе метод называется «Другая крипта». */
export function HeleketBrandIcon({ className }: BrandIconProps) {
  return (
    <svg viewBox="0 0 80 80" className={className} fill="none" aria-hidden>
      <path
        fill="#F21818"
        d="M5 55L30 67.5L21.25 56.25L31.25 28.75H46.25L56.25 57.5L46.25 67.5L35 78.75L73.75 60L52.5 0L46.25 16.25H32.5L25 0L5 55Z"
      />
    </svg>
  )
}

/** Знак Системы быстрых платежей (СБП). */
export function SbpBrandIcon({ className }: BrandIconProps) {
  return (
    <svg viewBox="0 0 41.2 52.6" className={className} aria-hidden>
      <path fill="#5B57A2" d="M0 12.86l6.15 10.99v6.7L.01 41.52 0 12.86z" />
      <path fill="#D90751" d="M23.6 19.85l5.76-3.53 11.79-.01-17.55 10.75v-7.21z" />
      <path fill="#FAB718" d="M23.57 12.8l.03 14.55-6.16-3.79V0l6.13 12.8z" />
      <path fill="#ED6F26" d="M41.15 16.31l-11.79.01-5.79-3.52L17.44 0l23.71 16.31z" />
      <path fill="#63B22F" d="M23.6 41.58v-7.06l-6.16-3.71.01 21.77 6.15-11z" />
      <path fill="#1487C9" d="M29.35 38.08L6.15 23.85 0 12.86l41.12 25.2-11.77.02z" />
      <path fill="#017F36" d="M17.45 52.58l6.15-11 5.75-3.5 11.77-.02-23.67 14.52z" />
      <path fill="#984995" d="M.01 41.52l17.48-10.71-5.88-3.6-5.46 3.34-6.14 10.97z" />
    </svg>
  )
}

export function VKBrandIcon({ className }: BrandIconProps) {
  return (
    <svg viewBox="0 0 24 24" className={className} aria-hidden>
      <rect x="2" y="2" width="20" height="20" rx="5" fill="#0077FF" />
      <path
        fill="#fff"
        d="M6 8.2h2.15c.07 0 .14.05.16.12.56 1.63 1.2 3.03 2.18 4.1.27.3.43.42.57.42.12 0 .18-.11.18-.34V8.37c0-.1.08-.17.17-.17h2.05c.1 0 .18.08.18.17v2.83c0 .3.07.41.22.41.14 0 .38-.12.72-.46.67-.73 1.15-1.84 1.42-2.83.02-.07.08-.12.16-.12h2.16c.12 0 .2.12.16.24-.26.86-.95 2.24-1.8 3.31-.1.12-.11.3 0 .42.81.89 1.55 2.03 1.91 3.28.04.12-.04.25-.16.25h-2.4c-.07 0-.13-.04-.15-.1-.27-.87-.82-1.93-1.45-2.58-.34-.35-.58-.47-.75-.47-.15 0-.22.1-.22.32v2.68c0 .1-.08.17-.18.17h-.98c-2.9 0-5.6-2.2-7.45-7.47-.04-.12.05-.25.18-.25z"
      />
    </svg>
  )
}

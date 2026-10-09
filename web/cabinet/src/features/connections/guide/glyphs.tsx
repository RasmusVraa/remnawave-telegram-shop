import type { ReactNode } from 'react'
import { Globe, Tv } from 'lucide-react'

import { cn } from '@/lib/utils'
import type { AppGuide, PlatformKey } from './types'

export const platformLabel: Record<PlatformKey, string> = {
  ios: 'iPhone',
  android: 'Android',
  macos: 'Mac',
  windows: 'Windows',
  linux: 'Linux',
  androidTV: 'Android TV',
  appleTV: 'Apple TV',
}

/** Телефонная платформа — в иллюстрациях рисуется телефон, а не ноутбук. */
export function isPhonePlatform(platform: PlatformKey): boolean {
  return platform === 'ios' || platform === 'android'
}

function PlatformIconSvg({ children, size }: { children: ReactNode; size: number }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      {children}
    </svg>
  )
}

export function PlatformIcon({ platform, size = 14 }: { platform: PlatformKey; size?: number }) {
  switch (platform) {
    case 'ios':
      return (
        <PlatformIconSvg size={size}>
          <path d="M8.286 7.008c-3.216 0 -4.286 3.23 -4.286 5.92c0 3.229 2.143 8.072 4.286 8.072c1.165 -.05 1.799 -.538 3.214 -.538c1.406 0 1.607 .538 3.214 .538s4.286 -3.229 4.286 -5.381c-.03 -.011 -2.649 -.434 -2.679 -3.23c-.02 -2.335 2.589 -3.179 2.679 -3.228c-1.096 -1.606 -3.162 -2.113 -3.75 -2.153c-1.535 -.12 -3.032 1.077 -3.75 1.077c-.729 0 -2.036 -1.077 -3.214 -1.077z" />
          <path d="M12 4a2 2 0 0 0 2 -2a2 2 0 0 0 -2 2" />
        </PlatformIconSvg>
      )
    case 'windows':
      return (
        <PlatformIconSvg size={size}>
          <path d="M17.8 20l-12 -1.5c-1 -.1 -1.8 -.9 -1.8 -1.9v-9.2c0 -1 .8 -1.8 1.8 -1.9l12 -1.5c1.2 -.1 2.2 .8 2.2 1.9v12.1c0 1.2 -1.1 2.1 -2.2 1.9z" />
          <path d="M12 5l0 14" />
          <path d="M4 12l16 0" />
        </PlatformIconSvg>
      )
    case 'android':
      return (
        <PlatformIconSvg size={size}>
          <path d="M4 10l0 6" />
          <path d="M20 10l0 6" />
          <path d="M7 9h10v8a1 1 0 0 1 -1 1h-8a1 1 0 0 1 -1 -1v-8a5 5 0 0 1 10 0" />
          <path d="M8 3l1 2" />
          <path d="M16 3l-1 2" />
          <path d="M9 18l0 3" />
          <path d="M15 18l0 3" />
        </PlatformIconSvg>
      )
    case 'macos':
      return (
        <PlatformIconSvg size={size}>
          <path d="M3 4m0 1a1 1 0 0 1 1 -1h16a1 1 0 0 1 1 1v14a1 1 0 0 1 -1 1h-16a1 1 0 0 1 -1 -1z" />
          <path d="M7 8v1" />
          <path d="M17 8v1" />
          <path d="M12.5 4c-.654 1.486 -1.26 3.443 -1.5 9h2.5c-.19 2.867 .094 5.024 .5 7" />
          <path d="M7 15.5c3.667 2 6.333 2 10 0" />
        </PlatformIconSvg>
      )
    case 'linux':
      return (
        <PlatformIconSvg size={size}>
          <path d="M12 5m-2 0a2 2 0 1 0 4 0a2 2 0 1 0 -4 0" />
          <path d="M17.723 7.41a7.992 7.992 0 0 0 -3.74 -2.162m-3.971 0a7.993 7.993 0 0 0 -3.789 2.216m-1.881 3.215a8 8 0 0 0 -.342 2.32c0 .738 .1 1.453 .287 2.132m1.96 3.428a7.993 7.993 0 0 0 3.759 2.19m4 0a7.993 7.993 0 0 0 3.747 -2.186m1.962 -3.43a8.008 8.008 0 0 0 .287 -2.131c0 -.764 -.107 -1.503 -.307 -2.203" />
          <path d="M5 17m-2 0a2 2 0 1 0 4 0a2 2 0 1 0 -4 0" />
          <path d="M19 17m-2 0a2 2 0 1 0 4 0a2 2 0 1 0 -4 0" />
        </PlatformIconSvg>
      )
    case 'androidTV':
    case 'appleTV':
      return <Tv size={size} aria-hidden />
    default:
      return <Globe size={size} aria-hidden />
  }
}

function normalizeAppKey(app: Pick<AppGuide, 'id' | 'name'>): string {
  return `${app.id} ${app.name}`.toLowerCase().replace(/\s+/g, ' ').trim()
}

/**
 * Глиф приложения без подложки. null — приложение незнакомое, вместо глифа
 * рисуются первые буквы названия (см. AppTile).
 */
function appGlyph(app: Pick<AppGuide, 'id' | 'name'>): ReactNode | null {
  const key = normalizeAppKey(app)
  const cls = 'size-[64%]'

  if (key.includes('happ')) {
    return (
      <svg className={cls} viewBox="0 0 50 50" fill="currentColor" aria-hidden>
        <path d="M22.3264 3H12.3611L9.44444 20.1525L21.3542 8.22034L22.3264 3Z" />
        <path d="M10.9028 20.1525L22.8125 8.22034L20.8681 21.1469H28.4028L27.9167 21.6441L20.8681 28.8531H19.4097V30.5932L7.5 42.5254L10.9028 20.1525Z" />
        <path d="M41.0417 8.22034L28.8889 20.1525L31.684 3H41.7708L41.0417 8.22034Z" />
        <path d="M30.3472 20.1525L42.5 8.22034L38.6111 30.3446L26.9444 42.5254L29.0104 28.8531H22.3264L29.6181 21.1469H30.3472V20.1525Z" />
        <path d="M40.0694 30.3446L28.4028 42.5254L27.9167 47H37.8819L40.0694 30.3446Z" />
        <path d="M18.6806 47H8.47222L8.95833 42.5254L20.8681 30.5932L18.6806 47Z" />
      </svg>
    )
  }
  if (key.includes('incy')) {
    // INCY — тёмные буквы на белом: плитка под ним белая (см. AppTile)
    return (
      <svg className="size-[130%]" viewBox="0 0 64 64" fill="none" aria-hidden>
        <path d="M21 23H25.2V41H21V23Z" fill="#050505" />
        <path d="M29 41V23H32.7L39.8 33.2V23H44V41H40.4L33.2 30.8V41H29Z" fill="#050505" />
      </svg>
    )
  }
  if (key.includes('shadowrocket') || key.includes('stash')) {
    return (
      <svg className={cls} viewBox="0 0 50 50" fill="none" aria-hidden>
        <path d="M21.2394 36.832L16.5386 39.568C16.5386 39.568 13.7182 36.832 11.8379 33.184C9.95756 29.536 16.5386 23.152 16.5386 23.152M21.2394 36.832H28.7606M21.2394 36.832C21.2394 36.832 15.5985 24.064 17.4788 16.768C19.3591 9.472 25 4 25 4C25 4 30.6409 9.472 32.5212 16.768C34.4015 24.064 28.7606 36.832 28.7606 36.832M28.7606 36.832L33.4614 39.568C33.4614 39.568 36.2818 36.832 38.1621 33.184C40.0424 29.536 33.4614 23.152 33.4614 23.152M25 46L26.8803 40.528H23.1197L25 46ZM25.9402 17.68C26.4594 18.1837 26.4594 19.0003 25.9402 19.504C25.4209 20.0077 24.5791 20.0077 24.0598 19.504C23.5406 19.0003 23.5406 18.1837 24.0598 17.68C24.5791 17.1763 25.4209 17.1763 25.9402 17.68Z" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    )
  }
  if (key.includes('streisand')) {
    return (
      <svg className={cls} viewBox="0 0 50 50" fill="none" aria-hidden>
        <path d="M25 46L6.14773 32.1591V19.9886L25 6.625L43.6136 19.9886V32.1591L25 46ZM4 39.5568L12.5909 33.1136M9.72727 43.8523L18.3182 37.4091M46 39.5568L37.4091 33.1136M40.2727 43.8523L31.6818 37.4091M45.5227 8.29545L36.9318 14.7386M39.7955 4L31.2045 10.4432M4.95455 8.29545L13.5455 14.7386M10.6818 4L19.2727 10.4432" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    )
  }
  if (key.includes('flclashx')) {
    return (
      <svg className={cls} viewBox="0 0 50 50" fill="currentColor" aria-hidden>
        <rect x="16.1458" y="47" width="6.66417" height="46.9593" rx="3.33209" transform="rotate(-150 16.1458 47)" />
        <path d="M38.1165 40.751C39.0362 42.3446 38.4902 44.3827 36.8967 45.3027C35.3031 46.2228 33.2652 45.6764 32.345 44.083L25.6887 32.5537L29.5364 25.8896L38.1165 40.751ZM13.4163 4.63477C15.01 3.71464 17.0479 4.26078 17.968 5.85449L24.5334 17.2266L20.6868 23.8906L12.1975 9.18652C11.2775 7.59298 11.8229 5.55504 13.4163 4.63477Z" />
      </svg>
    )
  }
  if (key.includes('koala clash')) {
    return (
      <svg className={cls} viewBox="0 0 50 50" fill="none" aria-hidden>
        <path
          fillRule="evenodd"
          clipRule="evenodd"
          d="M9.89914 12.0988C8.76625 12.3109 7.40023 12.9154 6.4671 13.6175C4.88097 14.8109 3.43945 16.9431 3.43945 18.0958C3.43945 18.5921 3.7749 18.897 4.32087 18.897C4.54535 18.897 4.56025 18.9067 4.52245 19.0284C4.49995 19.1006 4.4677 19.5801 4.45075 20.0939C4.42358 20.9148 4.43504 21.0917 4.54506 21.5535C4.77726 22.5281 5.36121 23.5213 6.10823 24.2123C7.0261 25.0612 8.09752 25.5287 9.47582 25.6819C10.0706 25.748 10.1056 25.7591 10.0711 25.8713C9.98977 26.1363 9.96722 28.7692 10.0409 29.3936C10.2707 31.3407 11.0434 33.2014 12.3129 34.8649C12.9693 35.7251 14.245 36.9013 15.2422 37.5658C17.8436 39.2992 21.8949 40.176 26.3591 39.9715C28.2677 39.8841 29.5744 39.695 31.0475 39.2929C34.981 38.2194 38.1435 35.3868 39.367 31.8411C39.8477 30.4483 39.9953 29.291 39.9344 27.3918C39.9128 26.7175 39.8806 26.0712 39.8628 25.9556L39.8304 25.7456L40.3178 25.705C42.8281 25.496 44.777 23.973 45.4062 21.7286C45.5595 21.1815 45.6046 19.9667 45.4944 19.3495L45.4136 18.897L45.6511 18.8969C46.1008 18.8968 46.5605 18.518 46.5605 18.1477C46.5605 17.6975 46.2365 16.8334 45.8015 16.1238C43.7587 12.7907 39.7682 11.1824 36.59 12.4113C36.027 12.6289 35.3838 13.0062 34.6993 13.5202C34.1087 13.9638 32.7678 15.2974 32.479 15.7285C32.3378 15.9393 32.2474 16.0228 32.1869 15.9983C31.0329 15.5301 28.8717 15.0268 27.045 14.8008C26.2485 14.7023 23.7063 14.701 22.8673 14.7988C21.2192 14.9908 19.7141 15.3186 18.4414 15.7624L17.6965 16.0221L17.4227 15.6351C17.0693 15.1358 15.9297 13.9978 15.3287 13.5442C14.4248 12.8621 13.614 12.4273 12.7882 12.1822C12.211 12.0108 10.6148 11.9648 9.89914 12.0988ZM25.8049 24.9694C26.7666 25.3068 27.3845 26.0745 27.8608 27.5239C28.5272 29.5517 28.8276 32.0738 28.5196 33.055C28.3591 33.5664 28.1983 33.8307 27.8071 34.2255C27.4325 34.6037 26.8449 34.9031 26.1978 35.0456C25.5992 35.1774 24.3828 35.1807 23.793 35.0522C22.0734 34.6774 21.2382 33.507 21.3472 31.6246C21.4385 30.0455 21.9862 27.7393 22.5465 26.5745C22.931 25.775 23.553 25.1993 24.2849 24.9655C24.7443 24.8187 25.3799 24.8204 25.8049 24.9694Z"
          stroke="currentColor"
          strokeWidth="2.5"
          strokeLinejoin="round"
        />
        <ellipse cx="17.2999" cy="27.0342" rx="1.54004" ry="1.54004" fill="currentColor" />
        <ellipse cx="32.9243" cy="27.0342" rx="1.54004" ry="1.54004" fill="currentColor" />
      </svg>
    )
  }
  if (key.includes('v2ray')) {
    return (
      <svg className={cls} viewBox="0 0 50 50" fill="currentColor" aria-hidden>
        <path d="M7.17 8.24503H2V3H15.16V20.9497L34.5475 3H49L7.17 47V8.24503Z" />
      </svg>
    )
  }
  if (key.includes('clash')) {
    return (
      <svg className={cls} viewBox="0 0 50 50" fill="currentColor" aria-hidden>
        <path d="M4.99239 5.21742C4.0328 5.32232 3.19446 5.43999 3.12928 5.47886C2.94374 5.58955 2.96432 33.4961 3.14997 33.6449C3.2266 33.7062 4.44146 34.002 5.84976 34.3022C7.94234 34.7483 8.60505 34.8481 9.47521 34.8481C10.3607 34.8481 10.5706 34.8154 10.7219 34.6541C10.8859 34.479 10.9066 33.7222 10.9338 26.9143L10.9638 19.3685L11.2759 19.1094C11.6656 18.7859 12.1188 18.7789 12.5285 19.0899C12.702 19.2216 14.319 20.624 16.1219 22.2061C17.9247 23.7883 19.5136 25.1104 19.6527 25.144C19.7919 25.1777 20.3714 25.105 20.9406 24.9825C22.6144 24.6221 23.3346 24.5424 24.9233 24.5421C26.4082 24.5417 27.8618 24.71 29.2219 25.0398C29.6074 25.1333 30.0523 25.1784 30.2107 25.1399C30.369 25.1016 31.1086 24.5336 31.8543 23.8777C33.3462 22.5653 33.6461 22.3017 35.4359 20.7293C36.1082 20.1388 36.6831 19.6313 36.7137 19.6017C37.5681 18.7742 38.0857 18.6551 38.6132 19.1642L38.9383 19.478V34.5138L39.1856 34.6809C39.6343 34.9843 41.2534 34.9022 43.195 34.4775C44.1268 34.2737 45.2896 34.0291 45.779 33.9339C46.2927 33.8341 46.7276 33.687 46.8079 33.5861C47.0172 33.3228 47.0109 5.87708 46.8014 5.6005C46.6822 5.4431 46.2851 5.37063 44.605 5.1996C43.477 5.08482 42.2972 5.00505 41.983 5.02223L41.4121 5.05368L35.4898 10.261C27.3144 17.4495 27.7989 17.0418 27.5372 16.9533C27.4148 16.912 26.1045 16.8746 24.6253 16.8702C22.0674 16.8626 21.9233 16.8513 21.6777 16.6396C21.0693 16.115 17.2912 12.8028 14.5726 10.4108C12.9548 8.98729 10.9055 7.18761 10.0186 6.41134L8.40584 5L7.5715 5.01331C7.11256 5.02072 5.95198 5.11252 4.99239 5.21742Z" />
      </svg>
    )
  }
  return null
}

function appInitials(name: string): string {
  return (name.replace(/[^\p{L}\p{N}]/gu, '').slice(0, 2) || '?').toUpperCase()
}

/**
 * Иконка приложения на подложке: в анимации шага 1, в кнопке выбора
 * устройства и в окне выбора. Один и тот же вид везде, чтобы человек узнавал
 * приложение, которое скачивает.
 */
export function AppTile({
  app,
  className,
  children,
}: {
  app: Pick<AppGuide, 'id' | 'name'>
  className?: string
  children?: ReactNode
}) {
  const glyph = appGlyph(app)
  const isIncy = normalizeAppKey(app).includes('incy')
  return (
    <span
      className={cn(
        'relative inline-grid shrink-0 place-items-center overflow-visible rounded-[28%]',
        isIncy ? 'bg-white text-[#050505]' : 'bg-slate-900 text-white',
        'ring-1 ring-foreground/15',
        className,
      )}
      aria-hidden
    >
      <span className="grid size-full place-items-center overflow-hidden rounded-[inherit]">
        {/* размер букв задаёт вызывающий через text-* в className */}
        {glyph ?? <span className="font-extrabold leading-none">{appInitials(app.name)}</span>}
      </span>
      {children}
    </span>
  )
}

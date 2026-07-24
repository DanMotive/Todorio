// Shared inline icon set — 24x24 stroke icons in the same style as the nav icons in App.tsx
// (fill="none", stroke="currentColor"). Used everywhere instead of emoji: emoji render
// completely differently across OS/browser combinations (different art style, size, baseline
// alignment), which looks inconsistent and unprofessional; these render identically everywhere.
import type { SVGProps } from "react"

export type IconProps = {
  size?: number
  className?: string
  style?: React.CSSProperties
  onClick?: React.MouseEventHandler<SVGSVGElement>
}

function base(p: IconProps): SVGProps<SVGSVGElement> {
  return {
    width: p.size ?? 16,
    height: p.size ?? 16,
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 2,
    strokeLinecap: "round",
    strokeLinejoin: "round",
    className: p.className,
    style: p.style,
    onClick: p.onClick,
  }
}

export const IconCheckCircle = (p: IconProps = {}) => (
  <svg {...base(p)}><circle cx="12" cy="12" r="10" /><polyline points="8 12 11 15 16 9" /></svg>
)

export const IconCircle = (p: IconProps = {}) => (
  <svg {...base(p)}><circle cx="12" cy="12" r="9" /></svg>
)

export const IconX = (p: IconProps = {}) => (
  <svg {...base(p)}><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
)

export const IconMessage = (p: IconProps = {}) => (
  <svg {...base(p)}><rect x="3" y="4" width="18" height="13" rx="2" /><polygon points="7,17 11,17 7,21" /></svg>
)

export const IconPin = (p: IconProps = {}) => (
  <svg {...base(p)}><circle cx="12" cy="9" r="5" /><line x1="12" y1="14" x2="12" y2="21" /></svg>
)

export const IconStar = (p: IconProps & { filled?: boolean } = {}) => (
  <svg {...base(p)} fill={p.filled ? "currentColor" : "none"}>
    <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
  </svg>
)

export const IconClock = (p: IconProps = {}) => (
  <svg {...base(p)}><circle cx="12" cy="12" r="9" /><line x1="12" y1="12" x2="12" y2="7" /><line x1="12" y1="12" x2="16" y2="14" /></svg>
)

export const IconGrid = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <rect x="3" y="3" width="7" height="7" /><rect x="14" y="3" width="7" height="7" />
    <rect x="14" y="14" width="7" height="7" /><rect x="3" y="14" width="7" height="7" />
  </svg>
)

export const IconList = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <line x1="8" y1="6" x2="21" y2="6" /><line x1="8" y1="12" x2="21" y2="12" /><line x1="8" y1="18" x2="21" y2="18" />
    <circle cx="4" cy="6" r="1" /><circle cx="4" cy="12" r="1" /><circle cx="4" cy="18" r="1" />
  </svg>
)

export const IconLock = (p: IconProps = {}) => (
  <svg {...base(p)}><rect x="4" y="11" width="16" height="9" rx="2" /><path d="M7 11V7a5 5 0 0 1 10 0v4" /></svg>
)

export const IconRefresh = (p: IconProps = {}) => (
  <svg {...base(p)}><path d="M21 12a9 9 0 1 1-3-6.7" /><polyline points="21 3 21 9 15 9" /></svg>
)

export const IconUser = (p: IconProps = {}) => (
  <svg {...base(p)}><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" /><circle cx="12" cy="7" r="4" /></svg>
)

export const IconUserPlus = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" /><circle cx="9" cy="7" r="4" />
    <line x1="19" y1="8" x2="19" y2="14" /><line x1="22" y1="11" x2="16" y2="11" />
  </svg>
)

export const IconPause = (p: IconProps = {}) => (
  <svg {...base(p)}><rect x="6" y="4" width="4" height="16" /><rect x="14" y="4" width="4" height="16" /></svg>
)

export const IconSlash = (p: IconProps = {}) => (
  <svg {...base(p)}><circle cx="12" cy="12" r="10" /><line x1="4.93" y1="4.93" x2="19.07" y2="19.07" /></svg>
)

export const IconAlertTriangle = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
    <line x1="12" y1="9" x2="12" y2="13" /><line x1="12" y1="17" x2="12.01" y2="17" />
  </svg>
)

export const IconAlertCircle = (p: IconProps = {}) => (
  <svg {...base(p)}><circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" /><line x1="12" y1="16" x2="12.01" y2="16" /></svg>
)

export const IconInfo = (p: IconProps = {}) => (
  <svg {...base(p)}><circle cx="12" cy="12" r="10" /><line x1="12" y1="16" x2="12" y2="12" /><line x1="12" y1="8" x2="12.01" y2="8" /></svg>
)

export const IconBarChart = (p: IconProps = {}) => (
  <svg {...base(p)}><line x1="18" y1="20" x2="18" y2="10" /><line x1="12" y1="20" x2="12" y2="4" /><line x1="6" y1="20" x2="6" y2="14" /></svg>
)

export const IconDownload = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline points="7 10 12 15 17 10" /><line x1="12" y1="15" x2="12" y2="3" />
  </svg>
)

export const IconPaperclip = (p: IconProps = {}) => (
  <svg {...base(p)}><path d="M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48" /></svg>
)

export const IconSliders = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <line x1="4" y1="21" x2="4" y2="14" /><line x1="4" y1="10" x2="4" y2="3" />
    <line x1="12" y1="21" x2="12" y2="12" /><line x1="12" y1="8" x2="12" y2="3" />
    <line x1="20" y1="21" x2="20" y2="16" /><line x1="20" y1="12" x2="20" y2="3" />
    <line x1="1" y1="14" x2="7" y2="14" /><line x1="9" y1="8" x2="15" y2="8" /><line x1="17" y1="16" x2="23" y2="16" />
  </svg>
)

export const IconAward = (p: IconProps = {}) => (
  <svg {...base(p)}><circle cx="12" cy="8" r="7" /><polyline points="8.21 13.89 7 23 12 20 17 23 15.79 13.88" /></svg>
)

export const IconArrowLeft = (p: IconProps = {}) => (
  <svg {...base(p)}><line x1="19" y1="12" x2="5" y2="12" /><polyline points="12 19 5 12 12 5" /></svg>
)

export const IconMenu = (p: IconProps = {}) => (
  <svg {...base(p)}><line x1="3" y1="6" x2="21" y2="6" /><line x1="3" y1="12" x2="21" y2="12" /><line x1="3" y1="18" x2="21" y2="18" /></svg>
)

export const IconColumns = (p: IconProps = {}) => (
  <svg {...base(p)}><rect x="3" y="4" width="5" height="16" /><rect x="10" y="4" width="5" height="10" /><rect x="17" y="4" width="4" height="13" /></svg>
)

export const IconTable = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <rect x="3" y="4" width="18" height="16" rx="1" /><line x1="3" y1="10" x2="21" y2="10" />
    <line x1="9" y1="10" x2="9" y2="20" /><line x1="15" y1="10" x2="15" y2="20" />
  </svg>
)

export const IconFileText = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" /><polyline points="14 2 14 8 20 8" />
    <line x1="8" y1="13" x2="16" y2="13" /><line x1="8" y1="17" x2="16" y2="17" />
  </svg>
)

export const IconActivity = (p: IconProps = {}) => (
  <svg {...base(p)}><polyline points="22 12 18 12 15 20 9 4 6 12 2 12" /></svg>
)

export const IconSearch = (p: IconProps = {}) => (
  <svg {...base(p)}><circle cx="11" cy="11" r="8" /><line x1="21" y1="21" x2="16.65" y2="16.65" /></svg>
)

export const IconKey = (p: IconProps = {}) => (
  <svg {...base(p)}><circle cx="8" cy="15" r="4" /><line x1="10.85" y1="12.15" x2="19" y2="4" /><line x1="16" y1="7" x2="19" y2="10" /></svg>
)

export const IconPlay = (p: IconProps = {}) => (
  <svg {...base(p)} fill="currentColor" stroke="none"><polygon points="5 3 19 12 5 21 5 3" /></svg>
)

export const IconKeyboard = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <rect x="2" y="6" width="20" height="12" rx="2" />
    <line x1="6" y1="10" x2="6.01" y2="10" /><line x1="10" y1="10" x2="10.01" y2="10" />
    <line x1="14" y1="10" x2="14.01" y2="10" /><line x1="18" y1="10" x2="18.01" y2="10" />
    <line x1="7" y1="14" x2="17" y2="14" />
  </svg>
)

export const IconCamera = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
    <circle cx="12" cy="13" r="4" />
  </svg>
)

export const IconBell = (p: IconProps = {}) => (
  <svg {...base(p)}><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" /><path d="M13.73 21a2 2 0 0 1-3.46 0" /></svg>
)

export const IconGlobe = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <circle cx="12" cy="12" r="10" /><line x1="2" y1="12" x2="22" y2="12" />
    <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
  </svg>
)

export const IconShield = (p: IconProps = {}) => (
  <svg {...base(p)}><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" /></svg>
)

export const IconTrash = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <polyline points="3 6 5 6 21 6" /><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
  </svg>
)

export const IconUpload = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline points="17 8 12 3 7 8" /><line x1="12" y1="3" x2="12" y2="15" />
  </svg>
)

export const IconLayout = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <rect x="3" y="3" width="18" height="18" rx="2" /><line x1="3" y1="9" x2="21" y2="9" /><line x1="9" y1="9" x2="9" y2="21" />
  </svg>
)

export const IconArchive = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <polyline points="21 8 21 21 3 21 3 8" /><rect x="1" y="3" width="22" height="5" /><line x1="10" y1="12" x2="14" y2="12" />
  </svg>
)

export const IconCalendar = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <rect x="3" y="4" width="18" height="18" rx="2" /><line x1="16" y1="2" x2="16" y2="6" />
    <line x1="8" y1="2" x2="8" y2="6" /><line x1="3" y1="10" x2="21" y2="10" />
  </svg>
)

// Inbox tray — a drawn SVG like every other icon here, deliberately not an emoji.
export const IconInbox = (p: IconProps = {}) => (
  <svg {...base(p)}>
    <polyline points="22 12 16 12 14 15 10 15 8 12 2 12" />
    <path d="M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z" />
  </svg>
)

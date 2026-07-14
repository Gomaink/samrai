import type { SVGProps } from 'react'

type IconProps = SVGProps<SVGSVGElement>

const base = {
  width: 20,
  height: 20,
  viewBox: '0 0 24 24',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.8,
  strokeLinecap: 'round' as const,
  strokeLinejoin: 'round' as const,
  'aria-hidden': true,
}

export function LibraryIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M4 19.5V5a2 2 0 0 1 2-2h2v18H6a2 2 0 0 1-2-1.5Z"/><path d="M8 3h4v18H8zM12 4.5l4-.8 3.2 16-4 .8z"/></svg>
}

export function HomeIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="m3 10 9-7 9 7"/><path d="M5 9v11h14V9M9 20v-6h6v6"/></svg>
}

export function UploadIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M12 16V4m0 0L7.5 8.5M12 4l4.5 4.5"/><path d="M4 15v4a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-4"/></svg>
}

export function SearchIcon(props: IconProps) {
  return <svg {...base} {...props}><circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/></svg>
}

export function SettingsIcon(props: IconProps) {
  return <svg {...base} {...props}><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-2.83 2.83-.06-.06A1.7 1.7 0 0 0 15 19.4a1.7 1.7 0 0 0-1 .6 1.7 1.7 0 0 0-.4 1v.1h-4v-.1a1.7 1.7 0 0 0-1.1-1.6 1.7 1.7 0 0 0-1.88.34l-.06.06-2.83-2.83.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-.6-1 1.7 1.7 0 0 0-1-.4h-.1v-4H3A1.7 1.7 0 0 0 4.6 8.5a1.7 1.7 0 0 0-.34-1.88l-.06-.06 2.83-2.83.06.06A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-.6 1.7 1.7 0 0 0 .4-1v-.1h4V3a1.7 1.7 0 0 0 1.1 1.6 1.7 1.7 0 0 0 1.88-.34l.06-.06 2.83 2.83-.06.06A1.7 1.7 0 0 0 19.4 9c.15.36.36.7.6 1 .25.3.6.45 1 .4h.1v4H21a1.7 1.7 0 0 0-1.6 1.1Z"/></svg>
}

export function LogOutIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M10 17l5-5-5-5M15 12H3"/><path d="M14 3h5a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-5"/></svg>
}

export function EyeIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M2.5 12s3.5-6 9.5-6 9.5 6 9.5 6-3.5 6-9.5 6-9.5-6-9.5-6Z"/><circle cx="12" cy="12" r="2.5"/></svg>
}

export function EyeOffIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="m3 3 18 18M10.6 10.6a2 2 0 0 0 2.8 2.8M9.9 5.2A10.4 10.4 0 0 1 12 5c6 0 9.5 7 9.5 7a15 15 0 0 1-2.1 3M6.6 6.6C3.9 8.4 2.5 12 2.5 12s3.5 7 9.5 7c1.1 0 2.1-.2 3-.5"/></svg>
}

export function ChevronRightIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="m9 18 6-6-6-6"/></svg>
}

export function MenuIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M4 7h16M4 12h16M4 17h16"/></svg>
}

export function CloseIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M6 6l12 12M18 6 6 18"/></svg>
}

export function SparklesIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="m12 3 1.2 3.3L16.5 7.5l-3.3 1.2L12 12l-1.2-3.3-3.3-1.2 3.3-1.2L12 3ZM18.5 13l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8.8-2.2ZM5.5 13l.7 1.8 1.8.7-1.8.7-.7 1.8-.7-1.8-1.8-.7 1.8-.7.7-1.8Z"/></svg>
}

export function CheckIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="m5 12 4 4L19 6"/></svg>
}

export function AlertIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M12 3 2.7 19h18.6L12 3Z"/><path d="M12 9v4M12 17h.01"/></svg>
}

export function FileIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M6 2h8l4 4v16H6z"/><path d="M14 2v5h5M9 13h6M9 17h4"/></svg>
}

export function ClockIcon(props: IconProps) {
  return <svg {...base} {...props}><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg>
}

export function ArrowLeftIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M19 12H5M11 18l-6-6 6-6"/></svg>
}

export function ArrowRightIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M5 12h14M13 6l6 6-6 6"/></svg>
}

export function BackIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M15 18l-6-6 6-6"/><path d="M9 12h11"/></svg>
}

export function FullscreenIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M8 3H3v5M16 3h5v5M21 16v5h-5M3 16v5h5"/></svg>
}

export function FullscreenExitIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M8 3v5H3M16 3v5h5M21 16h-5v5M3 16h5v5"/></svg>
}

export function DirectionIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M4 7h14M14 3l4 4-4 4M20 17H6M10 13l-4 4 4 4"/></svg>
}

export function FitIcon(props: IconProps) {
  return <svg {...base} {...props}><rect x="4" y="3" width="16" height="18" rx="2"/><path d="M8 8h8M8 16h8"/></svg>
}

export function SeriesIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="m12 3 8 4-8 4-8-4 8-4Z"/><path d="m4 12 8 4 8-4M4 17l8 4 8-4"/></svg>
}

export function SlidersIcon(props: IconProps) {
  return <svg {...base} {...props}><path d="M4 6h10M18 6h2M14 4v4M4 12h2M10 12h10M6 10v4M4 18h7M15 18h5M11 16v4"/></svg>
}

export function UserIcon(props: IconProps) { return <svg {...base} {...props}><circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/></svg> }
export function UsersIcon(props: IconProps) { return <svg {...base} {...props}><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75"/></svg> }
export function DownloadIcon(props: IconProps) { return <svg {...base} {...props}><path d="M12 3v12m0 0 5-5m-5 5-5-5"/><path d="M4 17v3h16v-3"/></svg> }
export function DatabaseIcon(props: IconProps) { return <svg {...base} {...props}><ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v6c0 1.7 3.6 3 8 3s8-1.3 8-3V5"/><path d="M4 11v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6"/></svg> }
export function KeyIcon(props: IconProps) { return <svg {...base} {...props}><circle cx="8" cy="15" r="4"/><path d="m11 12 9-9M16 7l2 2M14 9l2 2"/></svg> }
export function TrashIcon(props: IconProps) { return <svg {...base} {...props}><path d="M4 7h16M9 7V4h6v3M7 7l1 14h8l1-14M10 11v6M14 11v6"/></svg> }
export function ShieldIcon(props: IconProps) { return <svg {...base} {...props}><path d="M12 3 4 6v5c0 5 3.4 8.5 8 10 4.6-1.5 8-5 8-10V6l-8-3Z"/><path d="m9 12 2 2 4-4"/></svg> }
export function RefreshIcon(props: IconProps) { return <svg {...base} {...props}><path d="M20 7v5h-5M4 17v-5h5"/><path d="M18.5 9A7 7 0 0 0 6 6.5L4 9M5.5 15A7 7 0 0 0 18 17.5l2-2.5"/></svg> }
export function HeartIcon(props: IconProps) { return <svg {...base} {...props}><path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1-1.1a5.5 5.5 0 0 0-7.8 7.8l1 1L12 21l7.8-7.6 1-1a5.5 5.5 0 0 0 0-7.8Z"/></svg> }
export function HistoryIcon(props: IconProps) { return <svg {...base} {...props}><path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5M12 7v5l3 2"/></svg> }
export function NotesIcon(props: IconProps) { return <svg {...base} {...props}><path d="M5 3h14v18H5z"/><path d="M8 7h8M8 11h8M8 15h5"/></svg> }

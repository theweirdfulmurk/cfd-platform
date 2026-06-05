/* Inline SVG icon set (no icon-library dependency). 16px grid, currentColor,
 * stroke-based line icons with round joins; filled glyphs for play/check. */
import { SVGProps } from 'react';

type IconProps = SVGProps<SVGSVGElement> & { size?: number };

function base({ size = 16, ...rest }: IconProps) {
  return {
    width: size,
    height: size,
    viewBox: '0 0 16 16',
    'aria-hidden': true,
    focusable: false as const,
    ...rest,
  };
}

const stroke = {
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.5,
  strokeLinecap: 'round' as const,
  strokeLinejoin: 'round' as const,
};

export const IconPlay = (p: IconProps) => (
  <svg {...base(p)} fill="currentColor">
    <path d="M4 2.8v10.4a.6.6 0 0 0 .92.5l8.2-5.2a.6.6 0 0 0 0-1L4.92 2.3a.6.6 0 0 0-.92.5z" />
  </svg>
);

export const IconCheck = (p: IconProps) => (
  <svg {...base(p)}>
    <path {...stroke} d="M3 8.5l3.2 3.3L13 4.5" />
  </svg>
);

export const IconAlert = (p: IconProps) => (
  <svg {...base(p)}>
    <path {...stroke} d="M8 1.6 15 14H1L8 1.6z" />
    <path {...stroke} d="M8 6.4v3.2" />
    <circle cx="8" cy="11.7" r="0.35" fill="currentColor" stroke="none" />
  </svg>
);

export const IconClock = (p: IconProps) => (
  <svg {...base(p)}>
    <circle {...stroke} cx="8" cy="8" r="6.3" />
    <path {...stroke} d="M8 4.4V8l2.6 1.6" />
  </svg>
);

export const IconUpload = (p: IconProps) => (
  <svg {...base(p)}>
    <path {...stroke} d="M8 10.5V2.6m0 0L5.2 5.4M8 2.6l2.8 2.8" />
    <path {...stroke} d="M2.8 9.8v2.2c0 .77.63 1.4 1.4 1.4h7.6c.77 0 1.4-.63 1.4-1.4V9.8" />
  </svg>
);

export const IconTrash = (p: IconProps) => (
  <svg {...base(p)}>
    <path {...stroke} d="M2.8 4.2h10.4M6.2 4.2V3c0-.55.45-1 1-1h1.6c.55 0 1 .45 1 1v1.2M5 4.2l.5 8.2c.03.5.45.9.96.9h3.08c.5 0 .93-.4.96-.9L11 4.2" />
  </svg>
);

export const IconRefresh = (p: IconProps) => (
  <svg {...base(p)}>
    <path {...stroke} d="M13 7a5 5 0 1 0-.5 3.3" />
    <path {...stroke} d="M13.2 3.3V7H9.5" />
  </svg>
);

export const IconChip = (p: IconProps) => (
  <svg {...base(p)}>
    <rect {...stroke} x="4.2" y="4.2" width="7.6" height="7.6" rx="1.4" />
    <path {...stroke} d="M6.5 1.6v2m3-2v2m-3 9v2m3-2v2M1.6 6.5h2m-2 3h2m9-3h2m-2 3h2" />
  </svg>
);

export const IconSearch = (p: IconProps) => (
  <svg {...base(p)}>
    <circle {...stroke} cx="7" cy="7" r="4.4" />
    <path {...stroke} d="M10.3 10.3 13.6 13.6" />
  </svg>
);

export const IconX = (p: IconProps) => (
  <svg {...base(p)}>
    <path {...stroke} d="M4 4l8 8M12 4l-8 8" />
  </svg>
);

export const IconEye = (p: IconProps) => (
  <svg {...base(p)}>
    <path {...stroke} d="M1.4 8S3.6 3.6 8 3.6 14.6 8 14.6 8 12.4 12.4 8 12.4 1.4 8 1.4 8Z" />
    <circle {...stroke} cx="8" cy="8" r="2.1" />
  </svg>
);

export const IconCube = (p: IconProps) => (
  <svg {...base(p)}>
    <path {...stroke} d="M8 1.7 13.8 5v6L8 14.3 2.2 11V5z" />
    <path {...stroke} d="M2.2 5 8 8.3m0 0 5.8-3.3M8 8.3v6" />
  </svg>
);

export const IconDownload = (p: IconProps) => (
  <svg {...base(p)}>
    <path {...stroke} d="M8 2v7.4m0 0 2.8-2.8M8 9.4 5.2 6.6" />
    <path {...stroke} d="M2.8 11v1.6c0 .55.45 1 1 1h8.4c.55 0 1-.45 1-1V11" />
  </svg>
);

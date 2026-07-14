// icons.jsx 汎用アイコンを TSX 化
import type { CSSProperties, ReactNode } from 'react';

export interface IconProps {
  size?: number;
  fill?: boolean;
  style?: CSSProperties;
}

interface InnerProps extends IconProps {
  d: string | ReactNode;
}

function Icon({ d, size = 14, fill = false, style }: InnerProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      style={style}
      fill={fill ? 'currentColor' : 'none'}
      stroke={fill ? 'none' : 'currentColor'}
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      {typeof d === 'string' ? <path d={d} /> : d}
    </svg>
  );
}

type IconComponent = (p?: IconProps) => JSX.Element;

// アイコンマップ (data.jsx / icons.jsx より移植)
export const Icons: Record<string, IconComponent> = {
  ec2: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <rect x="3" y="5" width="18" height="14" rx="2" />
          <path d="M7 9h10M7 13h10M7 17h6" />
        </>
      }
    />
  ),
  rds: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <ellipse cx="12" cy="6" rx="8" ry="3" />
          <path d="M4 6v6c0 1.7 3.6 3 8 3s8-1.3 8-3V6" />
          <path d="M4 12v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6" />
        </>
      }
    />
  ),
  cache: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <path d="M4 7l8-4 8 4-8 4z" />
          <path d="M4 7v6l8 4 8-4V7" />
          <path d="M4 13v4l8 4 8-4v-4" />
        </>
      }
    />
  ),
  lambda: (p = {}) => <Icon {...p} d="M5 4l6 8-6 8h4l4-6 3 6h4L13 4z" />,
  ecs: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <rect x="3" y="4" width="7" height="7" rx="1" />
          <rect x="14" y="4" width="7" height="7" rx="1" />
          <rect x="3" y="14" width="7" height="7" rx="1" />
          <rect x="14" y="14" width="7" height="7" rx="1" />
        </>
      }
    />
  ),
  s3: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <circle cx="12" cy="12" r="9" />
          <path d="M3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18" />
        </>
      }
    />
  ),
  iam: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <circle cx="12" cy="8" r="4" />
          <path d="M4 21c0-4.4 3.6-8 8-8s8 3.6 8 8" />
        </>
      }
    />
  ),
  gcpiam: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <circle cx="12" cy="8" r="4" />
          <path d="M4 21c0-4.4 3.6-8 8-8s8 3.6 8 8" />
        </>
      }
    />
  ),
  gcpserviceaccounts: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <rect x="3" y="5" width="18" height="14" rx="2" />
          <circle cx="9" cy="12" r="2" />
          <path d="M14 9h4M14 13h4M14 15h3" />
        </>
      }
    />
  ),
  elb: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <circle cx="5" cy="12" r="2.5" />
          <circle cx="19" cy="5" r="2.5" />
          <circle cx="19" cy="12" r="2.5" />
          <circle cx="19" cy="19" r="2.5" />
          <path d="M7.5 11l9-5.2M7.5 12h9M7.5 13l9 5.2" />
        </>
      }
    />
  ),
  cloudfront: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <circle cx="12" cy="12" r="9" />
          <path d="M12 3a13 13 0 0 1 0 18M12 3a13 13 0 0 0 0 18M3.5 9h17M3.5 15h17" />
        </>
      }
    />
  ),
  apigw: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <path d="M7 8l-4 4 4 4M17 8l4 4-4 4M13 5l-2 14" />
        </>
      }
    />
  ),
  natgw: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <rect x="3" y="9" width="18" height="11" rx="2" />
          <path d="M8 9V6a4 4 0 0 1 8 0v3M12 13v3" />
        </>
      }
    />
  ),
  sqs: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <path d="M4 6h16M4 12h16M4 18h10" />
          <path d="M17 16l3 2-3 2" />
        </>
      }
    />
  ),
  kinesis: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <path d="M3 7c3 0 3 3 6 3s3-3 6-3 3 3 6 3" />
          <path d="M3 14c3 0 3 3 6 3s3-3 6-3 3 3 6 3" />
        </>
      }
    />
  ),
  waf: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <path d="M12 3l8 3v6c0 4.5-3.5 8-8 9-4.5-1-8-4.5-8-9V6z" />
          <path d="M9 12l2 2 4-4" />
        </>
      }
    />
  ),
  search: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <circle cx="11" cy="11" r="7" />
          <path d="M20 20l-3.5-3.5" />
        </>
      }
    />
  ),
  filter: (p = {}) => <Icon {...p} d="M3 5h18l-7 9v6l-4-2v-4z" />,
  plus: (p = {}) => <Icon {...p} d="M12 5v14M5 12h14" />,
  x: (p = {}) => <Icon {...p} d="M6 6l12 12M6 18L18 6" />,
  chevron: (p = {}) => <Icon {...p} d="M8 5l7 7-7 7" />,
  sun: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <circle cx="12" cy="12" r="4" />
          <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
        </>
      }
    />
  ),
  moon: (p = {}) => <Icon {...p} d="M20 15a8 8 0 1 1-10-10 7 7 0 0 0 10 10z" />,
  refresh: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <path d="M3 12a9 9 0 0 1 15.5-6.3L21 8" />
          <path d="M21 3v5h-5" />
          <path d="M21 12a9 9 0 0 1-15.5 6.3L3 16" />
          <path d="M3 21v-5h5" />
        </>
      }
    />
  ),
  bell: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9" />
          <path d="M10 21a2 2 0 0 0 4 0" />
        </>
      }
    />
  ),
  settings: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <circle cx="12" cy="12" r="3" />
          <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3h.1a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8v.1a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" />
        </>
      }
    />
  ),
  copy: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <rect x="9" y="9" width="11" height="11" rx="2" />
          <path d="M5 15V5a2 2 0 0 1 2-2h10" />
        </>
      }
    />
  ),
  more: (p = {}) => (
    <Icon
      {...p}
      fill
      d={
        <>
          <circle cx="5" cy="12" r="1" />
          <circle cx="12" cy="12" r="1" />
          <circle cx="19" cy="12" r="1" />
        </>
      }
    />
  ),
  external: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <path d="M14 4h6v6" />
          <path d="M20 4l-10 10" />
          <path d="M20 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h5" />
        </>
      }
    />
  ),
  terminal: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <rect x="3" y="4" width="18" height="16" rx="2" />
          <path d="M7 9l3 3-3 3M13 15h4" />
        </>
      }
    />
  ),
  chart: (p = {}) => <Icon {...p} d="M3 3v18h18M7 14l4-4 3 3 5-6" />,
  tag: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <path d="M20 12l-8 8-9-9V3h8z" />
          <circle cx="7" cy="7" r="1" fill="currentColor" />
        </>
      }
    />
  ),
  sparkles: (p = {}) => (
    <Icon {...p} d="M12 3l2 5 5 2-5 2-2 5-2-5-5-2 5-2zM19 14l1 2 2 1-2 1-1 2-1-2-2-1 2-1z" />
  ),
  cost: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <path d="M12 3v18" />
          <path d="M17 7H9.5a2.5 2.5 0 0 0 0 5h5a2.5 2.5 0 0 1 0 5H7" />
        </>
      }
    />
  ),
  clock: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <circle cx="12" cy="12" r="9" />
          <path d="M12 7v5l3 2" />
        </>
      }
    />
  ),
  alertTriangle: (p = {}) => (
    <Icon
      {...p}
      d={
        <>
          <path d="M12 3l9 16H3z" />
          <path d="M12 10v4M12 17.5v.01" />
        </>
      }
    />
  ),
};

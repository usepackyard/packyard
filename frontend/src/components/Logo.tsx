export function LogoIcon({ size = 24, className }: { size?: number; className?: string }) {
  const s = size;
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width={s} height={s} viewBox="0 0 40 40" className={className}>
      <polygon points="20,2 38,12 20,22 2,12" fill="#ff9e4a" opacity="0.32" />
      <polygon points="20,2 38,12 20,22 2,12" fill="none" stroke="#e07a20" strokeWidth="1.8" strokeLinejoin="round" />
      <polygon points="38,12 38,30 20,38 20,22" fill="#325476" opacity="0.22" />
      <polygon points="38,12 38,30 20,38 20,22" fill="none" stroke="#325476" strokeWidth="1.8" strokeLinejoin="round" />
      <polygon points="2,12 20,22 20,38 2,30" fill="#e07a20" opacity="0.12" />
      <polygon points="2,12 20,22 20,38 2,30" fill="none" stroke="#5c7290" strokeWidth="1.8" strokeLinejoin="round" />
    </svg>
  );
}

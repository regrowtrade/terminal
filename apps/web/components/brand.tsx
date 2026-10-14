export function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <span className="brand">
      <svg viewBox="0 0 36 36" width="34" height="34" aria-hidden="true">
        <rect width="36" height="36" rx="11" fill="currentColor" />
        <path
          d="M10 25V12h5.5c6.2 0 7.4 6.8 2.3 8.3L25 25h-7l-5-5v5z"
          fill="#101214"
        />
        <path d="m21 10 5-2-2 5" fill="#101214" />
      </svg>
      {!compact && (
        <span>
          regrow<span className="brand-period">.</span>
        </span>
      )}
    </span>
  );
}

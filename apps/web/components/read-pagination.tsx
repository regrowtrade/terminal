export const readPageSize = 50;

export function ReadPagination({
  label,
  total,
  page,
  onPageChange,
}: {
  label: string;
  total: number;
  page: number;
  onPageChange: (page: number) => void;
}) {
  if (total === 0) return null;
  return (
    <nav className="read-pagination" aria-label={`Paginación de ${label}`}>
      <span>
        {page * readPageSize + 1}–{Math.min((page + 1) * readPageSize, total)}{" "}
        de {total} registros que coinciden
      </span>
      <button
        className="button secondary compact"
        type="button"
        disabled={page === 0}
        onClick={() => onPageChange(page - 1)}
      >
        Anteriores {label}
      </button>
      <button
        className="button secondary compact"
        type="button"
        disabled={(page + 1) * readPageSize >= total}
        onClick={() => onPageChange(page + 1)}
      >
        Siguientes {label}
      </button>
    </nav>
  );
}

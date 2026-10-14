"use client";
export default function ErrorPage({ reset }: { reset: () => void }) {
  return (
    <main className="standalone-state">
      <span className="eyebrow">SIN ACCIONES EJECUTADAS</span>
      <h1>No pudimos cargar esta vista.</h1>
      <p>Vuelve a intentarlo. Esta versión no ejecuta órdenes.</p>
      <button className="button primary" onClick={reset}>
        Reintentar
      </button>
    </main>
  );
}

import Link from "next/link";
export default function NotFound() {
  return (
    <main className="standalone-state">
      <span className="eyebrow">404 / FUERA DEL WORKSPACE</span>
      <h1>Esta página no existe.</h1>
      <Link className="button primary" href="/">
        Volver a la terminal
      </Link>
    </main>
  );
}

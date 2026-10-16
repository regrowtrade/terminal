import { redirect } from "next/navigation";
import { ArrowRight, LockKeyhole, ShieldCheck } from "lucide-react";
import { authenticated, isConfigured } from "@/lib/auth";
import { Brand } from "@/components/brand";

export const dynamic = "force-dynamic";
export default async function Login({
  searchParams,
}: {
  searchParams: Promise<{ error?: string }>;
}) {
  if (await authenticated()) redirect("/");
  const { error } = await searchParams;
  const configured = isConfigured();
  const messages: Record<string, string> = {
    invalid: "No se pudo iniciar sesión. Revisa tu contraseña.",
    limited: "Demasiados intentos. Vuelve a intentarlo en cinco minutos.",
    unavailable:
      "El servicio de sesiones no está disponible o alcanzó su límite. Revisa el motor y PostgreSQL antes de reintentar.",
    configuration:
      "Configura los secretos del servidor antes de iniciar sesión.",
  };
  return (
    <main className="login-page">
      <section className="login-story">
        <Brand />
        <div className="login-copy">
          <span className="eyebrow">TU NUEVO CENTRO DE OPERACIONES</span>
          <h1>
            Menos ruido.
            <br />
            Más <span>control.</span>
          </h1>
          <p>
            Una perspectiva clara de tus operaciones.
            <br />
            Un solo lugar para gestionar tu riesgo.
          </p>
          <div className="login-mini-chart" aria-hidden="true">
            <svg viewBox="0 0 480 140">
              <path
                d="M0 130 45 116 70 124 110 94 145 106 190 65 220 78 250 42 290 62 340 27 380 41 425 12 480 0"
                fill="none"
                stroke="currentColor"
                strokeWidth="3"
              />
            </svg>
            <span>Diseñada para operar con intención.</span>
          </div>
        </div>
        <span className="login-footer">REGROW TERMINAL · ACCESO PRIVADO</span>
      </section>
      <section className="login-panel">
        <div className="login-form-wrap">
          <div className="icon-box">
            <LockKeyhole size={22} />
          </div>
          <span className="eyebrow">BIENVENIDO A TU WORKSPACE</span>
          <h2>Todo bajo control.</h2>
          <p>
            Accede a tu terminal personal. La primera versión funciona
            exclusivamente en simulación.
          </p>
          {!configured && (
            <div className="notice warning" role="status">
              <ShieldCheck size={18} />
              <span>
                Acceso bloqueado hasta configurar <code>TERMINAL_PASSWORD</code>
                , <code>SESSION_SECRET</code> y <code>APP_ORIGIN</code> en el
                servidor. Consulta el README.
              </span>
            </div>
          )}
          {error && (
            <div className="notice danger" role="alert">
              {messages[error] ?? messages.invalid}
            </div>
          )}
          <form method="post" action="/api/auth/login">
            <label htmlFor="password">Contraseña de acceso</label>
            <input
              id="password"
              name="password"
              type="password"
              minLength={16}
              maxLength={256}
              autoComplete="current-password"
              required
              placeholder="Tu contraseña privada"
              disabled={!configured}
            />
            <button
              className="button primary full"
              type="submit"
              disabled={!configured}
            >
              Entrar a mi terminal <ArrowRight size={17} />
            </button>
          </form>
          <div className="login-security">
            <ShieldCheck size={15} /> Sesión privada · Sin permisos de trading
            real
          </div>
        </div>
      </section>
    </main>
  );
}

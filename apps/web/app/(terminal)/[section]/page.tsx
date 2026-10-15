import { notFound } from "next/navigation";
import {
  ArrowRight,
  BookOpen,
  Check,
  Database,
  Fingerprint,
  FlaskConical,
  KeyRound,
  Layers3,
  LockKeyhole,
  ShieldCheck,
  Target,
  Unplug,
  Wallet,
} from "lucide-react";
import { Positions } from "@/components/positions";
import { Planner } from "@/components/planner";
import { PlanButton } from "@/components/workspace";
import { requireOwner } from "@/lib/auth";
import { sessionBackend } from "@/lib/sessions";

export default async function Section({
  params,
}: {
  params: Promise<{ section: string }>;
}) {
  await requireOwner();
  const { section } = await params;
  if (section === "positions")
    return (
      <>
        <Heading
          eyebrow="EXPOSICIÓN EN UN SOLO LUGAR"
          title="Cada lado. Toda la perspectiva."
          description="Explora la tabla con datos sintéticos. La sincronización real con Binance y Bybit aún no está habilitada."
        />
        <div className="notice">
          <Unplug size={18} />
          <span>
            No hay cuentas conectadas. Los precios y cantidades siguientes son
            ficticios.
          </span>
        </div>
        <Positions full />
      </>
    );
  if (section === "strategies")
    return (
      <>
        <Heading
          eyebrow="CONSTRUCTOR DE PLANES"
          title="Decisiones con estructura."
          description="Tres metodologías. Un presupuesto explícito. El cálculo se valida en el motor Go."
        />
        <section className="card planner-page">
          <Planner />
        </section>
      </>
    );
  if (section === "risk")
    return (
      <>
        <Heading
          eyebrow="EL CAPITAL SE PROTEGE CON REGLAS"
          title="Tu riesgo, sin ambigüedades."
          description="Antes de automatizar, define exactamente qué estás dispuesto a arriesgar."
        />
        <div className="rules-grid">
          <Rule
            icon={<Wallet size={21} />}
            title="Una base visible"
            text="El porcentaje se convierte a USDT usando el saldo o equity que introduces para esa simulación. La base queda incluida en el resultado."
          />
          <Rule
            icon={<Target size={21} />}
            title="Un presupuesto por gestión"
            text="El riesgo adicional se confirma en cada activación. El umbral parte del resultado neto actual, no de la pérdida original."
          />
          <Rule
            icon={<Layers3 size={21} />}
            title="Exposición real, no teórica"
            text="El motor contiene cálculo de cierres parciales. La vista previa muestra escenarios por nivel completo; todavía no supervisa posiciones reales."
          />
          <Rule
            icon={<ShieldCheck size={21} />}
            title="Límites honestos"
            text="El plan limita monedas y margen estimado, pero no comprueba liquidación, liquidez ni filtros del exchange. Un stop no garantiza una pérdida exacta."
          />
        </div>
        <section className="card risk-example">
          <span className="eyebrow">UN EJEMPLO CONCRETO</span>
          <h2>El presupuesto cambia cuando recuperas.</h2>
          <div className="risk-example-grid">
            <div>
              <small>Antes</small>
              <strong>−2 USDT</strong>
              <p>+ 2 USDT adicionales</p>
              <span>Umbral: −4 USDT</span>
            </div>
            <ArrowRight size={25} />
            <div>
              <small>Nueva activación</small>
              <strong>−1 USDT</strong>
              <p>+ 1 USDT adicional</p>
              <span>Umbral: −2 USDT</span>
            </div>
          </div>
          <PlanButton mode="hedge">
            Calcular mi escenario <ArrowRight size={16} />
          </PlanButton>
        </section>
      </>
    );
  if (section === "journal")
    return (
      <>
        <Heading
          eyebrow="APRENDE DE CADA DECISIÓN"
          title="Tu historia como trader."
          description="Aquí se reunirán las ejecuciones, los costes y las notas de cada operación, sin duplicar resultados."
        />
        <section className="card empty-state">
          <div className="empty-illustration">
            <BookOpen size={38} />
            <span />
          </div>
          <span className="eyebrow">UNA HISTORIA TODAVÍA POR ESCRIBIR</span>
          <h2>Aún no hay operaciones registradas.</h2>
          <p>
            Esta entrega no conecta exchanges ni guarda planes. Las vistas
            previas de la calculadora no se convierten en trades.
          </p>
          <PlanButton>
            Explorar una entrada <ArrowRight size={16} />
          </PlanButton>
          <div className="empty-roadmap">
            <span>
              <Check size={14} /> Cálculo de escenarios
            </span>
            <span>
              <Database size={14} /> Persistencia pendiente
            </span>
            <span>
              <Unplug size={14} /> Sincronización pendiente
            </span>
          </div>
        </section>
      </>
    );
  if (section === "settings")
    return (
      <>
        <Heading
          eyebrow="UN WORKSPACE PRIVADO"
          title="Las bases de tu terminal."
          description="Configuración del entorno y límites de esta primera entrega."
        />
        <div className="settings-grid">
          <section className="card settings-card">
            <div className="card-heading">
              <h2>Seguridad del acceso</h2>
              <LockKeyhole size={18} />
            </div>
            <Setting label="Identidad" value="Propietario · Acceso personal" />
            <Setting
              label="Autenticación"
              value="Contraseña privada del servidor"
            />
            <Setting
              label="Sesión"
              value={
                sessionBackend() === "postgres"
                  ? "PostgreSQL · Revocable · 8 horas"
                  : "Local de desarrollo · Se pierde al reiniciar"
              }
            />
            <Setting label="Autorización" value="Validada en páginas y API" />
            <form method="post" action="/api/auth/logout-all">
              <button className="button secondary" type="submit">
                Cerrar todas las sesiones
              </button>
            </form>
            <p className="muted-copy">
              <Fingerprint size={15} /> El acceso multiusuario y MFA se
              incorporarán antes de abrir el producto al público.
            </p>
          </section>
          <section className="card settings-card">
            <div className="card-heading">
              <h2>Conexiones de exchange</h2>
              <KeyRound size={18} />
            </div>
            <div className="exchange-row">
              <span className="exchange-icon binance">B</span>
              <div>
                <strong>Binance Futures</strong>
                <small>Adaptador aún no implementado</small>
              </div>
              <span className="pill muted">NO CONECTADO</span>
            </div>
            <div className="exchange-row">
              <span className="exchange-icon bybit">B</span>
              <div>
                <strong>Bybit Linear</strong>
                <small>Adaptador aún no implementado</small>
              </div>
              <span className="pill muted">NO CONECTADO</span>
            </div>
            <p className="muted-copy">
              No introduzcas API keys: esta versión no almacena credenciales de
              exchanges.
            </p>
          </section>
          <section className="card settings-card full-span">
            <div className="card-heading">
              <h2>Estado de la entrega</h2>
              <FlaskConical size={18} />
            </div>
            <div className="delivery-grid">
              <Setting label="Modo" value="Simulación exclusivamente" />
              <Setting label="Motor" value="Go · Aritmética decimal" />
              <Setting
                label="Persistencia"
                value={
                  sessionBackend() === "postgres"
                    ? "Identidad y sesiones · Planes pendientes"
                    : "Sesiones locales · Sin datos reales"
                }
              />
              <Setting label="Trading real" value="Deshabilitado por diseño" />
            </div>
          </section>
        </div>
      </>
    );
  notFound();
}
function Heading({
  eyebrow,
  title,
  description,
}: {
  eyebrow: string;
  title: string;
  description: string;
}) {
  return (
    <div className="page-heading">
      <div>
        <span className="eyebrow">{eyebrow}</span>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
    </div>
  );
}
function Rule({
  icon,
  title,
  text,
}: {
  icon: React.ReactNode;
  title: string;
  text: string;
}) {
  return (
    <section className="card rule-card">
      <span className="rule-icon">{icon}</span>
      <h2>{title}</h2>
      <p>{text}</p>
    </section>
  );
}
function Setting({ label, value }: { label: string; value: string }) {
  return (
    <div className="setting-row">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

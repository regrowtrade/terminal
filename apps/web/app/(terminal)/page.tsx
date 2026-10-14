import Link from "next/link";
import {
  ArrowRight,
  ArrowUpRight,
  CircleDollarSign,
  Layers3,
  ShieldCheck,
  Target,
  TrendingUp,
} from "lucide-react";
import { EquityChart } from "@/components/equity-chart";
import { Positions } from "@/components/positions";
import { PlanButton } from "@/components/workspace";
import { requireOwner } from "@/lib/auth";

export default async function Dashboard() {
  await requireOwner();
  return (
    <>
      <div className="page-heading">
        <div>
          <span className="eyebrow">UN ESPACIO PARA DECIDIR MEJOR</span>
          <h1>
            Tu operativa, en perspectiva<span className="accent">.</span>
          </h1>
          <p>
            Una visión clara de tu capital, tus posiciones y tu próximo
            movimiento.
          </p>
        </div>
        <span className="example-label">WORKSPACE DE EJEMPLO</span>
      </div>
      <div className="metric-grid">
        <Metric
          icon={<CircleDollarSign size={17} />}
          label="Capital de referencia"
          value="100,00"
          unit="USDT"
          note="Base ilustrativa de la cuenta"
        />
        <Metric
          icon={<TrendingUp size={17} />}
          label="PnL flotante"
          value="+1,00"
          unit="USDT"
          note="Suma de los tres ejemplos"
          positive
        />
        <Metric
          icon={<Layers3 size={17} />}
          label="Posiciones abiertas"
          value="03"
          note="2 activos · 1 cobertura"
        />
        <Metric
          icon={<ShieldCheck size={17} />}
          label="Pérdida de cobertura"
          value="2,00"
          unit="USDT"
          note="2 % del capital de referencia"
        />
      </div>
      <div className="dashboard-middle">
        <EquityChart />
        <section className="card risk-overview">
          <div className="card-heading">
            <div>
              <span className="eyebrow">EL RIESGO, A LA VISTA</span>
              <h2>Opera con límites.</h2>
            </div>
            <ShieldCheck size={19} className="subtle" />
          </div>
          <div className="risk-ring">
            <svg viewBox="0 0 160 160" aria-hidden="true">
              <circle
                cx="80"
                cy="80"
                r="65"
                fill="none"
                stroke="#2b3030"
                strokeWidth="9"
              />
              <circle
                cx="80"
                cy="80"
                r="65"
                fill="none"
                stroke="#c5f277"
                strokeWidth="9"
                strokeLinecap="round"
                strokeDasharray="102 408"
                transform="rotate(-90 80 80)"
              />
            </svg>
            <div>
              <strong>
                2<span>,00</span>
              </strong>
              <small>USDT DE PÉRDIDA</small>
            </div>
          </div>
          <div className="risk-detail">
            <span>Adicional sugerido</span>
            <strong>2,00 USDT</strong>
          </div>
          <div className="risk-detail">
            <span>Umbral de ejemplo</span>
            <strong>−4,00 USDT</strong>
          </div>
          <p className="muted-copy">
            El adicional se confirma en cada gestión. Nunca se aplica
            automáticamente.
          </p>
          <PlanButton mode="hedge" className="button secondary full">
            Diseñar gestión <ArrowRight size={15} />
          </PlanButton>
        </section>
      </div>
      <div className="section-heading">
        <div>
          <span className="eyebrow">TU PRÓXIMO MOVIMIENTO</span>
          <h2>Un plan para cada escenario</h2>
        </div>
        <Link href="/strategies" className="text-link">
          Explorar estrategias <ArrowUpRight size={14} />
        </Link>
      </div>
      <div className="strategy-grid">
        <Strategy
          icon={<Target size={20} />}
          label="01 / ENTRADA LIMPIA"
          title="Una entrada. Un objetivo."
          description="Define tu riesgo y una relación beneficio/riesgo a tu medida."
          mode="clean"
          color="lime"
        />
        <Strategy
          icon={<Layers3 size={20} />}
          label="02 / RECOMPRAS"
          title="Construye tu posición."
          description="Niveles 1, 1, 2, 4… con separación y límites de margen explícitos."
          mode="dca"
          color="blue"
        />
        <Strategy
          icon={<ShieldCheck size={20} />}
          label="03 / COBERTURA"
          title="Recupera con intención."
          description="Desmonta por etapas, con un presupuesto adicional definido por ti."
          mode="hedge"
          color="lavender"
        />
      </div>
      <Positions />
    </>
  );
}
function Metric({
  icon,
  label,
  value,
  unit,
  note,
  positive,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  unit?: string;
  note: string;
  positive?: boolean;
}) {
  return (
    <section className="metric-card">
      <div className="metric-label">
        <span>{label}</span>
        {icon}
      </div>
      <div className={`metric-value ${positive ? "positive" : ""}`}>
        {value}
        {unit && <small>{unit}</small>}
      </div>
      <span className="metric-note">
        {positive && <ArrowUpRight size={12} />}
        {note}
      </span>
    </section>
  );
}
function Strategy({
  icon,
  label,
  title,
  description,
  mode,
  color,
}: {
  icon: React.ReactNode;
  label: string;
  title: string;
  description: string;
  mode: "clean" | "dca" | "hedge";
  color: string;
}) {
  return (
    <PlanButton mode={mode} className={`strategy-card ${color}`}>
      <div className="strategy-card-top">
        <span className="strategy-icon">{icon}</span>
        <ArrowUpRight size={18} />
      </div>
      <span className="eyebrow">{label}</span>
      <h3>{title}</h3>
      <p>{description}</p>
      <span className="strategy-cta">
        Crear vista previa <ArrowRight size={14} />
      </span>
    </PlanButton>
  );
}

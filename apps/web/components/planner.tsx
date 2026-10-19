"use client";

import { useEffect, useId, useRef, useState } from "react";
import {
  ArrowRight,
  Check,
  FlaskConical,
  Info,
  Layers3,
  LoaderCircle,
  ShieldCheck,
  Target,
  X,
} from "lucide-react";
import {
  defaultRequest,
  planRequestSchema,
  planSchema,
  type Plan,
  type PlanRequest,
} from "@/lib/plans";
import { riskSuggestion } from "@/lib/risk-suggestion";
import { money } from "@/lib/demo";
import { PlanSave } from "./plan-save";

const modes = [
  {
    key: "clean" as const,
    title: "Entrada limpia",
    description: "Una entrada, SL y objetivo.",
    icon: Target,
  },
  {
    key: "dca" as const,
    title: "Recompras",
    description: "Construcción por niveles.",
    icon: Layers3,
  },
  {
    key: "hedge" as const,
    title: "Cobertura",
    description: "Desmonte y riesgo adicional.",
    icon: ShieldCheck,
  },
];
const exampleHedge: NonNullable<PlanRequest["hedge"]> = {
  longEntry: "100",
  shortEntry: "98",
  longQuantity: "1",
  shortQuantity: "1",
  markPrice: "99",
  realizedPnl: "0",
  feesPaid: "0",
  fundingNet: "0",
  closePercent: "5",
};
function preset(mode: PlanRequest["mode"]): PlanRequest {
  return {
    ...defaultRequest,
    mode,
    risk: mode === "dca" ? "10" : "2",
    maxLevels: mode === "dca" ? 11 : 4,
    ...(mode === "hedge"
      ? { symbol: "ETHUSDT", side: "SHORT", hedge: { ...exampleHedge } }
      : {}),
  };
}

export function PlanDialog({
  initialMode,
  onClose,
}: {
  initialMode: PlanRequest["mode"];
  onClose: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const headingId = useId();
  useEffect(() => {
    const dialog = ref.current;
    const previousFocus = document.activeElement;
    dialog?.showModal();
    const old = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      dialog?.close();
      document.body.style.overflow = old;
      if (previousFocus instanceof HTMLElement && previousFocus.isConnected)
        previousFocus.focus();
    };
  }, []);
  return (
    <dialog
      className="plan-dialog"
      ref={ref}
      aria-labelledby={headingId}
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="dialog-inner">
        <header className="dialog-heading">
          <div>
            <span className="eyebrow">PLANIFICA ANTES DE OPERAR</span>
            <h2 id={headingId}>
              Diseña tu próximo movimiento<span className="accent">.</span>
            </h2>
          </div>
          <button
            type="button"
            className="icon-button"
            onClick={onClose}
            aria-label="Cerrar calculadora"
          >
            <X size={21} />
          </button>
        </header>
        <Planner initialMode={initialMode} />
      </div>
    </dialog>
  );
}

export function Planner({
  initialMode = "clean",
}: {
  initialMode?: PlanRequest["mode"];
}) {
  const [request, setRequest] = useState<PlanRequest>(() =>
    preset(initialMode),
  );
  const [plan, setPlan] = useState<Plan | null>(null);
  const [error, setError] = useState("");
  const [issues, setIssues] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);
  const abort = useRef<AbortController | null>(null);
  const result = useRef<HTMLDivElement>(null);
  const id = useId().replaceAll(":", "");
  useEffect(() => () => abort.current?.abort(), []);

  function change<K extends keyof PlanRequest>(key: K, value: PlanRequest[K]) {
    setRequest((r) => ({ ...r, [key]: value }));
    setPlan(null);
    setError("");
    setIssues({});
  }
  function changeHedge(
    key: keyof NonNullable<PlanRequest["hedge"]>,
    value: string,
  ) {
    setRequest((r) => ({
      ...r,
      hedge: { ...(r.hedge ?? exampleHedge), [key]: value },
    }));
    setPlan(null);
    setError("");
    setIssues({});
  }
  let suggestion: ReturnType<typeof riskSuggestion> | null = null;
  if (request.hedge) {
    try {
      suggestion = riskSuggestion(
        request.hedge,
        request.riskUnit,
        request.accountReference,
      );
    } catch {
      /* Incomplete editable inputs. */
    }
  }
  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setPlan(null);
    const valid = planRequestSchema.safeParse(request);
    if (!valid.success) {
      const next = Object.fromEntries(
        valid.error.issues.map((issue) => [
          issue.path.join("."),
          issue.message,
        ]),
      );
      setIssues(next);
      setError("Revisa los campos señalados antes de calcular.");
      const first = valid.error.issues[0]?.path.join(".");
      document.getElementById(`${id}-${first}`)?.focus();
      return;
    }
    setPending(true);
    setIssues({});
    abort.current?.abort();
    abort.current = new AbortController();
    try {
      const response = await fetch("/api/plans/preview", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(valid.data),
        signal: abort.current.signal,
      });
      const body: unknown = await response.json();
      if (!response.ok) {
        const message =
          typeof body === "object" &&
          body !== null &&
          "error" in body &&
          typeof body.error === "string"
            ? body.error
            : "No se pudo calcular el plan.";
        throw new Error(message);
      }
      setPlan(planSchema.parse(body));
    } catch (e) {
      if (e instanceof Error && e.name === "AbortError") return;
      setError(
        e instanceof Error
          ? e.message
          : "No se pudo conectar con el motor. No se envió ninguna orden.",
      );
    } finally {
      setPending(false);
    }
  }
  useEffect(() => {
    if (plan) result.current?.focus();
  }, [plan]);
  const field = (
    key:
      | "symbol"
      | "entry"
      | "quantity"
      | "risk"
      | "rewardRatio"
      | "spacingPercent"
      | "maxMargin"
      | "costReserve"
      | "accountReference",
    label: string,
    suffix?: string,
    hint?: string,
  ) => (
    <Field
      key={key}
      id={`${id}-${key}`}
      label={label}
      value={request[key]}
      suffix={suffix}
      hint={hint}
      error={issues[key]}
      onChange={(value) =>
        change(key, key === "symbol" ? value.toUpperCase() : value)
      }
      decimal={key !== "symbol"}
    />
  );
  const hf = (
    key: keyof NonNullable<PlanRequest["hedge"]>,
    label: string,
    suffix?: string,
  ) => (
    <Field
      key={key}
      id={`${id}-hedge.${key}`}
      label={label}
      value={request.hedge?.[key] ?? ""}
      suffix={suffix}
      error={issues[`hedge.${key}`]}
      onChange={(value) => changeHedge(key, value)}
    />
  );
  return (
    <div className="planner">
      <div className="notice simulation-notice">
        <FlaskConical size={17} />
        <span>
          <strong>Vista previa, no ejecución.</strong> Los datos se introducen
          manualmente. No se guarda ni envía ninguna orden al exchange.
        </span>
      </div>
      <form onSubmit={submit}>
        <fieldset disabled={pending} className="planner-fieldset">
          <div className="mode-picker" aria-label="Tipo de plan">
            {modes.map(({ key, title, description, icon: Icon }) => (
              <button
                type="button"
                key={key}
                className={request.mode === key ? "selected" : ""}
                aria-pressed={request.mode === key}
                onClick={() => {
                  setRequest(preset(key));
                  setPlan(null);
                  setError("");
                  setIssues({});
                }}
              >
                <Icon size={19} />
                <strong>{title}</strong>
                <small>{description}</small>
                {request.mode === key && (
                  <Check size={13} className="mode-check" />
                )}
              </button>
            ))}
          </div>
          <div className="form-section-heading">
            <span>01</span>
            <h3>
              {request.mode === "hedge"
                ? "Configura el desmonte"
                : "Define la posición"}
            </h3>
          </div>
          <div className="form-grid">
            {field(
              "symbol",
              "Activo",
              undefined,
              "Contrato USDT, por ejemplo BTCUSDT",
            )}
            <label className="field">
              <span>
                {request.mode === "hedge"
                  ? "Lado que quieres cerrar"
                  : "Dirección"}
              </span>
              <select
                value={request.side}
                onChange={(e) =>
                  change("side", e.target.value as PlanRequest["side"])
                }
              >
                <option value="LONG">
                  {request.mode === "hedge" ? "Cerrar LONG" : "LONG · Comprar"}
                </option>
                <option value="SHORT">
                  {request.mode === "hedge" ? "Cerrar SHORT" : "SHORT · Vender"}
                </option>
              </select>
            </label>
            {field(
              "entry",
              request.mode === "hedge"
                ? "Precio inicial de cierre"
                : "Precio de entrada",
              "USDT",
            )}
            {request.mode !== "hedge" &&
              field("quantity", "Cantidad inicial", "monedas")}
            {request.mode !== "hedge" && (
              <label className="field">
                <span>Apalancamiento</span>
                <select
                  value={request.leverage}
                  onChange={(e) => change("leverage", Number(e.target.value))}
                >
                  {[1, 2, 5, 10, 20, 50, 100, 125].map((n) => (
                    <option value={n} key={n}>
                      {n}×
                    </option>
                  ))}
                </select>
              </label>
            )}
            {request.mode !== "hedge" &&
              field(
                "rewardRatio",
                "Beneficio / riesgo",
                ": 1",
                "3 significa un objetivo de 3 veces el riesgo.",
              )}
            {request.mode === "dca" &&
              field(
                "spacingPercent",
                "Separación entre niveles",
                "%",
                "Compuesta respecto al nivel anterior.",
              )}
            {request.mode !== "clean" && (
              <label className="field">
                <span>Máximo de niveles, incluida la primera orden</span>
                <select
                  value={request.maxLevels}
                  onChange={(e) => change("maxLevels", Number(e.target.value))}
                >
                  {Array.from({ length: 11 }, (_, i) => (
                    <option key={i + 1} value={i + 1}>
                      {i + 1} {i === 0 ? "nivel" : "niveles"}
                    </option>
                  ))}
                </select>
                <small>
                  El motor puede admitir menos niveles según los límites.
                </small>
              </label>
            )}
            {request.mode === "hedge" && (
              <label className="field">
                <span>Cierre inicial de monedas</span>
                <select
                  value={request.hedge?.closePercent}
                  onChange={(e) => changeHedge("closePercent", e.target.value)}
                >
                  <option value="5">5 % · Factor de distancia 1,50</option>
                  <option value="10">10 % · Factor de distancia 1,20</option>
                </select>
                <small>Secuencia de cantidades: 1, 1, 2, 4, 8…</small>
              </label>
            )}
          </div>
          {request.mode === "hedge" && (
            <>
              <div className="form-section-heading">
                <span>02</span>
                <h3>Fotografía actual de la cobertura</h3>
              </div>
              <div className="form-grid">
                {hf("longEntry", "Entrada media LONG", "USDT")}
                {hf("shortEntry", "Entrada media SHORT", "USDT")}
                {hf("longQuantity", "Monedas LONG")}
                {hf("shortQuantity", "Monedas SHORT")}
                {hf("markPrice", "Mark price de referencia", "USDT")}
                {hf(
                  "realizedPnl",
                  "PnL realizado de toda la operación",
                  "USDT",
                )}
                {hf("feesPaid", "Comisiones ya pagadas", "USDT")}
                {hf("fundingNet", "Funding neto (+ / −)", "USDT")}
              </div>
            </>
          )}
          <div className="form-section-heading">
            <span>{request.mode === "hedge" ? "03" : "02"}</span>
            <h3>Tu presupuesto de riesgo</h3>
          </div>
          <div className="form-grid">
            {field(
              "accountReference",
              "Capital de referencia de la cuenta",
              "USDT",
              "Valor manual capturado para este cálculo.",
            )}
            <label className="field">
              <span>Base del capital</span>
              <select
                value={request.accountBasis}
                onChange={(e) =>
                  change(
                    "accountBasis",
                    e.target.value as PlanRequest["accountBasis"],
                  )
                }
              >
                <option value="equity">Equity · Incluye PnL flotante</option>
                <option value="wallet">Saldo · Sin PnL flotante</option>
              </select>
            </label>
            <label className="field">
              <span>Unidad de riesgo</span>
              <select
                value={request.riskUnit}
                onChange={(e) =>
                  change("riskUnit", e.target.value as PlanRequest["riskUnit"])
                }
              >
                <option value="amount">Cantidad en USDT</option>
                <option value="percent">
                  Porcentaje del capital de referencia
                </option>
              </select>
            </label>
            {field(
              "risk",
              request.mode === "hedge"
                ? "Riesgo adicional autorizado"
                : "Riesgo total autorizado",
              request.riskUnit === "percent" ? "%" : "USDT",
            )}
            {request.mode !== "hedge" &&
              field("maxMargin", "Margen acumulado máximo", "USDT")}
            {field(
              "costReserve",
              "Reserva estimada de costes futuros",
              "USDT",
              "Manual: incluye costes que todavía no se han realizado.",
            )}
          </div>
          {request.mode === "hedge" && suggestion && (
            <div className="risk-suggestion">
              <Info size={17} />
              <div>
                <strong>
                  Resultado neto actual: {money(Number(suggestion.net))} USDT
                </strong>
                <p>Incluye ambos lados, PnL realizado, comisiones y funding.</p>
              </div>
              <button
                className="button secondary compact"
                type="button"
                disabled={suggestion.suggested === "0"}
                onClick={() => change("risk", suggestion!.suggested)}
              >
                Usar adicional sugerido: {money(Number(suggestion.suggested))}
                {request.riskUnit === "percent" ? " %" : " USDT"}
              </button>
            </div>
          )}
          <div className="planner-submit">
            <p>
              Confirmas los parámetros de una simulación.
              <br />
              <strong>No autorizas operaciones reales.</strong>
            </p>
            <button type="submit" className="button primary" disabled={pending}>
              {pending ? (
                <LoaderCircle size={17} className="spin" />
              ) : (
                <Target size={17} />
              )}
              {pending ? "Calculando…" : "Calcular vista previa"}
              <ArrowRight size={16} />
            </button>
          </div>
        </fieldset>
      </form>
      {error && (
        <div className="notice danger" role="alert">
          {error}
        </div>
      )}
      {plan && (
        <div
          ref={result}
          tabIndex={-1}
          className="plan-result"
          aria-label="Resultado del cálculo"
        >
          <PlanResult plan={plan} />
          <PlanSave request={request} />
        </div>
      )}
    </div>
  );
}

function Field({
  id,
  label,
  value,
  suffix,
  hint,
  error,
  onChange,
  decimal = true,
}: {
  id: string;
  label: string;
  value: string;
  suffix?: string;
  hint?: string;
  error?: string;
  onChange: (v: string) => void;
  decimal?: boolean;
}) {
  return (
    <label className={`field ${error ? "invalid" : ""}`} htmlFor={id}>
      <span id={`${id}-label`}>{label}</span>
      <div className="input-wrap">
        <input
          id={id}
          value={value}
          inputMode={decimal ? "decimal" : "text"}
          maxLength={decimal ? 32 : 24}
          autoComplete="off"
          onChange={(e) =>
            onChange(
              decimal ? e.target.value.replace(",", ".") : e.target.value,
            )
          }
          aria-labelledby={`${id}-label`}
          aria-invalid={Boolean(error)}
          aria-describedby={error || hint ? `${id}-hint` : undefined}
          required
        />
        {suffix && <span aria-hidden="true">{suffix}</span>}
      </div>
      {(error || hint) && <small id={`${id}-hint`}>{error ?? hint}</small>}
    </label>
  );
}
function PlanResult({ plan }: { plan: Plan }) {
  return (
    <>
      <div className="result-heading">
        <span className="result-check">
          <Check size={18} />
        </span>
        <div>
          <span className="eyebrow">ESCENARIOS CALCULADOS POR EL MOTOR GO</span>
          <h3>
            {plan.symbol} ·{" "}
            {plan.mode === "hedge" ? `Cerrar ${plan.side}` : plan.side}
          </h3>
        </div>
        <span className="pill muted">NO EJECUTADO</span>
      </div>
      <div className="result-metrics">
        <div>
          <span>
            {plan.mode === "hedge" ? "Riesgo adicional" : "Riesgo total"}
          </span>
          <strong>
            {money(Number(plan.risk))} <small>USDT</small>
          </strong>
        </div>
        <div>
          <span>
            {plan.mode === "hedge"
              ? "Resultado de referencia"
              : "Niveles admitidos"}
          </span>
          <strong>
            {plan.baselinePnl !== null
              ? money(Number(plan.baselinePnl))
              : plan.levels.length}
            {plan.baselinePnl !== null && <small> USDT</small>}
          </strong>
        </div>
        <div>
          <span>
            {plan.mode === "hedge"
              ? "Umbral de resultado neto"
              : "Margen acumulado"}
          </span>
          <strong>
            {money(Number(plan.pnlThreshold ?? plan.levels.at(-1)!.margin))}
            <small> USDT</small>
          </strong>
        </div>
      </div>
      <div className="plan-basis">
        Base capturada: {plan.accountBasis === "equity" ? "equity" : "saldo"} de{" "}
        {plan.accountReference} USDT · Separación: {plan.spacingPercent} % ·
        Reserva: {plan.costReserve} USDT
      </div>
      <div className="table-scroll">
        <table className="plan-table">
          <caption className="sr-only">
            Escenarios acumulados por nivel completo; precios estimados sin
            filtros de exchange.
          </caption>
          <thead>
            <tr>
              <th>NIVEL</th>
              <th>PRECIO</th>
              <th>MONEDAS</th>
              <th>ACUMULADO</th>
              <th>SL DEL ESCENARIO</th>
              <th>TP</th>
              <th>MONEDAS TP</th>
            </tr>
          </thead>
          <tbody>
            {plan.levels.map((l) => (
              <tr key={l.index}>
                <td>
                  <span className="level-badge">{l.index}</span>
                </td>
                <td className="numeric" title={l.price}>
                  {money(Number(l.price), 4)}
                </td>
                <td className="numeric">{l.quantity}</td>
                <td className="numeric">{l.cumulativeQuantity}</td>
                <td
                  className="numeric negative"
                  title={l.stopPrice ?? "Exposición neutral"}
                >
                  {l.stopPrice
                    ? money(Number(l.stopPrice), 4)
                    : "Sin SL direccional"}
                </td>
                <td className="numeric positive" title={l.takeProfit ?? ""}>
                  {l.takeProfit ? money(Number(l.takeProfit), 4) : "—"}
                </td>
                <td className="numeric">{l.takeProfitQuantity}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="stop-reason">
        <Info size={15} />
        {plan.stopReason}
      </p>
      <details className="plan-warnings" open>
        <summary>Supuestos y límites de esta vista previa</summary>
        <ul>
          {plan.warnings.map((warning) => (
            <li key={warning}>{warning}</li>
          ))}
        </ul>
      </details>
    </>
  );
}

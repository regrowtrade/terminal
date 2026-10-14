"use client";
import { useId, useState } from "react";
import { ArrowUpRight } from "lucide-react";

const paths = {
  "7D": "M0 145 L22 143 L44 150 L66 124 L88 130 L110 123 L132 130 L154 104 L176 106 L198 97 L220 107 L242 82 L264 89 L286 65 L308 71 L330 51 L352 56 L374 48 L396 67 L418 49 L440 54 L462 35 L484 41 L506 26 L528 35 L550 18 L572 28 L594 12 L616 21 L640 9",
  "30D":
    "M0 158 L22 141 L44 151 L66 127 L88 136 L110 143 L132 120 L154 125 L176 91 L198 103 L220 94 L242 108 L264 96 L286 66 L308 78 L330 64 L352 85 L374 59 L396 65 L418 37 L440 48 L462 40 L484 55 L506 32 L528 37 L550 19 L572 28 L594 12 L616 21 L640 9",
  "90D":
    "M0 170 L22 151 L44 159 L66 141 L88 147 L110 156 L132 133 L154 140 L176 117 L198 130 L220 104 L242 115 L264 87 L286 100 L308 114 L330 92 L352 67 L374 86 L396 74 L418 93 L440 71 L462 49 L484 65 L506 42 L528 31 L550 49 L572 23 L594 12 L616 21 L640 9",
};
export function EquityChart() {
  const [range, setRange] = useState<keyof typeof paths>("30D");
  const id = useId().replaceAll(":", "");
  return (
    <section className="card equity-card">
      <div className="card-heading">
        <div>
          <span className="eyebrow">RENDIMIENTO ILUSTRATIVO</span>
          <h2>La perspectiva completa</h2>
        </div>
        <div className="segmented small" aria-label="Periodo del gráfico">
          {(Object.keys(paths) as Array<keyof typeof paths>).map((r) => (
            <button
              key={r}
              aria-pressed={range === r}
              onClick={() => setRange(r)}
              className={range === r ? "selected" : ""}
            >
              {r}
            </button>
          ))}
        </div>
      </div>
      <div className="equity-value">
        <span>
          101<span className="decimal-part">,00</span>
        </span>
        <small>USDT</small>
        <span className="chart-change">
          <ArrowUpRight size={14} /> +1,00 %
        </span>
      </div>
      <div className="chart-legend">
        <i /> Equity de ejemplo <span>Capital inicial: 100 USDT</span>
      </div>
      <div className="chart-wrap">
        <div className="chart-y">
          <span>102</span>
          <span>100</span>
          <span>98</span>
          <span>96</span>
        </div>
        <svg
          viewBox="0 0 640 195"
          role="img"
          aria-label={`Curva ilustrativa de equity para ${range}; no representa historial real.`}
          preserveAspectRatio="none"
        >
          <defs>
            <linearGradient id={id} x1="0" x2="0" y1="0" y2="1">
              <stop offset="0%" stopColor="#c5f277" stopOpacity=".16" />
              <stop offset="100%" stopColor="#c5f277" stopOpacity="0" />
            </linearGradient>
          </defs>
          {[10, 65, 120, 175].map((y) => (
            <line
              key={y}
              x1="0"
              y1={y}
              x2="640"
              y2={y}
              stroke="#303437"
              strokeDasharray="3 6"
            />
          ))}
          <path d={`${paths[range]} L640 195 L0 195 Z`} fill={`url(#${id})`} />
          <path
            d={paths[range]}
            stroke="#c5f277"
            strokeWidth="2.4"
            strokeLinejoin="round"
            fill="none"
          />
          <circle cx="640" cy="9" r="4" fill="#c5f277" />
        </svg>
      </div>
      <div className="chart-x">
        <span>Inicio</span>
        <span>Primer tercio</span>
        <span>Segundo tercio</span>
        <span>Ejemplo actual</span>
      </div>
    </section>
  );
}

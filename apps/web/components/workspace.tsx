"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { createContext, useContext, useState } from "react";
import {
  ArrowUpRight,
  BookOpen,
  ChartNoAxesCombined,
  ChevronDown,
  CircleHelp,
  FlaskConical,
  LayoutDashboard,
  LogOut,
  Plus,
  Settings2,
  ShieldCheck,
  SlidersHorizontal,
  Wallet,
} from "lucide-react";
import { Brand } from "./brand";
import { PlanDialog } from "./planner";
import type { PlanRequest } from "@/lib/plans";

const WorkspaceContext = createContext<{
  openPlan: (mode?: PlanRequest["mode"]) => void;
}>({ openPlan: () => {} });
export const useWorkspace = () => useContext(WorkspaceContext);
const nav = [
  { href: "/", label: "Vista general", icon: LayoutDashboard },
  { href: "/positions", label: "Posiciones", icon: ChartNoAxesCombined },
  { href: "/strategies", label: "Estrategias", icon: SlidersHorizontal },
  { href: "/risk", label: "Control de riesgo", icon: ShieldCheck },
  { href: "/journal", label: "Trading journal", icon: BookOpen },
  { href: "/settings", label: "Configuración", icon: Settings2 },
];

export function Workspace({
  children,
  engine,
}: {
  children: React.ReactNode;
  engine: "available" | "unavailable";
}) {
  const pathname = usePathname();
  const [mode, setMode] = useState<PlanRequest["mode"] | null>(null);
  const [help, setHelp] = useState(false);
  const label = nav.find((item) => item.href === pathname)?.label ?? "Terminal";
  return (
    <WorkspaceContext.Provider
      value={{ openPlan: (m = "clean") => setMode(m) }}
    >
      <a className="skip-link" href="#main-content">
        Saltar al contenido
      </a>
      <div className="workspace">
        <aside className="sidebar">
          <Link
            href="/"
            className="brand-link"
            aria-label="Regrow, vista general"
          >
            <Brand />
          </Link>
          <div className="workspace-selector">
            <span className="account-avatar">
              <Wallet size={17} />
            </span>
            <span>
              <strong>Mi workspace</strong>
              <small>Personal · Simulación</small>
            </span>
            <ChevronDown size={15} />
          </div>
          <span className="nav-caption">WORKSPACE</span>
          <nav aria-label="Navegación principal">
            {nav.map(({ href, label, icon: Icon }) => (
              <Link
                key={href}
                href={href}
                className={`nav-item ${pathname === href ? "active" : ""}`}
                aria-current={pathname === href ? "page" : undefined}
              >
                <Icon size={18} />
                <span>{label}</span>
                {pathname === href && <span className="nav-active-dot" />}
              </Link>
            ))}
          </nav>
          <div className="sidebar-bottom">
            <div className="sidebar-card">
              <span className="sidebar-card-icon">
                <FlaskConical size={18} />
              </span>
              <strong>Primero, validar.</strong>
              <p>Diseña tu plan sin poner capital en riesgo.</p>
              <button onClick={() => setMode("clean")}>
                Abrir calculadora <ArrowUpRight size={14} />
              </button>
            </div>
            <button
              className="nav-item help-button"
              onClick={() => setHelp(!help)}
              aria-expanded={help}
            >
              <CircleHelp size={18} /> Guía rápida
            </button>
            {help && (
              <p className="help-copy">
                Los ejemplos son ficticios. «Nuevo plan» consulta al motor Go
                para calcular escenarios. Ningún botón ejecuta órdenes ni guarda
                operaciones.
              </p>
            )}
            <div className="profile">
              <span className="profile-avatar">R</span>
              <span>
                <strong>Mi terminal</strong>
                <small>Acceso privado</small>
              </span>
              <form method="post" action="/api/auth/logout">
                <button aria-label="Cerrar sesión" className="icon-button">
                  <LogOut size={17} />
                </button>
              </form>
            </div>
          </div>
        </aside>
        <div className="workspace-body">
          <header className="topbar">
            <div className="breadcrumb">
              <span>Workspace</span>
              <span>/</span>
              <strong>{label}</strong>
            </div>
            <div className="topbar-actions">
              <span
                className={`engine-status ${engine === "available" ? "online" : ""}`}
              >
                <i />
                {engine === "available"
                  ? "Motor disponible"
                  : "Motor desconectado"}
              </span>
              <span className="pill muted">SIMULACIÓN</span>
              <button
                className="button primary compact"
                onClick={() => setMode("clean")}
              >
                <Plus size={16} /> Nuevo plan
              </button>
              <form
                className="mobile-logout"
                method="post"
                action="/api/auth/logout"
              >
                <button aria-label="Cerrar sesión" className="icon-button">
                  <LogOut size={17} />
                </button>
              </form>
            </div>
          </header>
          <div className="demo-strip">
            <FlaskConical size={14} />
            <span>
              Entorno de simulación. Datos ilustrativos; no hay cuentas de
              exchange conectadas.
            </span>
          </div>
          <main id="main-content" className="main-content">
            {children}
          </main>
          <footer className="workspace-footer">
            <span>
              REGROW TERMINAL <span className="version">v0.1</span>
            </span>
            <span>Claridad antes que velocidad.</span>
            <span>
              Sin ejecución real <ShieldCheck size={12} />
            </span>
          </footer>
        </div>
      </div>
      {mode && <PlanDialog initialMode={mode} onClose={() => setMode(null)} />}
    </WorkspaceContext.Provider>
  );
}

export function PlanButton({
  mode = "clean",
  children,
  className = "button primary",
}: {
  mode?: PlanRequest["mode"];
  children: React.ReactNode;
  className?: string;
}) {
  const { openPlan } = useWorkspace();
  return (
    <button className={className} onClick={() => openPlan(mode)}>
      {children}
    </button>
  );
}

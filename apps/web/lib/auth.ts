import "server-only";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { configurationReady, verifySession } from "./security";

export function cookieName() {
  return process.env.NODE_ENV === "production"
    ? "__Host-regrow-session"
    : "regrow-session";
}
export function isConfigured() {
  return configurationReady(
    process.env.TERMINAL_PASSWORD ?? "",
    process.env.SESSION_SECRET ?? "",
  );
}
export async function authenticated() {
  return (
    isConfigured() &&
    verifySession(
      (await cookies()).get(cookieName())?.value,
      process.env.SESSION_SECRET ?? "",
    )
  );
}
export async function requireOwner() {
  if (!(await authenticated())) redirect("/login");
}

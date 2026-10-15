import "server-only";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { configurationReady, verifySession } from "./security";
import { sessionBackend, sessionOperation } from "./sessions";

export function cookieName() {
  return process.env.NODE_ENV === "production"
    ? "__Host-regrow-session"
    : "regrow-session";
}
export function isConfigured() {
  return (
    sessionBackend() !== null &&
    configurationReady(
      process.env.TERMINAL_PASSWORD ?? "",
      process.env.SESSION_SECRET ?? "",
    )
  );
}
export async function authenticated() {
  if (!isConfigured()) return false;
  const cookie = (await cookies()).get(cookieName())?.value;
  if (!cookie || !verifySession(cookie, process.env.SESSION_SECRET ?? ""))
    return false;
  try {
    return await sessionOperation("resolve", cookie);
  } catch {
    return false;
  }
}
export async function requireOwner() {
  if (!(await authenticated())) redirect("/login");
}

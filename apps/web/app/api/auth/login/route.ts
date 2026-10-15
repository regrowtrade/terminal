import { NextResponse } from "next/server";
import { cookieName, isConfigured } from "@/lib/auth";
import {
  AttemptLimiter,
  applicationOrigin,
  issueSession,
  sameOrigin,
  SESSION_SECONDS,
  verifyPassword,
} from "@/lib/security";
import { boundedText } from "@/lib/request";
import { sessionOperation, SessionUnavailable } from "@/lib/sessions";

const limiter = new AttemptLimiter(5, 5 * 60_000);
export async function POST(request: Request) {
  const origin = applicationOrigin(process.env.APP_ORIGIN);
  if (!origin)
    return new NextResponse("Configuración de origen no disponible", {
      status: 503,
    });
  const location = new URL("/login", origin);
  const failed = (reason: string) => {
    location.searchParams.set("error", reason);
    const response = NextResponse.redirect(location, 303);
    response.headers.set("Cache-Control", "no-store");
    return response;
  };
  if (!sameOrigin(request.headers.get("origin"), process.env.APP_ORIGIN))
    return new NextResponse("Origen no autorizado", { status: 403 });
  if (!isConfigured()) return failed("configuration");
  if (!limiter.allow()) return failed("limited");
  try {
    if (
      !request.headers
        .get("content-type")
        ?.startsWith("application/x-www-form-urlencoded")
    )
      return failed("invalid");
    const input =
      new URLSearchParams(await boundedText(request, 2048)).get("password") ??
      "";
    if (
      !verifyPassword(
        input,
        process.env.TERMINAL_PASSWORD ?? "",
        process.env.SESSION_SECRET ?? "",
      )
    )
      return failed("invalid");
    const session = issueSession(process.env.SESSION_SECRET!);
    if (!(await sessionOperation("create", session)))
      throw new SessionUnavailable();
    limiter.reset();
    const response = NextResponse.redirect(new URL("/", origin), 303);
    response.headers.set("Cache-Control", "no-store");
    response.cookies.set(cookieName(), session, {
      httpOnly: true,
      secure: process.env.NODE_ENV === "production",
      sameSite: "strict",
      path: "/",
      maxAge: SESSION_SECONDS,
    });
    return response;
  } catch (error) {
    if (error instanceof SessionUnavailable) return failed("unavailable");
    return failed("invalid");
  }
}

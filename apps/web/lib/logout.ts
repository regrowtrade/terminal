import "server-only";
import { NextResponse } from "next/server";
import { cookies } from "next/headers";
import { cookieName } from "@/lib/auth";
import { applicationOrigin, sameOrigin } from "@/lib/security";
import { sessionOperation } from "@/lib/sessions";
export async function logout(request: Request, all: boolean) {
  const origin = applicationOrigin(process.env.APP_ORIGIN);
  if (!origin)
    return new NextResponse("Configuración de origen no disponible", {
      status: 503,
    });
  if (!sameOrigin(request.headers.get("origin"), process.env.APP_ORIGIN))
    return new NextResponse("Origen no autorizado", { status: 403 });
  const cookie = (await cookies()).get(cookieName())?.value;
  try {
    if (cookie) {
      const revoked = await sessionOperation(
        all ? "revoke-all" : "revoke",
        cookie,
      );
      if (all && !revoked)
        return new NextResponse("Sesión requerida", { status: 401 });
    } else if (all)
      return new NextResponse("Sesión requerida", { status: 401 });
  } catch {
    // Retain the cookie so the user can retry. Never report a successful
    // logout while the copied cookie could still be valid in PostgreSQL.
    return new NextResponse(
      "No se pudo revocar la sesión. Vuelve a intentarlo.",
      { status: 503, headers: { "Cache-Control": "no-store" } },
    );
  }
  const response = NextResponse.redirect(new URL("/login", origin), 303);
  response.cookies.set(cookieName(), "", {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "strict",
    path: "/",
    maxAge: 0,
  });
  response.headers.set("Cache-Control", "no-store");
  return response;
}

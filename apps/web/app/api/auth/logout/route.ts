import { NextResponse } from "next/server";
import { cookieName } from "@/lib/auth";
import { sameOrigin } from "@/lib/security";
export async function POST(request: Request) {
  if (!sameOrigin(request.headers.get("origin"), process.env.APP_ORIGIN))
    return new NextResponse("Origen no autorizado", { status: 403 });
  const response = NextResponse.redirect(new URL("/login", request.url), 303);
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

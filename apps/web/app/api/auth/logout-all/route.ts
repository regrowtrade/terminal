import { logout } from "@/lib/logout";

export async function POST(request: Request) {
  return logout(request, true);
}

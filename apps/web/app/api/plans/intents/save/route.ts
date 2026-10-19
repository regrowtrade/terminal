import { plansRequest } from "@/lib/saved-plans";

export const runtime = "nodejs";
export async function POST(request: Request) {
  return plansRequest(request, "intents/save");
}

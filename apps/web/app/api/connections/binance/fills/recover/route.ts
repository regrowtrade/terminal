import { connectionRequest } from "@/lib/connections";

export const runtime = "nodejs";
export async function POST(request: Request) {
  return connectionRequest(request, "fills/recover");
}

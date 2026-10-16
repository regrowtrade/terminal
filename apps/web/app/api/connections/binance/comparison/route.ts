import { connectionRequest } from "@/lib/connections";

export async function POST(request: Request) {
  return connectionRequest(request, "comparison");
}

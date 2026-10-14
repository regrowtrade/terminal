import { requireOwner } from "@/lib/auth";
import { engineStatus } from "@/lib/engine";
import { Workspace } from "@/components/workspace";
export const dynamic = "force-dynamic";
export default async function TerminalLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  await requireOwner();
  return <Workspace engine={await engineStatus()}>{children}</Workspace>;
}

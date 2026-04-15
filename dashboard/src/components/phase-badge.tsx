import { Badge } from "@/components/ui/badge";

const phaseStyles: Record<string, string> = {
  Pending: "bg-yellow-100 text-yellow-800 hover:bg-yellow-100",
  Running: "bg-blue-100 text-blue-800 hover:bg-blue-100",
  Succeeded: "bg-green-100 text-green-800 hover:bg-green-100",
  Failed: "bg-red-100 text-red-800 hover:bg-red-100",
};

export function PhaseBadge({ phase }: { phase: string }) {
  return (
    <Badge variant="outline" className={phaseStyles[phase] || ""}>
      {phase || "Unknown"}
    </Badge>
  );
}
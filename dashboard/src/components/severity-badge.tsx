import { Badge } from "@/components/ui/badge";

const severityStyles: Record<string, string> = {
  critical: "bg-red-600 text-white hover:bg-red-600",
  high: "bg-orange-500 text-white hover:bg-orange-500",
  medium: "bg-yellow-500 text-white hover:bg-yellow-500",
  low: "bg-blue-400 text-white hover:bg-blue-400",
};

export function SeverityBadge({ severity }: { severity: string }) {
  return (
    <Badge className={severityStyles[severity] || ""}>
      {severity}
    </Badge>
  );
}

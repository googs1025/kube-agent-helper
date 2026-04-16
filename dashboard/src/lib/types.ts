export interface DiagnosticRun {
  ID: string;
  TargetJSON: string;
  SkillsJSON: string;
  Status: "Pending" | "Running" | "Succeeded" | "Failed";
  Message: string;
  StartedAt: string | null;
  CompletedAt: string | null;
  CreatedAt: string;
}

export interface Finding {
  ID: string;
  RunID: string;
  Dimension: string;
  Severity: "critical" | "high" | "medium" | "low";
  Title: string;
  Description: string;
  ResourceKind: string;
  ResourceNamespace: string;
  ResourceName: string;
  Suggestion: string;
  CreatedAt: string;
}

export interface Skill {
  ID: string;
  Name: string;
  Dimension: string;
  Prompt: string;
  ToolsJSON: string;
  RequiresDataJSON: string;
  Source: "builtin" | "cr";
  Enabled: boolean;
  Priority: number;
  UpdatedAt: string;
}

export interface CreateRunRequest {
  name?: string;
  namespace: string;
  target: {
    scope: "namespace" | "cluster";
    namespaces?: string[];
    labelSelector?: Record<string, string>;
  };
  skills?: string[];
  modelConfigRef: string;
  timeoutSeconds?: number;
  outputLanguage?: "zh" | "en";
}

export interface CreateSkillRequest {
  name: string;
  namespace: string;
  dimension: "health" | "security" | "cost" | "reliability";
  description: string;
  prompt: string;
  tools: string[];
  requiresData?: string[];
  enabled: boolean;
  priority?: number;
}

export interface Fix {
  ID: string;
  RunID: string;
  FindingTitle: string;
  TargetKind: string;
  TargetNamespace: string;
  TargetName: string;
  Strategy: string;
  ApprovalRequired: boolean;
  PatchType: string;
  PatchContent: string;
  Phase: "PendingApproval" | "Approved" | "Applying" | "Succeeded" | "Failed" | "RolledBack" | "DryRunComplete";
  ApprovedBy: string;
  RollbackSnapshot: string;
  Message: string;
  CreatedAt: string;
  UpdatedAt: string;
}
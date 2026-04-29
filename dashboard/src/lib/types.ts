export interface DiagnosticRun {
  ID: string;
  Name?: string;
  TargetJSON: string;
  SkillsJSON: string;
  Status: "Pending" | "Running" | "Succeeded" | "Failed" | "Scheduled";
  Message: string;
  StartedAt: string | null;
  CompletedAt: string | null;
  CreatedAt: string;
  // Scheduled run fields (only present when spec.schedule is set)
  Schedule?: string;
  HistoryLimit?: number;
  LastRunAt?: string | null;
  NextRunAt?: string | null;
  ActiveRuns?: string[];
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
  FixID?: string;
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
  fallbackModelConfigRefs?: string[];
  timeoutSeconds?: number;
  outputLanguage?: "zh" | "en";
  schedule?: string;
  historyLimit?: number;
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
  Name?: string;
  RunID: string;
  FindingID: string;
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
  BeforeSnapshot: string;
  Message: string;
  CreatedAt: string;
  UpdatedAt: string;
}

export interface K8sResourceItem {
  name: string;
  namespace?: string;
}

export interface ModelConfig {
  name: string;
  namespace: string;
  provider: string;
  model: string;
  baseURL?: string;
  maxTurns?: number;
  retries?: number;
  secretRef: string;
  secretKey: string;
  apiKey: string; // always "****"
}

export interface RunLogEntry {
  id: number;
  run_id: string;
  timestamp: string;
  type: "step" | "finding" | "fix" | "error" | "info";
  message: string;
  data?: string;
}

export interface KubeEvent {
  ID: number;
  UID: string;
  Namespace: string;
  Kind: string;
  Name: string;
  Reason: string;
  Message: string;
  Type: string;
  Count: number;
  FirstTime: string;
  LastTime: string;
  CreatedAt: string;
}

export interface PaginatedResult<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}

export interface ListParams {
  page?: number;
  pageSize?: number;
  sortBy?: string;
  sortOrder?: "asc" | "desc";
  cluster?: string;
  [key: string]: string | number | undefined;
}
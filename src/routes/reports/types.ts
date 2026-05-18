import type { ReportDefinitionSummary } from "$lib/api/reports";

export type ReportsTab =
  | "cashflow"
  | "net-worth"
  | "account-trends"
  | "spending"
  | "payees"
  | "gains"
  | "performance"
  | "valuation"
  | "exposure"
  | "saved";

export type ReportDefinitionForm = {
  name: string;
  kind: "builtin" | "custom";
  query_type: "sql" | "template";
  query_text: string;
  params_schema: string;
};

export function createTodayDate(now = new Date()): string {
  return now.toISOString().split("T")[0];
}

export function createYearStartDate(now = new Date()): string {
  return `${now.getFullYear()}-01-01`;
}

export function createReportDefinitionForm(
  definition?: ReportDefinitionSummary | null
): ReportDefinitionForm {
  return {
    name: definition?.name ?? "",
    kind: (definition?.kind as "builtin" | "custom" | undefined) ?? "custom",
    query_type: (definition?.query_type as "sql" | "template" | undefined) ?? "sql",
    query_text: definition?.query_text ?? "",
    params_schema: definition?.params_schema ?? "",
  };
}
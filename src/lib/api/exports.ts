import { apiText } from "$lib/api/client";

export async function exportAccountsCsv(bookId: number): Promise<string> {
  return apiText(`/exports/accounts.csv?book_id=${bookId}`);
}

export async function exportTransactionsCsv(bookId: number): Promise<string> {
  return apiText(`/exports/transactions.csv?book_id=${bookId}`);
}

export async function exportRegisterQif(accountId: number): Promise<string> {
  return apiText(`/exports/registers/${accountId}.qif`);
}

export async function exportReportCsv(reportKind: string, input: Record<string, unknown>): Promise<string> {
  return apiText(`/exports/reports/${reportKind}.csv`, {
    method: "POST",
    body: JSON.stringify(input),
    headers: { "Content-Type": "application/json" },
  });
}

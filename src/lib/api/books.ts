import { apiGet } from "$lib/api/client";

export type BookSummary = {
  id: number;
  slug: string;
  name: string;
  base_currency_code: string;
};

export type HealthStatus = {
  status: string;
  service: string;
  database: string;
};

export async function listBooks(): Promise<BookSummary[]> {
  return apiGet<BookSummary[]>("/books");
}

export async function getHealthStatus(): Promise<HealthStatus> {
  return apiGet<HealthStatus>("/health");
}
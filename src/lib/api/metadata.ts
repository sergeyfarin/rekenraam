import { apiGetWithTauriFallback } from "$lib/api/client";

export type CommoditySummary = {
  id: number;
  book_id: number;
  kind: string;
  symbol: string | null;
  name: string;
  scale: number;
  metadata: string | null;
  created_at: string;
  updated_at: string;
};

export type CountrySummary = {
  id: number;
  book_id: number;
  code: string;
  name: string;
  created_at: string;
  updated_at: string;
};

export type InstitutionSummary = {
  id: number;
  book_id: number;
  name: string;
  kind: string | null;
  country_id: number | null;
  country_name: string | null;
  created_at: string;
  updated_at: string;
};

export type CategorySummary = {
  id: number;
  book_id: number;
  parent_id: number | null;
  name: string;
  kind: string;
  color: string | null;
  created_at: string;
  updated_at: string;
};

export type PayeeSummary = {
  id: number;
  book_id: number;
  name: string;
  kind: string;
  metadata: string | null;
  created_at: string;
  updated_at: string;
};

export type TagSummary = {
  id: number;
  book_id: number;
  name: string;
  color: string | null;
  created_at: string;
  updated_at: string;
};

export type PersonSummary = {
  id: number;
  book_id: number;
  name: string;
  role: string;
  metadata: string | null;
  created_at: string;
  updated_at: string;
};

export type ProjectSummary = {
  id: number;
  book_id: number;
  name: string;
  status: string;
  metadata: string | null;
  created_at: string;
  updated_at: string;
};

export async function listCommodities(bookId = 1): Promise<CommoditySummary[]> {
  return apiGetWithTauriFallback<CommoditySummary[]>(`/commodities`, "list_commodities", { bookId });
}

export async function listCountries(bookId = 1): Promise<CountrySummary[]> {
  return apiGetWithTauriFallback<CountrySummary[]>(`/countries`, "list_countries", { bookId });
}

export async function listInstitutions(bookId = 1): Promise<InstitutionSummary[]> {
  return apiGetWithTauriFallback<InstitutionSummary[]>(`/institutions`, "list_institutions", { bookId });
}

export async function listCategories(bookId = 1): Promise<CategorySummary[]> {
  return apiGetWithTauriFallback<CategorySummary[]>(`/categories`, "list_categories", { bookId });
}

export async function listPayees(bookId = 1): Promise<PayeeSummary[]> {
  return apiGetWithTauriFallback<PayeeSummary[]>(`/payees`, "list_payees", { bookId });
}

export async function listTags(bookId = 1): Promise<TagSummary[]> {
  return apiGetWithTauriFallback<TagSummary[]>(`/tags`, "list_tags", { bookId });
}

export async function listPeople(bookId = 1): Promise<PersonSummary[]> {
  return apiGetWithTauriFallback<PersonSummary[]>(`/people`, "list_people", { bookId });
}

export async function listProjects(bookId = 1): Promise<ProjectSummary[]> {
  return apiGetWithTauriFallback<ProjectSummary[]>(`/projects`, "list_projects", { bookId });
}
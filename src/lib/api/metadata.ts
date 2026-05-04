import { apiDelete, apiGet, apiPost, apiPut } from "$lib/api/client";

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
  routing: string | null;
  website: string | null;
  metadata: string | null;
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

export type CategoryCreateInput = {
  book_id: number;
  parent_id: number | null;
  name: string;
  kind: string;
  color: string | null;
};

export type PayeeCreateInput = {
  book_id: number;
  name: string;
  kind: string;
  metadata: string | null;
};

export type TagCreateInput = {
  book_id: number;
  name: string;
  color: string | null;
};

export type PersonCreateInput = {
  book_id: number;
  name: string;
  role: string;
  metadata: string | null;
};

export type ProjectCreateInput = {
  book_id: number;
  name: string;
  status: string;
  metadata: string | null;
};

export type InstitutionCreateInput = {
  book_id: number;
  name: string;
  kind: string | null;
  country_id?: number | null;
};

export type InstitutionSettingsInput = {
  id?: number;
  book_id: number;
  name: string;
  kind: string | null;
  routing?: string | null;
  website?: string | null;
  metadata?: string | null;
  country_id?: number | null;
};

export async function listCommodities(bookId = 1): Promise<CommoditySummary[]> {
  void bookId;
  return apiGet<CommoditySummary[]>(`/commodities`);
}

export async function listCountries(bookId = 1): Promise<CountrySummary[]> {
  void bookId;
  return apiGet<CountrySummary[]>(`/countries`);
}

export async function listInstitutions(bookId = 1): Promise<InstitutionSummary[]> {
  void bookId;
  return apiGet<InstitutionSummary[]>(`/institutions`);
}

export async function listCategories(bookId = 1): Promise<CategorySummary[]> {
  void bookId;
  return apiGet<CategorySummary[]>(`/categories`);
}

export async function createCategory(input: CategoryCreateInput): Promise<CategorySummary> {
  return apiPost<CategorySummary, CategoryCreateInput>("/categories", input);
}

export async function updateCategory(category: CategoryCreateInput & { id: number }): Promise<void> {
  await apiPut<CategorySummary, CategoryCreateInput>(`/categories/${category.id}`, category);
}

export async function deleteCategory(categoryId: number, bookId = 1): Promise<void> {
  void bookId;
  await apiDelete<void>(`/categories/${categoryId}`);
}

export async function listPayees(bookId = 1): Promise<PayeeSummary[]> {
  void bookId;
  return apiGet<PayeeSummary[]>(`/payees`);
}

export async function createPayee(input: PayeeCreateInput): Promise<PayeeSummary> {
  return apiPost<PayeeSummary, PayeeCreateInput>("/payees", input);
}

export async function updatePayee(payee: PayeeCreateInput & { id: number }): Promise<void> {
  await apiPut<PayeeSummary, PayeeCreateInput>(`/payees/${payee.id}`, payee);
}

export async function deletePayee(payeeId: number, bookId = 1): Promise<void> {
  void bookId;
  await apiDelete<void>(`/payees/${payeeId}`);
}

export async function listTags(bookId = 1): Promise<TagSummary[]> {
  void bookId;
  return apiGet<TagSummary[]>(`/tags`);
}

export async function createTag(input: TagCreateInput): Promise<TagSummary> {
  return apiPost<TagSummary, TagCreateInput>("/tags", input);
}

export async function updateTag(tag: TagCreateInput & { id: number }): Promise<void> {
  await apiPut<TagSummary, TagCreateInput>(`/tags/${tag.id}`, tag);
}

export async function deleteTag(tagId: number, bookId = 1): Promise<void> {
  void bookId;
  await apiDelete<void>(`/tags/${tagId}`);
}

export async function listPeople(bookId = 1): Promise<PersonSummary[]> {
  void bookId;
  return apiGet<PersonSummary[]>(`/people`);
}

export async function createPerson(input: PersonCreateInput): Promise<PersonSummary> {
  return apiPost<PersonSummary, PersonCreateInput>("/people", input);
}

export async function listProjects(bookId = 1): Promise<ProjectSummary[]> {
  void bookId;
  return apiGet<ProjectSummary[]>(`/projects`);
}

export async function createProject(input: ProjectCreateInput): Promise<ProjectSummary> {
  return apiPost<ProjectSummary, ProjectCreateInput>("/projects", input);
}

export async function createInstitution(input: InstitutionCreateInput): Promise<InstitutionSummary> {
  return apiPost<InstitutionSummary, InstitutionCreateInput>("/institutions", input);
}

export async function saveInstitutionSettings(institution: InstitutionSettingsInput): Promise<void> {
  if (institution.id !== undefined) {
    await apiPut<InstitutionSummary, InstitutionSettingsInput>(`/institutions/${institution.id}`, institution);
    return;
  }

  await apiPost<InstitutionSummary, InstitutionSettingsInput>("/institutions", institution);
}

export async function deleteInstitution(institutionId: number, bookId = 1): Promise<void> {
  void bookId;
  await apiDelete<void>(`/institutions/${institutionId}`);
}
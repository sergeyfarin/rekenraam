import { apiDeleteWithTauriFallback, apiGetWithTauriFallback, apiPostWithTauriFallback, apiPutWithTauriFallback, invokeTauri } from "$lib/api/client";

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

export async function createCategory(input: CategoryCreateInput): Promise<CategorySummary> {
  return apiPostWithTauriFallback<CategorySummary, CategoryCreateInput>("/categories", input, "create_category", { input });
}

export async function updateCategory(category: CategoryCreateInput & { id: number }): Promise<void> {
  await apiPutWithTauriFallback<CategorySummary, CategoryCreateInput>(
    `/categories/${category.id}`,
    category,
    "update_category",
    { category }
  );
}

export async function deleteCategory(categoryId: number, bookId = 1): Promise<void> {
  await apiDeleteWithTauriFallback<void>(`/categories/${categoryId}`, "delete_category", { categoryId, bookId });
}

export async function listPayees(bookId = 1): Promise<PayeeSummary[]> {
  return apiGetWithTauriFallback<PayeeSummary[]>(`/payees`, "list_payees", { bookId });
}

export async function createPayee(input: PayeeCreateInput): Promise<PayeeSummary> {
  return apiPostWithTauriFallback<PayeeSummary, PayeeCreateInput>("/payees", input, "create_payee", { input });
}

export async function updatePayee(payee: PayeeCreateInput & { id: number }): Promise<void> {
  await apiPutWithTauriFallback<PayeeSummary, PayeeCreateInput>(`/payees/${payee.id}`, payee, "update_payee", { payee });
}

export async function deletePayee(payeeId: number, bookId = 1): Promise<void> {
  await apiDeleteWithTauriFallback<void>(`/payees/${payeeId}`, "delete_payee", { payeeId, bookId });
}

export async function listTags(bookId = 1): Promise<TagSummary[]> {
  return apiGetWithTauriFallback<TagSummary[]>(`/tags`, "list_tags", { bookId });
}

export async function createTag(input: TagCreateInput): Promise<TagSummary> {
  return apiPostWithTauriFallback<TagSummary, TagCreateInput>("/tags", input, "create_tag", { input });
}

export async function updateTag(tag: TagCreateInput & { id: number }): Promise<void> {
  await apiPutWithTauriFallback<TagSummary, TagCreateInput>(`/tags/${tag.id}`, tag, "update_tag", { tag });
}

export async function deleteTag(tagId: number, bookId = 1): Promise<void> {
  await apiDeleteWithTauriFallback<void>(`/tags/${tagId}`, "delete_tag", { tagId, bookId });
}

export async function listPeople(bookId = 1): Promise<PersonSummary[]> {
  return apiGetWithTauriFallback<PersonSummary[]>(`/people`, "list_people", { bookId });
}

export async function createPerson(input: PersonCreateInput): Promise<PersonSummary> {
  return apiPostWithTauriFallback<PersonSummary, PersonCreateInput>("/people", input, "create_person", { input });
}

export async function listProjects(bookId = 1): Promise<ProjectSummary[]> {
  return apiGetWithTauriFallback<ProjectSummary[]>(`/projects`, "list_projects", { bookId });
}

export async function createProject(input: ProjectCreateInput): Promise<ProjectSummary> {
  return apiPostWithTauriFallback<ProjectSummary, ProjectCreateInput>("/projects", input, "create_project", { input });
}

export async function createInstitution(input: InstitutionCreateInput): Promise<InstitutionSummary> {
  return apiPostWithTauriFallback<InstitutionSummary, InstitutionCreateInput>("/institutions", input, "create_institution", { input });
}

export async function saveInstitutionSettings(institution: InstitutionSettingsInput): Promise<void> {
  if (institution.id !== undefined) {
    await apiPutWithTauriFallback<InstitutionSummary, InstitutionSettingsInput>(
      `/institutions/${institution.id}`,
      institution,
      "update_institution",
      { institution }
    );
    return;
  }

  await apiPostWithTauriFallback<InstitutionSummary, InstitutionSettingsInput>("/institutions", institution, "create_institution", { input: institution });
}

export async function deleteInstitution(institutionId: number, bookId = 1): Promise<void> {
  await apiDeleteWithTauriFallback<void>(`/institutions/${institutionId}`, "delete_institution", { institutionId, bookId });
}
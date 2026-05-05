import { apiGet, apiPut } from "$lib/api/client";

export type UserPreferences = {
  default_book_id: number | null;
  locale: string | null;
  date_format: string;
  number_format: string;
  theme: string;
};

export async function getPreferences(): Promise<UserPreferences> {
  return apiGet<UserPreferences>("/preferences");
}

export async function updatePreferences(input: Partial<UserPreferences>): Promise<UserPreferences> {
  return apiPut<UserPreferences, Partial<UserPreferences>>("/preferences", input);
}

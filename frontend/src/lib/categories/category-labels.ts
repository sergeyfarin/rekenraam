import type { CategoryResponse } from '$lib/api/categories';
import { m } from '$lib/paraglide/messages.js';

export type CategoryType = CategoryResponse['category_type'];
export type CategoryStatus = CategoryResponse['status'];

export function categoryTypeLabel(categoryType: CategoryType): string {
  switch (categoryType) {
    case 'income':
      return m.category_type_income();
    case 'expense':
      return m.category_type_expense();
  }
}

export function categoryTypeRank(categoryType: CategoryType): number {
  switch (categoryType) {
    case 'income':
      return 1;
    case 'expense':
      return 2;
  }
}

export function categoryStatusLabel(status: CategoryStatus): string {
  switch (status) {
    case 'active':
      return m.category_status_active();
    case 'archived':
      return m.category_status_archived();
  }
}

export function categoryDisplayName(category: CategoryResponse): string {
  const userName = category.name?.trim();
  if (userName) {
    return userName;
  }

  if (category.is_builtin && category.builtin_key) {
    return builtinCategoryLabel(category.builtin_key);
  }

  return category.code?.trim() || m.categories_unnamed_category({ id: category.id });
}

export function categoryParentLabel(
  category: CategoryResponse,
  categoriesByID: ReadonlyMap<number, CategoryResponse>
): string {
  if (!category.parent_category_id) {
    return m.categories_parent_root();
  }

  const parent = categoriesByID.get(category.parent_category_id);
  return parent ? categoryDisplayName(parent) : m.categories_parent_unknown();
}

export function builtinCategoryLabel(key: string): string {
  switch (key) {
    case 'expense.housing':
      return m.category_builtin_expense_housing();
    case 'expense.housing.rent_mortgage':
      return m.category_builtin_expense_housing_rent_mortgage();
    case 'expense.housing.utilities':
      return m.category_builtin_expense_housing_utilities();
    case 'expense.housing.home_maintenance':
      return m.category_builtin_expense_housing_home_maintenance();
    case 'expense.housing.home_supplies':
      return m.category_builtin_expense_housing_home_supplies();
    case 'expense.food':
      return m.category_builtin_expense_food();
    case 'expense.food.groceries':
      return m.category_builtin_expense_food_groceries();
    case 'expense.food.dining_out':
      return m.category_builtin_expense_food_dining_out();
    case 'expense.food.coffee_snacks':
      return m.category_builtin_expense_food_coffee_snacks();
    case 'expense.transport':
      return m.category_builtin_expense_transport();
    case 'expense.transport.fuel':
      return m.category_builtin_expense_transport_fuel();
    case 'expense.transport.public_transport':
      return m.category_builtin_expense_transport_public_transport();
    case 'expense.transport.taxi_rideshare':
      return m.category_builtin_expense_transport_taxi_rideshare();
    case 'expense.transport.parking':
      return m.category_builtin_expense_transport_parking();
    case 'expense.transport.vehicle_maintenance':
      return m.category_builtin_expense_transport_vehicle_maintenance();
    case 'expense.health':
      return m.category_builtin_expense_health();
    case 'expense.health.medical':
      return m.category_builtin_expense_health_medical();
    case 'expense.health.pharmacy':
      return m.category_builtin_expense_health_pharmacy();
    case 'expense.health.dental_vision':
      return m.category_builtin_expense_health_dental_vision();
    case 'expense.health.fitness':
      return m.category_builtin_expense_health_fitness();
    case 'expense.insurance':
      return m.category_builtin_expense_insurance();
    case 'expense.insurance.health':
      return m.category_builtin_expense_insurance_health();
    case 'expense.insurance.home_renters':
      return m.category_builtin_expense_insurance_home_renters();
    case 'expense.insurance.vehicle':
      return m.category_builtin_expense_insurance_vehicle();
    case 'expense.insurance.life':
      return m.category_builtin_expense_insurance_life();
    case 'expense.family':
      return m.category_builtin_expense_family();
    case 'expense.family.childcare':
      return m.category_builtin_expense_family_childcare();
    case 'expense.family.education':
      return m.category_builtin_expense_family_education();
    case 'expense.family.pets':
      return m.category_builtin_expense_family_pets();
    case 'expense.family.gifts':
      return m.category_builtin_expense_family_gifts();
    case 'expense.personal':
      return m.category_builtin_expense_personal();
    case 'expense.personal.clothing':
      return m.category_builtin_expense_personal_clothing();
    case 'expense.personal.personal_care':
      return m.category_builtin_expense_personal_personal_care();
    case 'expense.personal.subscriptions':
      return m.category_builtin_expense_personal_subscriptions();
    case 'expense.personal.hobbies':
      return m.category_builtin_expense_personal_hobbies();
    case 'expense.travel':
      return m.category_builtin_expense_travel();
    case 'expense.travel.flights':
      return m.category_builtin_expense_travel_flights();
    case 'expense.travel.hotels':
      return m.category_builtin_expense_travel_hotels();
    case 'expense.travel.vacation_spending':
      return m.category_builtin_expense_travel_vacation_spending();
    case 'expense.financial':
      return m.category_builtin_expense_financial();
    case 'expense.financial.bank_fees':
      return m.category_builtin_expense_financial_bank_fees();
    case 'expense.financial.interest':
      return m.category_builtin_expense_financial_interest();
    case 'expense.financial.taxes':
      return m.category_builtin_expense_financial_taxes();
    case 'expense.giving':
      return m.category_builtin_expense_giving();
    case 'expense.giving.charity':
      return m.category_builtin_expense_giving_charity();
    case 'expense.giving.religious':
      return m.category_builtin_expense_giving_religious();
    case 'expense.other':
      return m.category_builtin_expense_other();
    case 'income.salary_wages':
      return m.category_builtin_income_salary_wages();
    case 'income.bonus':
      return m.category_builtin_income_bonus();
    case 'income.interest':
      return m.category_builtin_income_interest();
    case 'income.dividends':
      return m.category_builtin_income_dividends();
    case 'income.business_freelance':
      return m.category_builtin_income_business_freelance();
    case 'income.gifts_received':
      return m.category_builtin_income_gifts_received();
    case 'income.refunds_reimbursements':
      return m.category_builtin_income_refunds_reimbursements();
    case 'income.other':
      return m.category_builtin_income_other();
    default:
      return key;
  }
}

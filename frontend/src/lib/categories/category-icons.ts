import type { CategoryResponse } from '$lib/api/categories';
import { m } from '$lib/paraglide/messages.js';

export const categoryIconOptions = [
  'tags',
  'house',
  'shopping-cart',
  'utensils',
  'coffee',
  'car',
  'bus',
  'fuel',
  'heart-pulse',
  'shield',
  'users',
  'graduation-cap',
  'gift',
  'shirt',
  'plane',
  'landmark',
  'circle-dollar-sign',
  'briefcase-business',
  'banknote',
  'receipt',
  'wallet-cards',
  'hand-heart',
  'wrench',
  'ellipsis'
] as const;

export type CategoryIconName = (typeof categoryIconOptions)[number];

const categoryIconSet = new Set<string>(categoryIconOptions);

export function categoryIconName(category: CategoryResponse | undefined): CategoryIconName {
  if (category && isCategoryIconName(category.icon)) {
    return category.icon;
  }

  return inferredCategoryIconName(category);
}

export function isCategoryIconName(value: unknown): value is CategoryIconName {
  return typeof value === 'string' && categoryIconSet.has(value);
}

export function categoryIconLabel(iconName: CategoryIconName): string {
  switch (iconName) {
    case 'tags':
      return m.category_icon_tags();
    case 'house':
      return m.category_icon_house();
    case 'shopping-cart':
      return m.category_icon_shopping_cart();
    case 'utensils':
      return m.category_icon_utensils();
    case 'coffee':
      return m.category_icon_coffee();
    case 'car':
      return m.category_icon_car();
    case 'bus':
      return m.category_icon_bus();
    case 'fuel':
      return m.category_icon_fuel();
    case 'heart-pulse':
      return m.category_icon_heart_pulse();
    case 'shield':
      return m.category_icon_shield();
    case 'users':
      return m.category_icon_users();
    case 'graduation-cap':
      return m.category_icon_graduation_cap();
    case 'gift':
      return m.category_icon_gift();
    case 'shirt':
      return m.category_icon_shirt();
    case 'plane':
      return m.category_icon_plane();
    case 'landmark':
      return m.category_icon_landmark();
    case 'circle-dollar-sign':
      return m.category_icon_circle_dollar_sign();
    case 'briefcase-business':
      return m.category_icon_briefcase_business();
    case 'banknote':
      return m.category_icon_banknote();
    case 'receipt':
      return m.category_icon_receipt();
    case 'wallet-cards':
      return m.category_icon_wallet_cards();
    case 'hand-heart':
      return m.category_icon_hand_heart();
    case 'wrench':
      return m.category_icon_wrench();
    case 'ellipsis':
      return m.category_icon_ellipsis();
  }
}

function inferredCategoryIconName(category: CategoryResponse | undefined): CategoryIconName {
  const key = category?.builtin_key ?? '';
  if (key.startsWith('expense.housing')) {
    return 'house';
  }
  if (key === 'expense.food.groceries') {
    return 'shopping-cart';
  }
  if (key === 'expense.food.dining_out') {
    return 'utensils';
  }
  if (key === 'expense.food.coffee_snacks') {
    return 'coffee';
  }
  if (key.startsWith('expense.food')) {
    return 'utensils';
  }
  if (key === 'expense.transport.public_transport') {
    return 'bus';
  }
  if (key === 'expense.transport.fuel') {
    return 'fuel';
  }
  if (key.startsWith('expense.transport')) {
    return 'car';
  }
  if (key === 'expense.health.fitness') {
    return 'heart-pulse';
  }
  if (key.startsWith('expense.health')) {
    return 'heart-pulse';
  }
  if (key.startsWith('expense.insurance')) {
    return 'shield';
  }
  if (key === 'expense.family.education') {
    return 'graduation-cap';
  }
  if (key === 'expense.family.gifts') {
    return 'gift';
  }
  if (key.startsWith('expense.family')) {
    return 'users';
  }
  if (key === 'expense.personal.clothing') {
    return 'shirt';
  }
  if (key.startsWith('expense.travel')) {
    return 'plane';
  }
  if (key.startsWith('expense.financial')) {
    return 'landmark';
  }
  if (key.startsWith('expense.giving')) {
    return 'hand-heart';
  }
  if (key === 'income.salary_wages' || key === 'income.bonus') {
    return 'banknote';
  }
  if (key === 'income.business_freelance') {
    return 'briefcase-business';
  }
  if (key === 'income.interest' || key === 'income.dividends') {
    return 'circle-dollar-sign';
  }
  if (key === 'income.gifts_received') {
    return 'gift';
  }
  if (key === 'income.refunds_reimbursements') {
    return 'receipt';
  }
  if (key === 'income.other' || key === 'expense.other') {
    return 'ellipsis';
  }

  return category?.category_type === 'income' ? 'circle-dollar-sign' : 'tags';
}

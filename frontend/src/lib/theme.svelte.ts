export const themeNames = ['light', 'dark'] as const;

export type ThemeName = (typeof themeNames)[number];

type ThemeState = {
  name: ThemeName;
  initialized: boolean;
};

const storageKey = 'rekenraam-theme';

export const themeState = $state<ThemeState>({
  name: 'light',
  initialized: false
});

let mediaQuery: MediaQueryList | null = null;
let mediaQueryBound = false;

export function initializeTheme(): void {
  if (typeof window === 'undefined') {
    return;
  }

  syncTheme(readStoredTheme() ?? readSystemTheme());
  themeState.initialized = true;

  if (mediaQueryBound) {
    return;
  }

  mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
  mediaQuery.addEventListener('change', handleSystemThemeChange);
  mediaQueryBound = true;
}

export function setTheme(themeName: ThemeName): void {
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(storageKey, themeName);
  }

  syncTheme(themeName);
  themeState.initialized = true;
}

export function toggleTheme(): void {
  setTheme(themeState.name === 'light' ? 'dark' : 'light');
}

export function isThemeName(value: string | null): value is ThemeName {
  return value === 'light' || value === 'dark';
}

function readStoredTheme(): ThemeName | null {
  if (typeof window === 'undefined') {
    return null;
  }

  const storedTheme = window.localStorage.getItem(storageKey);
  return isThemeName(storedTheme) ? storedTheme : null;
}

function readSystemTheme(): ThemeName {
  if (typeof window === 'undefined') {
    return 'light';
  }

  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function handleSystemThemeChange(): void {
  if (readStoredTheme() !== null) {
    return;
  }

  syncTheme(readSystemTheme());
}

function syncTheme(themeName: ThemeName): void {
  themeState.name = themeName;

  if (typeof document === 'undefined') {
    return;
  }

  document.documentElement.dataset.theme = themeName;
  document.documentElement.style.colorScheme = themeName;
}
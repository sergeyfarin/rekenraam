(() => {
  const storageKey = 'rekenraam-theme';
  const baseColorStorageKey = 'rekenraam-base-color';
  const accentColorStorageKey = 'rekenraam-accent-color';
  const baseColors = ['olive', 'slate', 'zinc'];
  const accentColors = ['emerald', 'blue', 'violet', 'rose', 'amber'];
  const storedTheme = window.localStorage.getItem(storageKey);
  const storedBaseColor = window.localStorage.getItem(baseColorStorageKey);
  const storedAccentColor = window.localStorage.getItem(accentColorStorageKey);
  const theme = storedTheme === 'light' || storedTheme === 'dark'
    ? storedTheme
    : window.matchMedia('(prefers-color-scheme: dark)').matches
      ? 'dark'
      : 'light';

  document.documentElement.dataset.theme = theme;
  document.documentElement.dataset.baseColor = baseColors.includes(storedBaseColor) ? storedBaseColor : 'olive';
  document.documentElement.dataset.accentColor = accentColors.includes(storedAccentColor) ? storedAccentColor : 'emerald';
  document.documentElement.style.colorScheme = theme;
})();

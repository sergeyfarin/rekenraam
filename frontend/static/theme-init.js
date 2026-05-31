(() => {
  const storageKey = 'rekenraam-theme';
  const storedTheme = window.localStorage.getItem(storageKey);
  const theme = storedTheme === 'light' || storedTheme === 'dark'
    ? storedTheme
    : window.matchMedia('(prefers-color-scheme: dark)').matches
      ? 'dark'
      : 'light';

  document.documentElement.dataset.theme = theme;
  document.documentElement.style.colorScheme = theme;
})();

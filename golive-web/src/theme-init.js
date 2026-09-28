// Applies the colour theme before first paint so dark-mode users don't get a
// white flash while the app bundle loads. index.html loads this as a classic,
// render-blocking script (an external file because the production CSP forbids
// inline scripts); vite.config.ts emits it with a content hash. Keep the rules
// in sync with src/stores/useThemeStore.ts.
(function () {
  var theme = null;
  try {
    var stored = localStorage.getItem('golive-theme');
    var explicit = localStorage.getItem('golive-theme-explicit') === '1';
    // Older builds saved "light" on every first visit, so only an explicit
    // "light" (or any "dark") counts as the user's choice.
    if (stored === 'dark' || (stored === 'light' && explicit)) theme = stored;
  } catch (e) {
    /* storage unavailable: fall back to the system setting */
  }
  if (!theme) {
    theme =
      window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches
        ? 'dark'
        : 'light';
  }
  var root = document.documentElement;
  if (theme === 'dark') root.classList.add('dark');
  root.style.colorScheme = theme;
  var meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute('content', theme === 'dark' ? '#0f0f0f' : '#ffffff');
})();

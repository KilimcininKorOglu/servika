// Flat config (ESLint 9/10). The panel is a browser-only SPA, so the browser
// globals are the whole runtime surface; anything Node-shaped belongs to the
// build tooling, which is linted separately below.
//
// Type-aware rules are deliberately not enabled: `tsc --noEmit` already runs as
// its own gate in CI, so duplicating type checking here would double the lint
// time without catching anything new.

import js from '@eslint/js'
import globals from 'globals'
import tseslint from 'typescript-eslint'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'

export default tseslint.config(
  { ignores: ['dist', 'node_modules'] },
  {
    files: ['**/*.{ts,tsx}'],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
      // An unused function argument is often a signature the caller dictates, so
      // only flag it when it is not deliberately underscore-prefixed.
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      // Every browser offers to suppress these after a few in one tab, and from
      // then on alert() does nothing while confirm() always answers false, so a
      // failure reported only through one of them disappears and a confirmed
      // action silently cancels. useDialog draws the same three shapes in the
      // panel's own theme, and a local binding of the same name is not the
      // global, so a converted call site does not trip this.
      'no-restricted-globals': ['error',
        { name: 'alert', message: 'Use notify() from useDialog(): browsers suppress repeated native dialogs.' },
        { name: 'confirm', message: 'Use confirm() from useDialog(): a suppressed native confirm always answers false.' },
        { name: 'prompt', message: 'Use ask() from useDialog(): a suppressed native prompt always answers null.' },
      ],
      // The rule above sees only the bare global, and this codebase wrote
      // `window.confirm(...)` as readily as `confirm(...)`, so the same three
      // have to be barred through the window object as well.
      'no-restricted-properties': ['error',
        { object: 'window', property: 'alert', message: 'Use notify() from useDialog().' },
        { object: 'window', property: 'confirm', message: 'Use confirm() from useDialog().' },
        { object: 'window', property: 'prompt', message: 'Use ask() from useDialog().' },
      ],
    },
  },
  {
    // Build tooling runs in Node, not the browser.
    files: ['*.config.{js,ts}', 'scripts/**/*.{js,mjs,ts}'],
    languageOptions: { globals: globals.node },
  },
)

// data-theme on <html> from settings; "system" follows prefers-color-scheme
// (prism-ui skill §Theme).
const media = window.matchMedia('(prefers-color-scheme: dark)')
let current = 'system'

function apply() {
  const dark = current === 'dark' || (current === 'system' && media.matches)
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
}

media.addEventListener('change', apply)

export function setTheme(theme: string) {
  current = theme || 'system'
  apply()
}

apply()

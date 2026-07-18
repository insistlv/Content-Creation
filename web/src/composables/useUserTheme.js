import { computed, ref } from 'vue'

export const USER_THEME_STORAGE_KEY = 'image_agent_user_theme:v1'

const allowedThemes = new Set(['dark', 'light'])
const theme = ref('dark')

function readStoredTheme() {
  if (typeof window === 'undefined') {
    return 'dark'
  }

  try {
    const storedTheme = window.localStorage?.getItem(USER_THEME_STORAGE_KEY)
    return allowedThemes.has(storedTheme) ? storedTheme : 'dark'
  } catch {
    return 'dark'
  }
}

function persistTheme(nextTheme) {
  if (typeof window === 'undefined') {
    return
  }

  try {
    window.localStorage?.setItem(USER_THEME_STORAGE_KEY, nextTheme)
  } catch {
    // Theme persistence is a browser convenience; rendering should not depend on it.
  }
}

function setTheme(nextTheme) {
  theme.value = allowedThemes.has(nextTheme) ? nextTheme : 'dark'
  persistTheme(theme.value)
}

export function useUserTheme() {
  theme.value = readStoredTheme()

  const isDarkTheme = computed(() => theme.value === 'dark')

  function toggleTheme() {
    setTheme(isDarkTheme.value ? 'light' : 'dark')
  }

  return {
    theme,
    isDarkTheme,
    toggleTheme
  }
}

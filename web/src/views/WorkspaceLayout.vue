<script setup>
import { useRoute, useRouter } from 'vue-router'
import { computed, ref, onMounted, watch } from 'vue'
import { Menu, X } from 'lucide-vue-next'
import { api } from '../api/client.js'
import { clearCurrentUser, currentUser, ensureUserSession, loadCurrentUser } from '../stores/session.js'
import { usePresenceHeartbeat } from '../composables/usePresenceHeartbeat.js'
import { confirmLoginBeforeUse } from '../auth-navigation.js'
import { useUserTheme } from '../composables/useUserTheme.js'
import WorkspaceSidebar from '../components/WorkspaceSidebar.vue'
import AnnouncementPopup from '../components/AnnouncementPopup.vue'
import AuthModal from '../components/AuthModal.vue'
import { openAuthModal } from '../stores/auth-modal.js'

const route = useRoute()
const router = useRouter()
const me = currentUser
const pageError = ref('')
const sidebarOpen = ref(false)
const { theme } = useUserTheme()
const sidebarToggleLabel = computed(() => (sidebarOpen.value ? '收起菜单' : '展开菜单'))
const isLoggedIn = computed(() => Boolean(me.value?.username))
const themeShellClass = computed(() => (theme.value === 'light' ? 'user-light-shell' : 'user-dark-shell'))
const themeName = computed(() => (theme.value === 'light' ? 'user-light' : 'user-dark'))

usePresenceHeartbeat(isLoggedIn)

async function loadSession() {
  try {
    await loadCurrentUser({ force: true })
  } catch (error) {
    if (error?.status === 401) {
      pageError.value = ''
      return
    }
    pageError.value = error.message
  }
}

async function handleNavigate(routePath) {
  sidebarOpen.value = false
  if (routePath !== '/workspace') {
    const hasSession = await ensureUserSession()
    if (!hasSession) {
      confirmLoginBeforeUse(router, routePath)
      return
    }
    router.push(routePath)
    return
  }
  router.push(routePath)
}

function handleRecharge() {
  sidebarOpen.value = false
  if (!isLoggedIn.value) {
    confirmLoginBeforeUse(router, '/workspace')
    return
  }
  router.push('/pricing')
}

function handleSupport() {
  sidebarOpen.value = false
  router.push('/contact')
}

async function handleLogout() {
  try {
    await api.logout()
  } finally {
    clearCurrentUser()
    sidebarOpen.value = false
    router.push('/login')
  }
}

function toggleSidebar() {
  sidebarOpen.value = !sidebarOpen.value
}

function closeSidebar() {
  sidebarOpen.value = false
}

function firstQueryValue(value) {
  return Array.isArray(value) ? value[0] : value
}

function syncAuthModalFromRoute() {
  const mode = firstQueryValue(route.query?.auth)
  if (mode !== 'login' && mode !== 'register') {
    return
  }
  openAuthModal({
    mode,
    redirect: firstQueryValue(route.query?.redirect) || '/workspace',
    navigateOnSuccess: true,
    message: '需要登录才能使用该功能'
  })
}

onMounted(() => {
  loadSession()
  syncAuthModalFromRoute()
})

watch(
  () => [route.query?.auth, route.query?.redirect],
  syncAuthModalFromRoute
)
</script>

<template>
  <div v-if="pageError" class="workspace-error">
    <p>{{ pageError }}</p>
  </div>

  <div
    v-else
    :class="['workspace-with-sidebar', themeShellClass, { 'workspace-sidebar-open': sidebarOpen }]"
    :data-theme="themeName"
  >
    <div class="workspace-mobile-topbar">
      <button
        class="workspace-sidebar-toggle"
        type="button"
        data-testid="workspace-sidebar-toggle"
        :aria-expanded="String(sidebarOpen)"
        aria-controls="workspace-sidebar-panel"
        @click="toggleSidebar"
      >
        <component :is="sidebarOpen ? X : Menu" :size="18" />
        <span>{{ sidebarToggleLabel }}</span>
      </button>
    </div>

    <button
      v-if="sidebarOpen"
      class="workspace-sidebar-backdrop"
      type="button"
      data-testid="workspace-sidebar-backdrop"
      aria-label="收起菜单"
      @click="closeSidebar"
    />

    <div id="workspace-sidebar-panel" class="workspace-sidebar-shell" data-testid="workspace-sidebar-shell">
      <button
        v-if="sidebarOpen"
        class="workspace-sidebar-close"
        type="button"
        data-testid="workspace-sidebar-close"
        aria-label="收起菜单"
        @click="closeSidebar"
      >
        <X :size="18" />
      </button>
      <WorkspaceSidebar
        :me="me"
        :current-route="route.path"
        :theme="theme"
        @navigate="handleNavigate"
        @recharge="handleRecharge"
        @support="handleSupport"
        @logout="handleLogout"
      />
    </div>

    <div class="workspace-content">
      <router-view />
    </div>
    <AnnouncementPopup :enabled="isLoggedIn" client="web" />
    <AuthModal />
  </div>
</template>

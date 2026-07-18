<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Menu, X } from 'lucide-vue-next'

import { api } from '../api/client.js'
import { clearCurrentUser, currentUser, ensureUserSession } from '../stores/session.js'
import { usePresenceHeartbeat } from '../composables/usePresenceHeartbeat.js'
import { useUserTheme } from '../composables/useUserTheme.js'
import AnnouncementPopup from '../components/AnnouncementPopup.vue'
import ThemeToggle from '../components/ThemeToggle.vue'
import WorkspaceSidebar from '../components/WorkspaceSidebar.vue'

const route = useRoute()
const router = useRouter()
const menuOpen = ref(false)
const { theme } = useUserTheme()
const isHomeRoute = computed(() => route?.path === '/')
const isWorkspaceRoute = computed(() => route?.path === '/workspace')
const isWorksRoute = computed(() => route?.path === '/works')
const isPricingRoute = computed(() => route?.path === '/pricing')
const isContactRoute = computed(() => route?.path === '/contact')
const isAccountRoute = computed(() => route?.path === '/account')
const isAssetsRoute = computed(() => route?.path === '/assets')
const isAuthRoute = computed(() => route?.path === '/login' || route?.path === '/register')
const usesUserSidebar = computed(() => [
  '/works',
  '/assets',
  '/pricing',
  '/account',
  '/contact'
].includes(route?.path) || route?.path?.startsWith('/checkout/'))
const isLoggedIn = computed(() => Boolean(currentUser.value?.username))
const themeShellClass = computed(() => (theme.value === 'light' ? 'user-light-shell' : 'user-dark-shell'))
const themeName = computed(() => (theme.value === 'light' ? 'user-light' : 'user-dark'))
const sidebarToggleLabel = computed(() => (menuOpen.value ? '收起菜单' : '展开菜单'))
const userDisplayName = computed(() => (
  currentUser.value?.display_name || currentUser.value?.username || '用户'
))
const userAvatarLabel = computed(() => userDisplayName.value.trim().slice(0, 1).toUpperCase() || 'U')

usePresenceHeartbeat(isLoggedIn)

function toggleMobileMenu() {
  menuOpen.value = !menuOpen.value
}

function closeMobileMenu() {
  menuOpen.value = false
}

function navigateTo(routePath) {
  closeMobileMenu()
  router.push(routePath)
}

function handleRecharge() {
  closeMobileMenu()
  router.push('/pricing')
}

function handleSupport() {
  closeMobileMenu()
  router.push('/contact')
}

async function loadCurrentUser() {
  await ensureUserSession()
}

async function logout() {
  try {
    await api.logout()
  } finally {
    clearCurrentUser()
    closeMobileMenu()
    router.push('/login')
  }
}

onMounted(loadCurrentUser)

watch(
  () => route?.path,
  () => {
    closeMobileMenu()
    loadCurrentUser()
  }
)
</script>

<template>
  <div
    :class="[
      'site-shell',
      themeShellClass,
      {
        'site-shell-home': isHomeRoute,
        'site-shell-workspace': isWorkspaceRoute,
        'site-shell-works': isWorksRoute,
        'site-shell-pricing': isPricingRoute,
        'site-shell-contact': isContactRoute,
        'site-shell-account': isAccountRoute,
        'site-shell-assets': isAssetsRoute,
        'site-shell-auth': isAuthRoute,
        'site-shell-user-sidebar': usesUserSidebar
      }
    ]"
    :data-theme="themeName"
  >
    <template v-if="usesUserSidebar">
      <div :class="['site-user-layout', { 'workspace-sidebar-open': menuOpen }]">
        <div class="site-user-mobile-topbar">
          <button
            class="workspace-sidebar-toggle"
            type="button"
            data-testid="site-mobile-menu-toggle"
            :aria-expanded="String(menuOpen)"
            aria-controls="site-primary-nav"
            @click="toggleMobileMenu"
          >
            <component :is="menuOpen ? X : Menu" :size="18" />
            <span>{{ sidebarToggleLabel }}</span>
          </button>
        </div>

        <button
          v-if="menuOpen"
          class="workspace-sidebar-backdrop"
          type="button"
          data-testid="workspace-sidebar-backdrop"
          aria-label="收起菜单"
          @click="closeMobileMenu"
        />

        <div id="site-primary-nav" class="workspace-sidebar-shell site-user-sidebar-shell" data-testid="workspace-sidebar-shell">
          <button
            v-if="menuOpen"
            class="workspace-sidebar-close"
            type="button"
            data-testid="workspace-sidebar-close"
            aria-label="收起菜单"
            @click="closeMobileMenu"
          >
            <X :size="18" />
          </button>
          <WorkspaceSidebar
            :me="currentUser"
            :current-route="route.path"
            :theme="theme"
            user-menu-toggle-test-id="site-user-menu-toggle"
            user-menu-test-id="site-user-menu"
            logout-button-test-id="site-logout-button"
            @navigate="navigateTo"
            @recharge="handleRecharge"
            @support="handleSupport"
            @logout="logout"
          />
        </div>

        <main class="site-main site-user-main">
          <div class="site-content-shell">
            <RouterView />
          </div>
        </main>
      </div>
    </template>

    <template v-else>
      <header class="site-header">
      <div class="site-header-shell">
        <RouterLink class="brand-mark" to="/" aria-label="白霖共享 首页">
          <span class="brand-star" aria-hidden="true"></span>
          <strong>白霖共享</strong>
          <span class="brand-divider" aria-hidden="true"></span>
          <span class="brand-tagline">创作者 AI 图片平台</span>
        </RouterLink>

        <nav
          id="site-primary-nav"
          :class="['site-nav', { 'site-nav-open': menuOpen }]"
          aria-label="站点导航"
        >
          <RouterLink class="nav-link" to="/workspace" @click="closeMobileMenu">工作台</RouterLink>
          <RouterLink class="nav-link" to="/works" @click="closeMobileMenu">作品库</RouterLink>
          <RouterLink class="nav-link" to="/pricing" @click="closeMobileMenu">套餐</RouterLink>
          <RouterLink class="nav-link" to="/account" @click="closeMobileMenu">账户</RouterLink>
          <RouterLink class="nav-link" to="/contact" @click="closeMobileMenu">联系客服</RouterLink>
          <RouterLink v-if="!isLoggedIn" class="nav-link nav-link-login" to="/login" @click="closeMobileMenu">登录</RouterLink>
          <RouterLink v-if="!isLoggedIn" class="nav-link nav-link-register" to="/register" @click="closeMobileMenu">注册</RouterLink>

          <div
            v-if="isLoggedIn"
            :class="['site-user-menu', { 'site-user-menu-open': menuOpen }]"
            data-testid="site-user-menu"
          >
            <div class="site-user-menu-head">
              <span>{{ userAvatarLabel }}</span>
              <div>
                <strong>{{ userDisplayName }}</strong>
                <small>{{ currentUser?.username }}</small>
              </div>
            </div>
            <RouterLink
              class="site-user-menu-item"
              data-testid="site-account-link"
              to="/account"
              @click="closeMobileMenu"
            >
              账户中心
            </RouterLink>
            <RouterLink
              class="site-user-menu-item"
              data-testid="site-password-link"
              to="/account#security"
              @click="closeMobileMenu"
            >
              修改密码
            </RouterLink>
            <button
              class="site-user-menu-item site-user-menu-logout"
              data-testid="site-logout-button"
              type="button"
              @click="logout"
            >
              退出平台
            </button>
          </div>
        </nav>

        <div class="site-header-actions">
          <ThemeToggle class="site-theme-toggle" />

          <button
            v-if="!isLoggedIn"
            class="site-mobile-menu-button"
            data-testid="site-mobile-menu-toggle"
            type="button"
            :aria-expanded="menuOpen ? 'true' : 'false'"
            aria-controls="site-primary-nav"
            @click="toggleMobileMenu"
          >
            <span aria-hidden="true"></span>
            {{ menuOpen ? '收起' : '菜单' }}
          </button>

          <button
            v-else
            class="site-user-menu-button"
            data-testid="site-user-menu-toggle"
            type="button"
            :aria-label="`打开 ${userDisplayName} 的账户菜单`"
            :aria-expanded="menuOpen ? 'true' : 'false'"
            aria-controls="site-primary-nav"
            @click="toggleMobileMenu"
          >
            <span class="site-user-avatar" aria-hidden="true">{{ userAvatarLabel }}</span>
            <span class="site-user-name">{{ userDisplayName }}</span>
          </button>
        </div>
      </div>
      </header>

      <main class="site-main">
        <div class="site-content-shell">
          <RouterView />
        </div>
      </main>
    </template>
    <AnnouncementPopup :enabled="isLoggedIn" client="web" />
  </div>
</template>

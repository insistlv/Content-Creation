<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import AuthForm from '../components/AuthForm.vue'
import { resolveSafeRedirect } from '../auth-navigation.js'

const props = defineProps({
  mode: {
    type: String,
    default: 'login'
  }
})

const route = useRoute()
const router = useRouter()

const initialReset = computed(() => firstQueryValue(route.query?.reset) === '1')
const initialResetPhone = computed(() => firstQueryValue(route.query?.phone) || '')

function firstQueryValue(value) {
  return Array.isArray(value) ? value[0] : value
}

function handleModeChange(mode) {
  const path = mode === 'register' ? '/register' : '/login'
  if (route.path !== path) {
    router.push(path)
  }
}

function handleAuthenticated() {
  router.push(resolveSafeRedirect(route.query?.redirect, '/workspace'))
}
</script>

<template>
  <AuthForm
    :mode="props.mode"
    :initial-reset="initialReset"
    :initial-reset-phone="initialResetPhone"
    @mode-change="handleModeChange"
    @authenticated="handleAuthenticated"
  />
</template>

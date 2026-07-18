<script setup>
import { computed, onMounted, ref } from 'vue'
import { Bell, CheckCircle2, Clipboard, Headphones, MessageCircle, Phone, ShieldCheck, Smartphone, Zap } from 'lucide-vue-next'

import { api } from '../api/client.js'

const config = ref(null)
const loading = ref(false)
const errorMessage = ref('')
const copyMessage = ref('')

const serviceTags = computed(() => config.value?.service_tags ?? [])
const stats = computed(() => config.value?.stats ?? [])
const features = computed(() => config.value?.features ?? [])
const faqs = computed(() => config.value?.faqs ?? [])

const featureIcons = [Zap, Headphones, Smartphone, ShieldCheck]
const statIcons = [Phone, Zap, ShieldCheck]

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    config.value = await api.getCustomerService()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

async function copyText(text, label) {
  copyMessage.value = ''
  try {
    await navigator?.clipboard?.writeText?.(text)
    copyMessage.value = `${label}已复制`
  } catch {
    copyMessage.value = `${label}：${text}`
  }
}

function channelIcon(channel) {
  return channel === 'wechat' ? MessageCircle : Bell
}

function qrFallbackText(channel) {
  return channel === 'wechat' ? '微信' : 'QQ'
}

onMounted(load)
</script>

<template>
  <section class="contact-service-page">
    <p v-if="loading" class="page-status">正在加载客服信息...</p>
    <p v-else-if="errorMessage" class="status-error">{{ errorMessage }}</p>

    <template v-else-if="config">
      <div class="contact-hero-grid">
        <div class="contact-copy">
          <p class="contact-eyebrow">✦ {{ config.eyebrow }}</p>
          <h1>{{ config.title }}</h1>
          <h2>{{ config.subtitle }}</h2>
          <p>{{ config.description }}</p>
          <div class="contact-primary-actions">
            <button class="contact-copy-button wechat" type="button" @click="copyText(config.wechat.account, '微信号')">
              <MessageCircle :size="20" />
              复制微信号
            </button>
            <button class="contact-copy-button qq" type="button" @click="copyText(config.qq.account, 'QQ号')">
              <Bell :size="20" />
              复制QQ号
            </button>
            <RouterLink class="contact-copy-button report" to="/content-report">
              <ShieldCheck :size="20" />
              算法/内容投诉
            </RouterLink>
          </div>
          <div class="contact-tags">
            <span v-for="tag in serviceTags" :key="tag">{{ tag }}</span>
          </div>
          <p v-if="copyMessage" class="status-success">{{ copyMessage }}</p>
        </div>

        <div class="contact-card-wall">
          <div class="contact-channel-grid">
            <article
              v-for="channel in ['wechat', 'qq']"
              :key="channel"
              class="contact-channel-card"
            >
              <h3>
                <component :is="channelIcon(channel)" :size="24" />
                {{ config[channel].label }}
              </h3>
              <div class="contact-qr-frame">
                <img
                  v-if="config[channel].qr_url"
                  :src="config[channel].qr_url"
                  :alt="`${config[channel].label}二维码`"
                  :data-testid="`${channel}-qr`"
                />
                <div v-else class="contact-qr-fallback" :data-testid="`${channel}-qr`">
                  <span>{{ qrFallbackText(channel) }}</span>
                </div>
              </div>
              <p class="contact-channel-tip">{{ channel === 'wechat' ? '移动端可长按识别二维码添加微信' : '扫码添加 QQ 客服' }}</p>
              <strong>{{ channel === 'wechat' ? '微信号：' : 'QQ：' }} {{ config[channel].account }}</strong>
              <button type="button" @click="copyText(config[channel].account, channel === 'wechat' ? '微信号' : 'QQ号')">
                <Clipboard :size="16" />
                {{ channel === 'wechat' ? '复制微信号' : '复制QQ号' }}
              </button>
            </article>
          </div>

          <div class="contact-stats">
            <article v-for="(item, index) in stats" :key="item.label">
              <component :is="statIcons[index] ?? CheckCircle2" :size="25" />
              <div>
                <span>{{ item.label }}</span>
                <strong>{{ item.value }}</strong>
              </div>
            </article>
          </div>
        </div>
      </div>

      <div class="contact-bottom-grid">
        <article v-for="(item, index) in features" :key="item.title" class="contact-feature-card">
          <span><component :is="featureIcons[index] ?? CheckCircle2" :size="28" /></span>
          <strong>{{ item.title }}</strong>
          <p>{{ item.text }}</p>
        </article>

        <section class="contact-faq-panel">
          <div class="contact-faq-head">
            <h2>常见问题</h2>
            <RouterLink to="/pricing">查看全部帮助中心 →</RouterLink>
          </div>
          <RouterLink v-for="item in faqs" :key="item.title" class="contact-faq-row" :to="item.url || '/contact'">
            <span>Q</span>
            <strong>{{ item.title }}</strong>
            <b>›</b>
          </RouterLink>
        </section>
      </div>
    </template>
  </section>
</template>

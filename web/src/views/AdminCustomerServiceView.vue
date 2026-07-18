<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { MessageCircle, Save, Upload } from 'lucide-vue-next'

import { api } from '../api/client.js'

const loading = ref(false)
const saving = ref(false)
const uploadingChannel = ref('')
const message = ref('')
const errorMessage = ref('')

const form = reactive(blankConfig())

const serviceTagsText = computed({
  get: () => form.service_tags.join('\n'),
  set: (value) => {
    form.service_tags = lines(value)
  }
})

const statsText = computed({
  get: () => form.stats.map((item) => `${item.label}|${item.value}`).join('\n'),
  set: (value) => {
    form.stats = pipeRows(value, ['label', 'value'])
  }
})

const featuresText = computed({
  get: () => form.features.map((item) => `${item.title}|${item.text}`).join('\n'),
  set: (value) => {
    form.features = pipeRows(value, ['title', 'text'])
  }
})

const faqsText = computed({
  get: () => form.faqs.map((item) => `${item.title}|${item.url}`).join('\n'),
  set: (value) => {
    form.faqs = pipeRows(value, ['title', 'url'])
  }
})

function blankConfig() {
  return {
    title: '',
    eyebrow: '',
    subtitle: '',
    description: '',
    wechat: { label: '', account: '', qr_url: '' },
    qq: { label: '', account: '', qr_url: '' },
    service_tags: [],
    stats: [],
    features: [],
    faqs: []
  }
}

function assignConfig(value = {}) {
  Object.assign(form, blankConfig(), value)
  form.wechat = { ...blankConfig().wechat, ...(value.wechat ?? {}) }
  form.qq = { ...blankConfig().qq, ...(value.qq ?? {}) }
  form.service_tags = value.service_tags ?? []
  form.stats = value.stats ?? []
  form.features = value.features ?? []
  form.faqs = value.faqs ?? []
}

function lines(value) {
  return `${value}`.split('\n').map((item) => item.trim()).filter(Boolean)
}

function pipeRows(value, keys) {
  return `${value}`.split('\n').map((line) => {
    const parts = line.split('|').map((item) => item.trim())
    return Object.fromEntries(keys.map((key, index) => [key, parts[index] ?? '']))
  }).filter((item) => item[keys[0]])
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    assignConfig(await api.getAdminCustomerService())
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  message.value = ''
  errorMessage.value = ''
  try {
    await api.updateAdminCustomerService(JSON.parse(JSON.stringify(form)))
    message.value = '客服配置已保存'
    await load()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    saving.value = false
  }
}

async function uploadQRCode(channel, event) {
  const file = event.target.files?.[0]
  if (!file) return

  uploadingChannel.value = channel
  message.value = ''
  errorMessage.value = ''
  try {
    const uploaded = await api.uploadCustomerServiceQRCode(file)
    form[channel].qr_url = uploaded.url
    await api.updateAdminCustomerService(JSON.parse(JSON.stringify(form)))
    message.value = `${channel === 'wechat' ? '微信' : 'QQ'}二维码已上传并保存`
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    uploadingChannel.value = ''
    event.target.value = ''
  }
}

onMounted(load)
</script>

<template>
  <section class="admin-customer-service-page">
    <div class="admin-page-heading">
      <div>
        <p class="eyebrow">Customer service</p>
        <h1>客服页面配置</h1>
        <span>配置前台联系客服页面、导航入口、二维码和常见问题</span>
      </div>
    </div>

    <form class="admin-panel customer-service-admin-form" data-testid="customer-service-form" @submit.prevent="save">
      <div class="panel-title-row">
        <div>
          <p class="eyebrow">Page content</p>
          <h2><MessageCircle :size="20" /> 页面内容</h2>
        </div>
        <button class="primary-button icon-button-text" type="submit" :disabled="saving">
          <Save :size="16" />
          {{ saving ? '保存中...' : '保存配置' }}
        </button>
      </div>

      <div class="customer-service-admin-grid">
        <label>
          <span class="field-label">标题</span>
          <input v-model="form.title" data-testid="contact-title" class="text-input" />
        </label>
        <label>
          <span class="field-label">英文标识</span>
          <input v-model="form.eyebrow" class="text-input" />
        </label>
        <label class="wide">
          <span class="field-label">副标题</span>
          <input v-model="form.subtitle" class="text-input" />
        </label>
        <label class="wide">
          <span class="field-label">说明文案</span>
          <textarea v-model="form.description" class="text-input admin-textarea" rows="3" />
        </label>

        <label>
          <span class="field-label">微信标题</span>
          <input v-model="form.wechat.label" class="text-input" />
        </label>
        <label>
          <span class="field-label">微信号</span>
          <input v-model="form.wechat.account" data-testid="contact-wechat-account" class="text-input" />
        </label>
        <label class="wide">
          <span class="field-label">微信二维码图片 URL</span>
          <div class="admin-qr-upload-row">
            <input v-model="form.wechat.qr_url" data-testid="contact-wechat-qr-url" class="text-input" />
            <label class="secondary-button icon-button-text admin-file-upload-button">
              <Upload :size="16" />
              {{ uploadingChannel === 'wechat' ? '上传中...' : '上传图片' }}
              <input
                data-testid="contact-wechat-qr-upload"
                type="file"
                accept="image/png,image/jpeg,image/webp"
                :disabled="uploadingChannel !== ''"
                @change="uploadQRCode('wechat', $event)"
              />
            </label>
          </div>
          <img v-if="form.wechat.qr_url" class="admin-qr-preview" :src="form.wechat.qr_url" alt="微信二维码预览" />
        </label>

        <label>
          <span class="field-label">QQ 标题</span>
          <input v-model="form.qq.label" class="text-input" />
        </label>
        <label>
          <span class="field-label">QQ 号</span>
          <input v-model="form.qq.account" class="text-input" />
        </label>
        <label class="wide">
          <span class="field-label">QQ 二维码图片 URL</span>
          <div class="admin-qr-upload-row">
            <input v-model="form.qq.qr_url" data-testid="contact-qq-qr-url" class="text-input" />
            <label class="secondary-button icon-button-text admin-file-upload-button">
              <Upload :size="16" />
              {{ uploadingChannel === 'qq' ? '上传中...' : '上传图片' }}
              <input
                data-testid="contact-qq-qr-upload"
                type="file"
                accept="image/png,image/jpeg,image/webp"
                :disabled="uploadingChannel !== ''"
                @change="uploadQRCode('qq', $event)"
              />
            </label>
          </div>
          <img v-if="form.qq.qr_url" class="admin-qr-preview" :src="form.qq.qr_url" alt="QQ 二维码预览" />
        </label>

        <label>
          <span class="field-label">服务标签（一行一个）</span>
          <textarea v-model="serviceTagsText" data-testid="contact-service-tags" class="text-input admin-textarea" rows="5" />
        </label>
        <label>
          <span class="field-label">服务信息（标签|内容）</span>
          <textarea v-model="statsText" class="text-input admin-textarea" rows="5" />
        </label>
        <label>
          <span class="field-label">能力卡片（标题|说明）</span>
          <textarea v-model="featuresText" class="text-input admin-textarea" rows="6" />
        </label>
        <label>
          <span class="field-label">常见问题（标题|链接）</span>
          <textarea v-model="faqsText" data-testid="contact-faqs" class="text-input admin-textarea" rows="6" />
        </label>
      </div>
    </form>

    <p v-if="loading" class="page-status">加载中...</p>
    <p v-if="message" class="status-success">{{ message }}</p>
    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
  </section>
</template>

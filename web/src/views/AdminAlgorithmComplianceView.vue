<script setup>
import { onMounted, reactive, ref } from 'vue'
import { Download, Save } from 'lucide-vue-next'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const form = reactive({
  algorithm_name: '',
  algorithm_type: '',
  service_description: '',
  provider_description: '',
  governance_summary: '',
  marking_summary: '',
  user_rights_summary: '',
  disclosure_version: '',
  status: 'draft'
})
const loading = ref(false)
const saving = ref(false)
const message = ref('')
const errorMessage = ref('')
const statusOptions = [
  { value: 'draft', label: '草稿' },
  { value: 'published', label: '已发布' }
]

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    Object.assign(form, await api.getAlgorithmDisclosure())
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
    Object.assign(form, await api.updateAlgorithmDisclosure(form))
    message.value = '算法公示已保存'
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    saving.value = false
  }
}

function exportEvidence() {
  window.open(api.algorithmComplianceExportURL(), '_blank', 'noopener')
}

onMounted(load)
</script>

<template>
  <section class="admin-compliance-page">
    <div class="admin-page-heading">
      <div>
        <p>ALGORITHM</p>
        <h1>算法公示管理</h1>
        <span>维护拟公示内容，并导出自评估佐证包。</span>
      </div>
      <button class="primary-button icon-button-text" type="button" @click="exportEvidence">
        <Download :size="17" />
        导出佐证
      </button>
    </div>

    <form class="admin-panel compliance-editor" @submit.prevent="save">
      <p v-if="loading" class="page-status">正在读取算法公示...</p>
      <label>
        <span>算法名称</span>
        <input v-model="form.algorithm_name" required />
      </label>
      <label>
        <span>算法类型</span>
        <input v-model="form.algorithm_type" required />
      </label>
      <label>
        <span>版本</span>
        <input v-model="form.disclosure_version" />
      </label>
      <label>
        <span>状态</span>
        <ClickSelect v-model="form.status" :options="statusOptions" aria-label="状态" />
      </label>
      <label>
        <span>服务说明</span>
        <textarea v-model="form.service_description" rows="3"></textarea>
      </label>
      <label>
        <span>模型来源</span>
        <textarea v-model="form.provider_description" rows="3"></textarea>
      </label>
      <label>
        <span>治理策略</span>
        <textarea v-model="form.governance_summary" rows="4"></textarea>
      </label>
      <label>
        <span>结果标识</span>
        <textarea v-model="form.marking_summary" rows="3"></textarea>
      </label>
      <label>
        <span>用户权益</span>
        <textarea v-model="form.user_rights_summary" rows="3"></textarea>
      </label>
      <button class="primary-button icon-button-text" type="submit" :disabled="saving">
        <Save :size="17" />
        {{ saving ? '保存中' : '保存公示' }}
      </button>
      <p v-if="message" class="status-success">{{ message }}</p>
      <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
    </form>
  </section>
</template>

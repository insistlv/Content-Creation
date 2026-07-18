<script setup>
import { onMounted, reactive, ref } from 'vue'
import { Plus, RefreshCw } from 'lucide-vue-next'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const items = ref([])
const loading = ref(false)
const saving = ref(false)
const errorMessage = ref('')
const message = ref('')
const form = reactive({
  title: '',
  severity: 'medium',
  status: 'open',
  description: '',
  action: '',
  owner: ''
})
const severityOptions = [
  { value: 'low', label: '低' },
  { value: 'medium', label: '中' },
  { value: 'high', label: '高' }
]
const statusOptions = [
  { value: 'open', label: '待处理' },
  { value: 'mitigating', label: '处理中' },
  { value: 'resolved', label: '已解决' }
]

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await api.listAlgorithmIncidents({ page: 1, page_size: 50 })
    items.value = data.items ?? []
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

async function createIncident() {
  saving.value = true
  message.value = ''
  errorMessage.value = ''
  try {
    await api.createAlgorithmIncident(form)
    Object.assign(form, { title: '', severity: 'medium', status: 'open', description: '', action: '', owner: '' })
    message.value = '事件已记录'
    await load()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="admin-compliance-page">
    <div class="admin-page-heading">
      <div>
        <p>INCIDENTS</p>
        <h1>应急事件记录</h1>
        <span>记录算法、内容安全和模型供应商异常事件。</span>
      </div>
      <button class="primary-button icon-button-text" type="button" @click="load">
        <RefreshCw :size="17" />
        刷新
      </button>
    </div>

    <form class="admin-panel compliance-editor" @submit.prevent="createIncident">
      <label>
        <span>标题</span>
        <input v-model="form.title" required placeholder="例如：公开分享违规扩散处置" />
      </label>
      <label>
        <span>严重程度</span>
        <ClickSelect v-model="form.severity" :options="severityOptions" aria-label="严重程度" />
      </label>
      <label>
        <span>状态</span>
        <ClickSelect v-model="form.status" :options="statusOptions" aria-label="状态" />
      </label>
      <label>
        <span>负责人</span>
        <input v-model="form.owner" placeholder="姓名或岗位" />
      </label>
      <label>
        <span>事件说明</span>
        <textarea v-model="form.description" rows="3"></textarea>
      </label>
      <label>
        <span>处置动作</span>
        <textarea v-model="form.action" rows="3"></textarea>
      </label>
      <button class="primary-button icon-button-text" type="submit" :disabled="saving">
        <Plus :size="17" />
        {{ saving ? '记录中' : '记录事件' }}
      </button>
      <p v-if="message" class="status-success">{{ message }}</p>
      <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
    </form>

    <div class="admin-panel">
      <div class="admin-table-scroll">
        <table class="admin-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>标题</th>
              <th>严重程度</th>
              <th>状态</th>
              <th>负责人</th>
              <th>处置</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in items" :key="item.id">
              <td>#{{ item.id }}</td>
              <td>{{ item.title }}</td>
              <td>{{ item.severity }}</td>
              <td>{{ item.status }}</td>
              <td>{{ item.owner || '-' }}</td>
              <td>{{ item.action || '-' }}</td>
            </tr>
            <tr v-if="!loading && items.length === 0">
              <td colspan="6">暂无事件</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { RefreshCw, Search } from 'lucide-vue-next'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const items = ref([])
const total = ref(0)
const loading = ref(false)
const errorMessage = ref('')
const actionMessage = ref('')
const filters = reactive({ status: 'pending', q: '' })
const statusOptions = [
  { value: 'pending', label: '待审核' },
  { value: 'manual_review', label: '人工复核' },
  { value: 'pass', label: '已通过' },
  { value: 'reject', label: '已拒绝' },
  { value: 'all', label: '全部' }
]

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await api.listContentReviews({ ...filters, page: 1, page_size: 50 })
    items.value = data.items ?? []
    total.value = data.total ?? 0
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

async function decide(item, status) {
  actionMessage.value = ''
  try {
    await api.updateContentReview(item.id, {
      status,
      action: status === 'reject' ? '限制传播并保留证据' : '审核通过',
      comment: status === 'reject' ? '后台人工处置' : '后台人工放行'
    })
    actionMessage.value = '处置已保存'
    await load()
  } catch (error) {
    errorMessage.value = error.message
  }
}

onMounted(load)
</script>

<template>
  <section class="admin-compliance-page">
    <div class="admin-page-heading">
      <div>
        <p>CONTENT SAFETY</p>
        <h1>内容审核台</h1>
        <span>生成前、参考图、结果后审和公开分享复核记录。</span>
      </div>
      <button class="primary-button icon-button-text" type="button" @click="load">
        <RefreshCw :size="17" />
        刷新
      </button>
    </div>

    <div class="admin-panel compliance-toolbar">
      <label>
        <span>状态</span>
        <ClickSelect v-model="filters.status" :options="statusOptions" aria-label="状态" @change="load" />
      </label>
      <label>
        <span>关键词</span>
        <input v-model="filters.q" placeholder="原因 / 输入摘要 / request id" @keyup.enter="load" />
      </label>
      <button class="primary-button compact-button" type="button" @click="load">
        <Search :size="16" />
        筛选
      </button>
    </div>

    <p v-if="loading" class="page-status">正在读取审核记录...</p>
    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
    <p v-if="actionMessage" class="status-success">{{ actionMessage }}</p>

    <div class="admin-panel">
      <div class="admin-table-scroll">
        <table class="admin-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>类型</th>
              <th>状态</th>
              <th>风险</th>
              <th>追溯</th>
              <th>原因</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in items" :key="item.id">
              <td>#{{ item.id }}</td>
              <td>{{ item.review_type }}</td>
              <td>{{ item.status }}</td>
              <td>{{ item.risk_level || '-' }}</td>
              <td>
                <strong v-if="item.generation_record_id">生成 #{{ item.generation_record_id }}</strong>
                <small>{{ item.provider_request_id || item.model || '-' }}</small>
              </td>
              <td>{{ item.reason || item.input_summary || '-' }}</td>
              <td class="compliance-actions">
                <button type="button" @click="decide(item, 'pass')">通过</button>
                <button type="button" @click="decide(item, 'reject')">拒绝</button>
              </td>
            </tr>
            <tr v-if="!loading && items.length === 0">
              <td colspan="7">暂无记录</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="compliance-total">共 {{ total }} 条</p>
    </div>
  </section>
</template>

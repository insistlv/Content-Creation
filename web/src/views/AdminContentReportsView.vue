<script setup>
import { onMounted, reactive, ref } from 'vue'
import { RefreshCw, Search } from 'lucide-vue-next'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const items = ref([])
const total = ref(0)
const loading = ref(false)
const errorMessage = ref('')
const filters = reactive({ status: 'pending', q: '' })
const statusOptions = [
  { value: 'pending', label: '待处理' },
  { value: 'resolved', label: '已处理' },
  { value: 'rejected', label: '不成立' },
  { value: 'all', label: '全部' }
]

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await api.listContentReports({ ...filters, page: 1, page_size: 50 })
    items.value = data.items ?? []
    total.value = data.total ?? 0
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="admin-compliance-page">
    <div class="admin-page-heading">
      <div>
        <p>REPORTS</p>
        <h1>投诉举报台</h1>
        <span>用户提交的内容、算法和权益投诉处理记录。</span>
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
        <input v-model="filters.q" placeholder="原因 / 说明 / 联系方式" @keyup.enter="load" />
      </label>
      <button class="primary-button compact-button" type="button" @click="load">
        <Search :size="16" />
        筛选
      </button>
    </div>

    <p v-if="loading" class="page-status">正在读取举报记录...</p>
    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>

    <div class="admin-panel">
      <div class="admin-table-scroll">
        <table class="admin-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>对象</th>
              <th>状态</th>
              <th>原因</th>
              <th>联系方式</th>
              <th>处置</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in items" :key="item.id">
              <td>#{{ item.id }}</td>
              <td>
                <strong>{{ item.target_type }} #{{ item.target_id }}</strong>
                <small v-if="item.generation_record_id">生成 #{{ item.generation_record_id }}</small>
              </td>
              <td>{{ item.status }}</td>
              <td>{{ item.reason || item.description || '-' }}</td>
              <td>{{ item.contact || '-' }}</td>
              <td>{{ item.resolution || '-' }}</td>
            </tr>
            <tr v-if="!loading && items.length === 0">
              <td colspan="6">暂无记录</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="compliance-total">共 {{ total }} 条</p>
    </div>
  </section>
</template>

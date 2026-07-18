<script setup>
import { reactive, ref } from 'vue'
import { Send } from 'lucide-vue-next'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const form = reactive({
  target_type: 'generation',
  target_id: '',
  generation_record_id: '',
  reason: '',
  description: '',
  contact: ''
})
const submitting = ref(false)
const message = ref('')
const errorMessage = ref('')
const targetTypeOptions = [
  { value: 'generation', label: '生成记录' },
  { value: 'work', label: '作品' },
  { value: 'share', label: '公开分享' }
]

async function submit() {
  submitting.value = true
  message.value = ''
  errorMessage.value = ''
  try {
    const payload = {
      target_type: form.target_type,
      target_id: Number(form.target_id),
      generation_record_id: Number(form.generation_record_id || form.target_id),
      reason: form.reason.trim(),
      description: form.description.trim(),
      contact: form.contact.trim()
    }
    const data = await api.createContentReport(payload)
    message.value = `举报已提交，编号 ${data.id}`
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="legal-page">
    <section class="legal-hero">
      <p>内容与算法投诉</p>
      <h1>提交举报</h1>
      <span>用于内容违规、算法服务和权益相关投诉</span>
    </section>
    <form class="legal-content compliance-form" @submit.prevent="submit">
      <label>
        <span>举报对象类型</span>
        <ClickSelect v-model="form.target_type" :options="targetTypeOptions" aria-label="举报对象类型" />
      </label>
      <label>
        <span>对象 ID</span>
        <input v-model="form.target_id" required inputmode="numeric" placeholder="生成记录或作品 ID" />
      </label>
      <label>
        <span>生成记录 ID（可选）</span>
        <input v-model="form.generation_record_id" inputmode="numeric" placeholder="用于后台追溯" />
      </label>
      <label>
        <span>举报原因</span>
        <input v-model="form.reason" required maxlength="120" placeholder="例如：疑似侵权、虚假信息、未授权肖像" />
      </label>
      <label>
        <span>补充说明</span>
        <textarea v-model="form.description" rows="5" placeholder="请描述问题位置、传播场景或权利主张"></textarea>
      </label>
      <label>
        <span>联系方式</span>
        <input v-model="form.contact" placeholder="手机号或邮箱，便于反馈处理结果" />
      </label>
      <button class="primary-button icon-button-text" type="submit" :disabled="submitting">
        <Send :size="17" />
        {{ submitting ? '提交中' : '提交举报' }}
      </button>
      <p v-if="message" class="status-success">{{ message }}</p>
      <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
    </form>
  </main>
</template>

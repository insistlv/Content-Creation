<script setup>
import { computed, ref } from 'vue'
import { Bot, ChevronDown, ImagePlus, RefreshCw, Send, Trash2, X } from 'lucide-vue-next'

const props = defineProps({
  messages: {
    type: Array,
    default: () => []
  },
  planning: {
    type: Boolean,
    default: false
  },
  referenceItems: {
    type: Array,
    default: () => []
  },
  works: {
    type: Array,
    default: () => []
  },
  referenceUploading: {
    type: Boolean,
    default: false
  },
  requiresAuth: {
    type: Boolean,
    default: false
  },
  failure: {
    type: Object,
    default: null
  }
})

const emit = defineEmits([
  'send-message',
  'upload-reference',
  'remove-reference',
  'clear-references',
  'use-work-reference',
  'require-auth',
  'retry-plan'
])

const input = ref('')
const fileInputRef = ref(null)

const openSections = ref({
  reference: true,
  works: false,
  starters: false
})

function toggleSection(key) {
  openSections.value[key] = !openSections.value[key]
}

const sceneStarters = [
  '商品主图',
  '海报 KV',
  '人像写真',
  '局部改图',
  '扩图',
  '朋友圈营销图'
]

const selectedWorkIds = computed(() => new Set(
  props.referenceItems
    .filter((item) => item?.reference_kind === 'work')
    .map((item) => Number(item.work_id))
    .filter(Boolean)
))

function submitMessage() {
  const message = input.value.trim()
  if (!message || props.planning) return
  emit('send-message', message)
  input.value = ''
}

function sendStarter(label) {
  if (props.planning) return
  emit('send-message', `我想做${label}，请帮我整理创作方案。`)
}

function openUpload() {
  if (props.requiresAuth) {
    emit('require-auth')
    return
  }
  fileInputRef.value?.click()
}

function handleFileChange(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (file) {
    emit('upload-reference', file)
  }
}

function handlePaste(event) {
  if (props.referenceUploading || props.planning) return
  if (props.requiresAuth) {
    emit('require-auth')
    return
  }
  const file = Array.from(event.clipboardData?.files || []).find((item) => item.type?.startsWith('image/'))
  if (file) {
    emit('upload-reference', file)
  }
}

function referenceLabel(item) {
  if (item?.reference_kind === 'work') return '作品库'
  return '上传'
}
</script>

<template>
  <section class="agent-task-input-panel" data-testid="agent-task-input-panel" aria-label="Agent 任务输入" @paste="handlePaste">
    <header class="agent-panel-head">
      <span class="agent-panel-kicker">Task</span>
      <h2>任务输入</h2>
      <p>描述目标并补充参考素材，Agent 会先给出可确认方案。</p>
    </header>

    <form class="agent-chat-form agent-task-form" data-testid="agent-chat-form" @submit.prevent="submitMessage">
      <textarea
        v-model="input"
        data-testid="agent-chat-input"
        placeholder="例如：做一张适合小红书投放的香薰商品主图，浅色背景，突出玻璃质感"
        rows="4"
        maxlength="800"
        :disabled="planning"
        @keydown.enter.exact.prevent="submitMessage"
      />
      <button type="submit" aria-label="发送任务" :disabled="planning || !input.trim()">
        <Send :size="18" />
      </button>
    </form>

    <div v-if="failure" class="agent-failure-card" data-testid="agent-plan-failure" role="alert">
      <strong>{{ failure.reasonTitle }}</strong>
      <p>{{ failure.message }}</p>
      <span>{{ failure.suggestion }}</span>
      <button
        type="button"
        class="secondary-button"
        data-testid="agent-retry-plan"
        :disabled="planning || !failure.retryable"
        @click="emit('retry-plan')"
      >
        <RefreshCw :size="15" />
        {{ failure.retryLabel || '重新生成方案' }}
      </button>
    </div>

    <div class="agent-accordion-section" :class="{ open: openSections.reference }">
      <button
        type="button"
        class="agent-accordion-head"
        data-testid="agent-section-toggle-reference"
        :aria-expanded="openSections.reference"
        @click="toggleSection('reference')"
      >
        <span class="agent-accordion-title">参考素材</span>
        <span v-if="referenceItems.length" class="agent-accordion-count">{{ referenceItems.length }}</span>
        <ChevronDown class="agent-accordion-chevron" :size="16" aria-hidden="true" />
      </button>

      <div v-show="openSections.reference" class="agent-accordion-content agent-reference-manager" aria-label="参考素材">
        <div class="agent-reference-strip">
          <button
            type="button"
            class="agent-upload-button"
            data-testid="agent-upload-reference"
            :disabled="referenceUploading || planning"
            @click="openUpload"
          >
            <ImagePlus :size="16" />
            <span>{{ referenceUploading ? '上传中' : '上传参考图' }}</span>
          </button>
          <span class="agent-paste-hint">也可直接粘贴图片</span>
          <button
            v-if="referenceItems.length"
            type="button"
            class="agent-text-button"
            data-testid="agent-clear-references"
            :disabled="referenceUploading || planning"
            @click="emit('clear-references')"
          >
            <Trash2 :size="14" />
            清空
          </button>
          <input ref="fileInputRef" type="file" accept="image/jpeg,image/png" hidden @change="handleFileChange" />
        </div>

        <div v-if="referenceItems.length" class="agent-reference-list">
          <button
            v-for="item in referenceItems"
            :key="item.id"
            type="button"
            class="agent-reference-chip"
            :data-testid="`agent-reference-item-${item.id}`"
            @click="emit('remove-reference', item)"
          >
            <img v-if="item.preview_url" :src="item.preview_url" alt="" />
            <span class="agent-reference-kind">{{ referenceLabel(item) }}</span>
            <span>{{ item.original_filename || item.prompt || '参考图' }}</span>
            <X :size="13" aria-hidden="true" />
          </button>
        </div>
      </div>
    </div>

    <div class="agent-accordion-section" v-if="works.length" :class="{ open: openSections.works }">
      <button
        type="button"
        class="agent-accordion-head"
        data-testid="agent-section-toggle-works"
        :aria-expanded="openSections.works"
        @click="toggleSection('works')"
      >
        <span class="agent-accordion-title">作品库引用</span>
        <span class="agent-accordion-count">{{ Math.min(works.length, 4) }}</span>
        <ChevronDown class="agent-accordion-chevron" :size="16" aria-hidden="true" />
      </button>

      <div v-show="openSections.works" class="agent-accordion-content agent-work-picks">
        <button
          v-for="work in works.slice(0, 4)"
          :key="work.work_id"
          type="button"
          :class="{ active: selectedWorkIds.has(Number(work.work_id)) }"
          :data-testid="`agent-work-reference-${work.work_id}`"
          @click="emit('use-work-reference', work)"
        >
          <img v-if="work.preview_url" :src="work.preview_url" alt="" />
          <span>{{ work.prompt || `作品 ${work.work_id}` }}</span>
          <small v-if="selectedWorkIds.has(Number(work.work_id))">已引用</small>
        </button>
      </div>
    </div>

    <div class="agent-accordion-section" :class="{ open: openSections.starters }">
      <button
        type="button"
        class="agent-accordion-head"
        data-testid="agent-section-toggle-starters"
        :aria-expanded="openSections.starters"
        @click="toggleSection('starters')"
      >
        <span class="agent-accordion-title">快捷场景</span>
        <ChevronDown class="agent-accordion-chevron" :size="16" aria-hidden="true" />
      </button>

      <div v-show="openSections.starters" class="agent-accordion-content agent-starters" aria-label="快捷场景">
        <button
          v-for="starter in sceneStarters"
          :key="starter"
          type="button"
          :disabled="planning"
          @click="sendStarter(starter)"
        >
          {{ starter }}
        </button>
      </div>
    </div>

    <div class="agent-message-list" data-testid="agent-message-list" aria-label="对话记录">
      <div
        v-for="(message, index) in messages"
        :key="`${message.role}-${index}-${message.content}`"
        class="agent-message"
        :class="`is-${message.role}`"
      >
        <span v-if="message.role === 'assistant'" class="agent-message-avatar" aria-hidden="true">
          <Bot :size="16" />
        </span>
        <p>{{ message.content }}</p>
      </div>
      <div v-if="planning" class="agent-message is-assistant">
        <span class="agent-message-avatar" aria-hidden="true">
          <Bot :size="16" />
        </span>
        <p>正在拆解目标和补齐参数...</p>
      </div>
    </div>
  </section>
</template>

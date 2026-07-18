<script setup>
import { computed, reactive, ref } from 'vue'
import { Image, Plus, Trash2, X } from 'lucide-vue-next'
import ClickSelect from './ClickSelect.vue'

const props = defineProps({
  presets: {
    type: Array,
    default: () => []
  },
  templates: {
    type: Array,
    default: () => []
  },
  selectedPresetId: {
    type: [Number, String],
    default: null
  },
  selectedTemplateId: {
    type: [Number, String],
    default: null
  },
  contentReferenceLimit: {
    type: Number,
    default: 4
  },
  saving: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['select-preset', 'select-template', 'create-template', 'delete-template'])

const activeTab = ref('official')
const dialogOpen = ref(false)
const selectedFile = ref(null)
const localError = ref('')
const form = reactive({
  title: '',
  description: '',
  style_prompt: ''
})

const hasPresets = computed(() => props.presets.length > 0)
const hasTemplates = computed(() => props.templates.length > 0)
const selectedFileName = computed(() => selectedFile.value?.name ?? '')
const selectedPresetIndex = computed(() => {
  const index = props.presets.findIndex((preset) => Number(props.selectedPresetId) === Number(preset?.id))
  return index >= 0 ? index : 0
})
const currentPreset = computed(() => props.presets[selectedPresetIndex.value] ?? null)
const currentPresetPosition = computed(() => (currentPreset.value ? selectedPresetIndex.value + 1 : 0))
const selectedPresetValue = computed(() => currentPreset.value?.id ?? '')
const selectedPresetTitle = computed(() => presetTitle(currentPreset.value))
const presetOptions = computed(() => props.presets.map((preset) => ({
  value: preset?.id,
  label: presetTitle(preset)
})))

function presetTitle(preset) {
  return preset?.title || `风格 ${preset?.id ?? ''}`.trim()
}

function itemTags(item) {
  return Array.isArray(item?.tags) ? item.tags : []
}

function isSelectedPreset(preset) {
  return Number(props.selectedPresetId) === Number(preset?.id)
}

function selectPresetById(presetId) {
  const preset = props.presets.find((item) => Number(item?.id) === Number(presetId))
  if (preset) {
    emit('select-preset', preset)
  }
}

function isSelectedTemplate(template) {
  return Number(props.selectedTemplateId) === Number(template?.id)
}

function openCreateDialog() {
  form.title = ''
  form.description = ''
  form.style_prompt = ''
  selectedFile.value = null
  localError.value = ''
  dialogOpen.value = true
}

function closeCreateDialog() {
  if (props.saving) return
  dialogOpen.value = false
  localError.value = ''
}

function handleTemplateFile(event) {
  selectedFile.value = Array.from(event?.target?.files || [])[0] ?? null
  if (event?.target) {
    event.target.value = ''
  }
}

function submitTemplate() {
  const title = form.title.trim()
  if (!title || !selectedFile.value) {
    localError.value = '请填写模板名称并上传风格参考图'
    return
  }
  localError.value = ''
  emit('create-template', {
    title,
    description: form.description.trim(),
    style_prompt: form.style_prompt.trim(),
    file: selectedFile.value
  })
  dialogOpen.value = false
}
</script>

<template>
  <section class="video-style-library" data-testid="video-style-library">
    <div class="video-style-library-head">
      <div>
        <p class="eyebrow">Style Presets</p>
        <h2>视觉风格</h2>
      </div>
    </div>

    <div class="video-style-tabs" role="tablist" aria-label="视频视觉风格">
      <button
        type="button"
        data-testid="video-style-tab-official"
        :class="['video-style-tab', activeTab === 'official' ? 'active' : '']"
        @click="activeTab = 'official'"
      >
        官方风格 <small>Official</small>
      </button>
      <button
        type="button"
        data-testid="video-style-tab-custom"
        :class="['video-style-tab', activeTab === 'custom' ? 'active' : '']"
        @click="activeTab = 'custom'"
      >
        我的模板
      </button>
    </div>

    <div v-if="activeTab === 'official'" class="video-style-track official-single">
      <label v-if="hasPresets" class="video-style-official-control">
        <span>官方视觉风格</span>
        <ClickSelect
          :model-value="selectedPresetValue"
          :options="presetOptions"
          :trigger-label="selectedPresetTitle"
          data-testid="video-style-preset-select"
          class="video-style-preset-select"
          aria-label="官方视觉风格"
          @change="selectPresetById"
        />
      </label>
      <article
        v-if="currentPreset"
        :key="currentPreset.id"
        :class="['video-style-card', 'video-style-preview-card', isSelectedPreset(currentPreset) ? 'is-selected' : '']"
        :data-testid="`video-style-preset-${currentPreset.id}`"
      >
        <span class="video-style-card-copy">
          <span class="video-style-card-meta">
            <small>{{ currentPreset.category || '通用风格' }}</small>
            <small>{{ currentPresetPosition }}/{{ presets.length }}</small>
          </span>
          <strong>{{ currentPreset.title }}</strong>
          <p>{{ currentPreset.description }}</p>
          <span class="video-style-tags">
            <em v-for="tag in itemTags(currentPreset)" :key="tag">{{ tag }}</em>
          </span>
        </span>
      </article>
      <p v-if="!hasPresets" class="video-style-empty" data-testid="video-style-empty-official">暂无官方风格</p>
    </div>

    <div v-else class="video-style-track">
      <button class="video-style-card video-style-create-card" data-testid="video-style-create" type="button" @click="openCreateDialog">
        <span class="video-style-card-media">
          <span class="video-style-card-placeholder">
            <Plus :size="24" />
          </span>
        </span>
        <span class="video-style-card-copy">
          <strong>创建自定义风格</strong>
          <small>上传一张图作为专属模板</small>
          <p>保存后可在后续视频任务中复用。</p>
        </span>
      </button>
      <article
        v-for="template in templates"
        :key="template.id"
        :class="['video-style-card', isSelectedTemplate(template) ? 'is-selected' : '']"
        :data-testid="`video-style-template-${template.id}`"
      >
        <button class="video-style-card-main" type="button" @click="emit('select-template', template)">
          <span class="video-style-card-media">
            <img v-if="template.preview_url" :src="template.preview_url" :alt="template.title" />
            <span v-else class="video-style-card-placeholder">
              <Image :size="22" />
            </span>
          </span>
          <span class="video-style-card-copy">
            <strong>{{ template.title }}</strong>
            <small>我的模板</small>
            <p>{{ template.description || template.style_prompt || '自定义视觉风格' }}</p>
          </span>
        </button>
        <button
          class="video-style-delete"
          type="button"
          :aria-label="`删除 ${template.title}`"
          @click.stop="emit('delete-template', template)"
        >
          <Trash2 :size="14" />
        </button>
      </article>
      <p v-if="!hasTemplates" class="video-style-empty">暂无自定义模板</p>
    </div>

    <div v-if="dialogOpen" class="video-style-template-backdrop">
      <form class="video-style-template-modal" data-testid="video-style-template-modal" @submit.prevent="submitTemplate">
        <div class="video-style-template-head">
          <div>
            <h3>创建自定义风格</h3>
            <p>上传一张参考图，保存为可复用的视频视觉模板。</p>
          </div>
          <button class="icon-button" type="button" aria-label="关闭" @click="closeCreateDialog">
            <X :size="16" />
          </button>
        </div>

        <label>
          <span>模板名称</span>
          <input v-model="form.title" data-testid="video-style-template-title" required />
        </label>
        <label>
          <span>描述</span>
          <textarea v-model="form.description" data-testid="video-style-template-description" rows="2" />
        </label>
        <label>
          <span>风格提示词</span>
          <textarea v-model="form.style_prompt" data-testid="video-style-template-prompt" rows="2" />
        </label>
        <label class="video-style-template-upload">
          <input data-testid="video-style-template-upload-input" type="file" accept="image/jpeg,image/png" @change="handleTemplateFile" />
          <span>{{ selectedFileName || '上传风格参考图' }}</span>
        </label>
        <p v-if="localError" class="status-error">{{ localError }}</p>

        <div class="video-style-template-actions">
          <button class="secondary-button" type="button" @click="closeCreateDialog">取消</button>
          <button class="primary-button" data-testid="video-style-template-save" type="button" :disabled="saving" @click="submitTemplate">
            {{ saving ? '保存中...' : '保存模板' }}
          </button>
        </div>
      </form>
    </div>
  </section>
</template>

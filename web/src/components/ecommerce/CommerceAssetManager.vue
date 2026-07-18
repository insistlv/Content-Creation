<script setup>
import { computed, ref, watch } from 'vue'
import ImageUploadZone from '../ImageUploadZone.vue'
const props = defineProps({ assets: { type: Array, default: () => [] }, pipeline: { type: String, default: 'general' } })
const emit = defineEmits(['upload', 'delete'])
const shared = ['logo', 'pattern', 'scene_reference', 'style_reference']
const product = ['product_front', 'product_back', 'product_detail', 'replacement']
const fashion = ['garment_front', 'garment_back', 'garment_detail', 'model_reference', 'pose_reference']
const roles = computed(() => props.pipeline === 'mixed' ? [...product, ...shared, ...fashion] : props.pipeline === 'fashion' ? [...fashion, ...shared] : [...product, ...shared])
const role = ref(roles.value[0])
const lifecycle = ref('project')
watch(roles, (next) => { if (!next.includes(role.value)) role.value = next[0] })
function upload(file) { emit('upload', file, { role: role.value, lifecycle: lifecycle.value }) }
</script>
<template><section class="commerce-card asset-manager"><h2>分类素材</h2><label>素材角色<select v-model="role"><option v-for="item in roles" :key="item" :value="item">{{ item }}</option></select></label><label>保留策略<select v-model="lifecycle"><option value="project">随项目保留</option><option value="temporary">临时保留</option></select></label><ImageUploadZone :max-images="20" @upload="upload"/><p class="privacy">素材按项目私有保存；project 随项目保留，temporary 到 retain_until 后清理。请勿上传无授权的人像或商标。</p><div class="asset-grid"><article v-for="asset in assets" :key="asset.id"><img :src="asset.preview_url" :alt="asset.original_filename || asset.role"><span>{{ asset.role }}</span><small>{{ asset.lifecycle }}<template v-if="asset.retain_until"> · {{ asset.retain_until.slice(0, 10) }}</template></small><button type="button" :data-testid="`commerce-asset-delete-${asset.id}`" @click="emit('delete', asset)">删除</button></article></div></section></template>

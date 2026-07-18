<script setup>
defineProps({ project: { type: Object, default: null } })
const steps = ['商品建档', '素材上传', '商品报告状态', '生产计划']
</script>
<template>
  <section class="commerce-card setup"><h2>四步建档</h2><ol><li v-for="(step, index) in steps" :key="step" data-testid="commerce-setup-step"><b>{{ index + 1 }}</b><span>{{ step }}</span></li></ol><p>商品报告：<strong>{{ project?.report_status === 'pending_input' ? '待补录' : '待分析' }}</strong></p></section>
</template>

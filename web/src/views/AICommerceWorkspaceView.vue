<script setup>
import { onMounted } from 'vue'
import CommerceCreatorShell from '../components/ecommerce/CommerceCreatorShell.vue'
import { useCommerceDetailWorkflow } from '../composables/useCommerceDetailWorkflow.js'
const workflow = useCommerceDetailWorkflow()
onMounted(workflow.initialize)
</script>
<template><CommerceCreatorShell :workflow="workflow"/></template>

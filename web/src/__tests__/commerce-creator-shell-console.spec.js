import { describe, expect, it } from 'vitest'

import shellSource from '../components/ecommerce/CommerceCreatorShell.vue?raw'

describe('CommerceCreatorShell 生产控制台接线', () => {
  it('保留左侧三步并把现有工作流数据传给右侧控制台', () => {
    expect(shellSource).toContain('<CommerceProductInput')
    expect(shellSource).toContain('<CommerceReportEditor')
    expect(shellSource).toContain('<CommerceGenerationConfigurator')
    for (const binding of [
      ':events="w.batches.events.value"',
      ':assets="w.assets.value"',
      ':creative-spec="w.creativeSpec.value"',
      ':selected-sections="w.selectedSections.value"',
      ':aspect-ratio="w.aspectRatio.value"',
      ':quality-tier="w.qualityTier.value"',
      ':layout-template="w.layoutTemplate.value"',
      ':estimate="w.estimateResult.value"',
      ':current-project="w.currentProject.value"',
    ]) expect(shellSource).toContain(binding)
  })
})

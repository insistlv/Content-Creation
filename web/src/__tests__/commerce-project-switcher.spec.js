import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import CommerceProjectSwitcher from '../components/ecommerce/CommerceProjectSwitcher.vue'

describe('CommerceProjectSwitcher', () => {
  it('按最近创作、项目下拉框和新建创作的顺序渲染工具栏', () => {
    const wrapper = mount(CommerceProjectSwitcher, {
      props: {
        projects: [{ id: 7, title: '便携式水杯' }],
        currentProject: { id: 7, title: '便携式水杯' },
      },
    })

    expect(wrapper.findAll('[data-testid]').map((item) => item.attributes('data-testid'))).toEqual([
      'commerce-project-label',
      'commerce-project-select',
      'commerce-project-new',
    ])
    expect(wrapper.get('[data-testid="commerce-project-label"]').text()).toBe('最近创作')
    expect(wrapper.get('label').attributes('for')).toBe('commerce-project-select')
    expect(wrapper.get('select').attributes('id')).toBe('commerce-project-select')
  })
})

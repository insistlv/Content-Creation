import { DOMWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'

const originalSetValue = DOMWrapper.prototype.setValue

DOMWrapper.prototype.setValue = async function setValue(value) {
  const element = this.element
  if (element?.classList?.contains('click-select-trigger')) {
    const testid = element.getAttribute('data-testid')
    if (testid) {
      let option = document.body.querySelector(`[data-testid="${testid}-option-${String(value)}"]`)
      if (!option) {
        element.click()
        await nextTick()
        option = document.body.querySelector(`[data-testid="${testid}-option-${String(value)}"]`)
      }
      if (option) {
        option.click()
        await nextTick()
        return
      }
    }
  }

  return originalSetValue.call(this, value)
}

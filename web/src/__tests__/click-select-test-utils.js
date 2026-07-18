import { nextTick } from 'vue'

export async function openClickSelect(wrapper, testid) {
  await wrapper.get(`[data-testid="${testid}"]`).trigger('click')
  await nextTick()
}

export async function chooseClickSelect(wrapper, testid, value) {
  await openClickSelect(wrapper, testid)
  const option = document.body.querySelector(`[data-testid="${testid}-option-${String(value)}"]`)
  if (!option) {
    throw new Error(`Missing ClickSelect option ${testid}-option-${String(value)}`)
  }
  option.click()
  await nextTick()
}

export function clickSelectMenu(testid) {
  return document.body.querySelector(`[data-testid="${testid}-menu"]`)
}

export function clickSelectOption(testid, value) {
  return document.body.querySelector(`[data-testid="${testid}-option-${String(value)}"]`)
}

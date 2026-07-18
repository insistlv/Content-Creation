export function displayLabel(options, value, fallback) {
  const items = Array.isArray(options) ? options : []
  return items.find(item => item?.value === value && item?.label)?.label || fallback
}

const fieldLabels = {
  name: '商品名称',
  category: '品类',
  material: '材质',
  capacity: '容量',
  price: '价格',
  certification: '认证',
  efficacy: '功效',
  brand_tone: '品牌调性',
  color: '颜色'
}

export function commerceFieldLabel(field) {
  return fieldLabels[field] || '其他商品信息'
}

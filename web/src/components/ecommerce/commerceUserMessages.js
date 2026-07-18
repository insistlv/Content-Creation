const messages = {
  commerce_disabled: 'AI 电商未开启',
  worker_unavailable: '生成服务暂不可用',
  version_conflict: '报告已更新，请刷新后重试',
  pricing_stale: '估价已过期，请重新估价',
  pricing_snapshot_expired: '估价已过期，请重新估价',
  estimate_expired: '估价已过期，请重新估价',
  insufficient_credits: '点数余额不足，请充值后重试',
  provider_policy_rejected: '内容未通过安全审核，请调整后重试'
}

export function commerceUserMessage(errorOrCode, fallback = '操作失败，请稍后重试') {
  const code = typeof errorOrCode === 'string' ? errorOrCode : errorOrCode?.code
  if (messages[code]) return messages[code]
  const message = typeof errorOrCode === 'string' ? errorOrCode : errorOrCode?.message
  const localChineseMessage = typeof message === 'string'
    && /[\u3400-\u9fff]/u.test(message)
    && /^[\u3400-\u9fff\d\s，。！？；：、,.!?;:（）()【】\[\]《》“”‘’—…·%+\-]+$/u.test(message)
  return localChineseMessage ? message : fallback
}

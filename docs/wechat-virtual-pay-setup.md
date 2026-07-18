# 微信小程序支付配置

微信应用密钥、商户私钥、API v3 密钥和虚拟支付 AppKey 必须存入加密密钥表。不要把多行私钥写入 `.env`，也不要在仓库或镜像中保存证书私钥。

## 密钥设置

在后台“系统设置”中按实际功能配置：

- `WECHAT_PAY_APP_ID`
- `WECHAT_APP_SECRET`
- `WECHAT_PAY_MCH_ID`
- `WECHAT_PAY_MCH_CERT_SERIAL_NO`
- `WECHAT_PAY_MCH_PRIVATE_KEY`
- `WECHAT_PAY_API_V3_KEY`
- `WECHAT_PAY_PLATFORM_PUBLIC_KEY`
- `WECHAT_VIRTUAL_PAY_OFFER_ID`
- `WECHAT_VIRTUAL_PAY_APP_KEY`
- `WECHAT_VIRTUAL_PAY_SANDBOX_APP_KEY`（仅非生产环境）

非敏感参数仍可通过环境变量配置：

```env
WECHAT_PAY_NOTIFY_URL=https://example.com/api/payments/wechat/notify
WECHAT_VIRTUAL_PAY_ENV=0
```

旧环境可在受控终端临时提供密钥并执行 `go run ./cmd/secrets import-env`。导入成功后立即删除旧环境中的原值。

## 验收

1. 小程序通过 `uni.login` 获取临时 code。
2. 后端创建虚拟支付订单并返回签名参数。
3. 小程序调用 `wx.requestVirtualPayment`。
4. 后端查询微信订单状态，只在金额和状态匹配时发放点数。
5. 重复确认或重复通知不得重复发放点数。

生产环境应使用 `WECHAT_VIRTUAL_PAY_ENV=0`；沙箱测试应使用独立的非生产环境。

# 支付宝支付配置

支付宝应用标识、应用私钥和支付宝公钥属于业务密钥，必须通过后台“系统设置”或一次性导入命令写入加密密钥表，不得长期保存在 `.env`、systemd 环境、镜像或仓库中。

## 配置项

后台密钥设置中配置：

- `ALIPAY_APP_ID`
- `ALIPAY_PRIVATE_KEY`
- `ALIPAY_PUBLIC_KEY`

非敏感运行参数可继续通过环境变量提供：

```env
APP_BASE_URL=https://example.com
ALIPAY_SANDBOX=0
ALIPAY_GATEWAY=
```

`ALIPAY_GATEWAY` 留空时，后端按 `ALIPAY_SANDBOX` 选择默认网关。生产环境必须关闭沙箱，并确保 `APP_BASE_URL` 是用户实际访问的 HTTPS 地址。

如从旧部署迁移，可在受控终端临时设置上述三个密钥，然后执行：

```sh
go run ./cmd/secrets import-env
```

确认后台显示“已配置”后立即从终端会话和旧环境文件中删除原值。接口、审计和日志只显示配置状态，不返回密钥原文。

## 联调检查

1. 确认异步通知地址 `APP_BASE_URL/api/payments/alipay/notify` 可访问。
2. 创建测试订单并进入收银台。
3. 支付后确认订单由 `pending` 变为 `paid`，且点数只发放一次。
4. 异步通知延迟时，通过订单查询接口刷新状态。

若历史仓库、日志或共享文件中出现过应用私钥，必须先在支付宝侧作废并重新签发，不能仅从文件中删除。

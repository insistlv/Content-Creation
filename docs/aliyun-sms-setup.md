# 阿里云短信配置

建议使用最小权限的 RAM 子账号，并在阿里云控制台完成短信签名和验证码模板审核。AccessKey 属于业务密钥，必须存入应用的加密密钥表。

## 密钥设置

在后台“系统设置”中配置：

- `ALIYUN_SMS_ACCESS_KEY_ID`
- `ALIYUN_SMS_ACCESS_KEY_SECRET`
- `ALIYUN_SMS_SIGN_NAME`
- `ALIYUN_SMS_REGISTER_TEMPLATE_CODE`
- `ALIYUN_SMS_RESET_TEMPLATE_CODE`

非敏感参数可通过环境变量设置：

```env
SMS_PROVIDER=aliyun
ALIYUN_SMS_ENDPOINT=dysmsapi.aliyuncs.com
```

如需迁移旧部署，可在受控终端临时设置原配置并执行：

```sh
go run ./cmd/secrets import-env
```

导入后确认后台只显示“已配置”，再删除旧 `.env` 或进程环境中的密钥。不要在日志、工单或截图中复制 AccessKey。

## 验收

1. 注册验证码与重置密码验证码均可发送。
2. 验证码频率限制和过期时间按预期生效。
3. 错误响应和应用日志不包含 AccessKey 原文。
4. 若密钥曾进入提交、日志或共享文件，应立即在阿里云侧禁用并重新创建。

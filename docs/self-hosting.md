# 通用自托管

## 根信任

应用启动只需要：

- `DATABASE_URL` 或 `DATABASE_URL_FILE`
- `APP_SECRETS_MASTER_KEY` 或 `APP_SECRETS_MASTER_KEY_FILE`，内容为恰好 32 字节随机值的标准 Base64

生产优先使用只读挂载文件或密钥管理器。主密钥不得写入镜像、数据库、公开 CI 或源码仓库。

## 首次启动

1. 创建空 PostgreSQL 数据库。
2. 生成主密钥，例如 `openssl rand -base64 32`。
3. 使用 `STARTUP_DATABASE_MIGRATIONS=bootstrap` 启动一次。
4. 执行 `go run ./cmd/admin create --username <name>`，交互输入不少于 12 位的密码。
5. 将迁移模式改为 `existing` 或按你的迁移流程管理。

JWT 密钥会在首次启动时生成并加密保存。模型 Key 为空不会阻止服务启动。

## 导入业务密钥

一次性命令支持以下变量及其 `_FILE` 形式：

`OPENAI_API_KEY`、`DEEPSEEK_API_KEY`、`OSS_ACCESS_KEY_ID`、`OSS_ACCESS_KEY_SECRET`、`AI_COMMERCE_OSS_ACCESS_KEY_ID`、`AI_COMMERCE_OSS_ACCESS_KEY_SECRET`、`ALIYUN_SMS_*`、`ALIPAY_*`、`WECHAT_*`、`ARK_API_KEY`、`ZZ_API_KEY`。

```bash
go run ./cmd/secrets import-env --actor operator-name
```

导入成功后，从服务环境和临时文件中删除业务密钥。现有 `model_configs.api_key` 与 `model_providers.api_key` 会在安全启动迁移时加密导入并清空旧值。

## 轮换主密钥

先备份数据库，再通过 `APP_SECRETS_NEW_MASTER_KEY_FILE` 提供新密钥：

```bash
go run ./cmd/secrets rotate-key --actor operator-name
```

命令在事务中重新加密全部记录。成功后更新 `APP_SECRETS_MASTER_KEY_FILE` 与 `APP_SECRETS_KEY_VERSION`，重启并验证，再销毁旧主密钥。

## 管理接口

- `GET /api/admin/secret-settings` 返回 `configured`、`updated_at` 和 `key_version`。
- `PATCH /api/admin/secret-settings` 接收 `{"items":[{"name":"OPENAI_API_KEY","value":"..."}]}`。空 `value` 保持不变；只有 `clear:true` 会删除。

响应、审计和日志不会返回原值。修改运行密钥后需要滚动重启以重建客户端。

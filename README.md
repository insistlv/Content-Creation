# image-agent

image-agent 是一个基于 Go、Gin、GORM、Vue 3 和 uni-app 的 AI 内容创作平台，包含 Web、管理端、移动 H5/微信小程序以及可选的模型、对象存储、短信和支付适配。

## 安全默认值

- 应用只从环境或挂载文件读取 `DATABASE_URL(_FILE)` 与 `APP_SECRETS_MASTER_KEY(_FILE)` 两个根信任。
- 业务凭据使用 AES-256-GCM 加密后保存在 `secret_records`，随机 nonce，并使用 `namespace/owner_id/name` 作为 AAD。
- 没有模型 API Key 时服务仍可启动；相关功能显示为未配置。
- JWT 密钥首次启动时自动生成并加密保存。首个管理员通过交互式命令创建，不使用启动密码环境变量。
- 管理接口只返回密钥是否已配置、更新时间和密钥版本，从不返回原值。

## 快速开始

需要 Go 1.26、Node.js 24 和 PostgreSQL 15+。

```bash
cp .env.example .env
# 生成 32 字节随机值并进行 Base64 编码，写入 APP_SECRETS_MASTER_KEY。
cd web && npm ci && npm run build && cd ..
go run ./cmd/server
```

首次启动空库时使用 `STARTUP_DATABASE_MIGRATIONS=bootstrap`。随后创建管理员：

```bash
go run ./cmd/admin create --username admin
```

需要第三方服务时，临时把对应值放入进程环境或受保护的 `*_FILE`，执行一次导入，然后从环境中删除：

```bash
go run ./cmd/secrets import-env
```

详细说明见 [自托管文档](docs/self-hosting.md)。Docker 用户可以从 [compose.yaml](compose.yaml) 开始。

## 开发与验证

```bash
go test ./internal/app
go build ./cmd/server ./cmd/admin ./cmd/secrets
cd web && npm test && npm run build
```

CI 只执行测试、构建、许可证、依赖、安全与泄密检查，不包含生产部署。

## 安全报告

请不要在公开 Issue 中披露漏洞或凭据。报告方式和响应目标见 [SECURITY.md](SECURITY.md)。任何曾进入提交、日志、截图或共享文件的凭据都应先在供应商侧作废并重新签发。

## 许可证与源码提供

Copyright © image-agent contributors。

本项目按 [GNU Affero General Public License v3.0](LICENSE) 发布。通过网络运行修改版本时，运营者必须按 AGPL-3.0 第 13 条向交互用户提供对应源码。构建部署时请把 `VITE_SOURCE_CODE_URL` 设置为该运行版本的公开源码地址。

素材与第三方声明见 [ASSET_LICENSES.md](ASSET_LICENSES.md) 和 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。贡献前请阅读 [CONTRIBUTING.md](CONTRIBUTING.md) 与 [DCO](DCO)。

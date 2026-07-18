# 公开发布流程

当前私有仓库历史包含已泄露的旧凭据，因此不得把现有 `.git`、标签、远程引用或 checkpoint refs 推送到公开托管平台。

## 不可跳过的门槛

- 在供应商侧作废旧支付宝私钥，并轮换所有无法证明从未泄露的凭据。
- 备份数据库，执行 `secrets import-env`，确认旧模型 API Key 列已清空。
- 运行 Go、Web、小程序、许可证、依赖与公开快照检查。
- 使用 Gitleaks 扫描工作树；对导出的新仓库扫描工作树和完整历史，零未解释发现。
- 人工确认 `ASSET_LICENSES.md`，并确认代码版权归贡献者或已取得书面授权。

## 导出单次快照

私有仓完成提交且工作树干净后：

```bash
scripts/export-public-snapshot.sh ../image-agent-public
cd ../image-agent-public
gitleaks dir . --redact --no-banner
gitleaks git . --redact --no-banner
```

脚本只导出明确白名单，不复制 `.git`、旧标签、部署文件、缓存或本地工具目录，并创建一个签署的中文初始提交。

## GitHub 暂存与公开

1. 新建私有 GitHub 仓库，推送快照的 `main`。
2. 启用 Push Protection、Secret Scanning、Private Vulnerability Reporting、Dependabot 和 CodeQL。
3. 为 `main` 创建 ruleset：禁止强推和删除；要求 PR、至少一次审核、分支最新、CI 与 CodeQL 通过；限制绕过权限。
4. 完成人工安全与版权复核后再切换为 Public。
5. 设置 `VITE_SOURCE_CODE_URL` 为公开仓地址，构建并确认 Web/小程序能看到源码入口。
6. 使用维护者签名的 tag 发布首版；生产上线是独立授权动作，不由本流程自动执行。

现有私有 Gitee 仓仅作归档和内部开发，不镜像历史到公开仓。

# IMAGE AGENT Mobile H5

独立移动端项目，基于 uni-app + Vue 3 + Vite。目录与现有 `web/` 分离，后续可继续扩展 H5、APP 和小程序端。

## Scripts

```bash
npm install
npm run dev:h5
npm run build:h5
npm run dev:mp-weixin
npm run build:mp-weixin:clean
```

## WeChat DevTools

1. 在源码目录先生成干净的小程序构建产物：

```bash
npm run build:mp-weixin:clean
```

2. 推荐在微信开发者工具中导入 `mobile-h5/dist/build/mp-weixin`。如果开发者工具当前项目路径仍是源码目录 `mobile-h5`，也可以运行；源码根项目通过 `miniprogramRoot: "dist/build/mp-weixin"` 指向构建产物。
3. 在微信开发者工具“项目详情 / 本地设置”确认“压缩 JS”和“压缩 WXML”关闭。本仓库的源码根和 dist 根配置都固定 `setting.minified=false`、`setting.minifyWXML=false`、`setting.urlCheck=false`、`libVersion=3.15.1`。
4. 如果排查旧项目缓存，先在“项目详情”检查项目路径：路径是 `mobile-h5` 时读取源码根 `project.config.json` 和 `project.private.config.json`；路径是 `mobile-h5/dist/build/mp-weixin` 时读取 dist 根配置。
5. 编译前先在微信开发者工具文件树确认能看到关键页面产物：`pages/support/index.wxml`、`pages/couple-album/create/index.wxml`、`pages/couple-album/detail/index.wxml`、`pages/couple-album/share/index.wxml`。
6. 如果开发者工具提示相册页 `index.wxml not found` 或 `__route__ is not defined`，优先确认当前导入路径是 `mobile-h5/dist/build/mp-weixin`，或源码根项目的 `miniprogramRoot` 仍指向 `dist/build/mp-weixin`。同时检查 `dist/build/mp-weixin/app.json` 已注册 `pages/couple-album/create/index`、`pages/couple-album/detail/index`、`pages/couple-album/share/index`。
7. 每次排查旧包缓存时，重新运行 `npm run build:mp-weixin:clean`，回到微信开发者工具执行“清除全部缓存”，再点“重新编译”。
8. 模拟器控制台应出现 `IMAGE_AGENT_MP_BUILD no-urlsearchparams-v2`。如果没有看到这行日志，当前运行的仍不是最新构建产物。

未登录进入相册创建页时，请求 `/api/me` 返回 `401` 是预期鉴权行为，页面应继续跳转登录页；它不是相册页 WXML 缺失的根因。

当前项目固定微信基础库 `libVersion` 为 `3.15.1`。Linux 版微信开发者工具 `2.01.2510290` 搭配基础库 `3.15.2` 可能在页面跳转后抛出 `Cannot read property '__subPageFrameEndTime__' of null`，堆栈位于 `__dev__/WAServiceMainContext.js`；这属于开发者工具/基础库运行时内部错误，不是业务页面缺少文件。

控制台里的 `reportRealtimeAction:fail not support` 是微信开发者工具或当前基础库能力日志，不是客服页 `pages/support/index.wxml` 缺失的根因。

小程序模拟器默认请求 `https://example.com`。如果要连本地后端，可以在构建时覆盖 `VITE_API_BASE_URL`。

如果模拟器报 `request:fail url not in domain list`，这是微信开发者工具的合法域名校验拦截，不代表 `8888` 端口没启动。请确认“本地设置 / 不校验合法域名、web-view、TLS 版本以及 HTTPS 证书”已勾选；本仓库的 `project.config.json` 和 `project.private.config.json` 也应保持 `setting.urlCheck` 为 `false`。真机预览和正式版不能依赖关闭校验，需要使用 HTTPS 域名并配置到小程序后台的 request 合法域名。

真机预览如果要连本地开发机，需用电脑局域网地址重新构建，例如：

```bash
VITE_API_BASE_URL=http://电脑局域网IP:8888 npm run build:mp-weixin:clean
```

## Structure

- `src/pages.json`: 页面注册，首页是 `pages/home/index`
- `src/manifest.json`: H5、APP、小程序基础配置
- `src/pages/home/index.vue`: 移动端首页
- `src/styles`: 全局设计变量和基础样式
- `src/static/home-reference.png`: 首页视觉参考图

当前首页先完成静态移动端体验，工作台、充值、账户、客服路由预留在 `src/utils/routes.js`。

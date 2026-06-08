# 用 Sealtun 把本地服务一键暴露到公网

本地服务开发完成后，最常见的麻烦不是“能不能跑起来”，而是“怎么让别人访问到”。

你可能遇到过这些场景：

- 前端页面跑在本地 `3000` 端口，想发给同事验收
- 后端接口还没部署，但需要给前端联调
- 第三方平台的 Webhook 必须回调公网地址
- 想临时给客户演示一个功能，但不想走完整发布流程

如果你已经在使用 Sealos，那么可以直接用 **Sealtun** 解决这个问题。

## Sealtun 是什么

Sealtun 是一个面向 Sealos Cloud 和 Kubernetes 用户的本地隧道工具。

它的使用方式很直接：

```bash
npm install -g sealtun
sealtun login
sealtun expose 3000
```

执行后，Sealtun 会为你的本地 `localhost:3000` 生成一个公网 HTTPS 地址。别人访问这个地址时，请求会通过 Sealos 上的隧道代理转发回你的本地服务。

## 适合哪些场景

### 1. 本地页面快速验收

假设你正在开发一个 Next.js、Vite 或 Nuxt 项目：

```bash
npm run dev
```

服务运行在本地 `3000` 端口后，只需要执行：

```bash
sealtun expose 3000
```

你就可以拿到一个公网 HTTPS 地址，直接发给产品、设计或测试同事。

不需要临时部署，不需要改 DNS，也不需要把半成品代码推到生产环境。

### 2. 前后端本地联调

后端服务还在本地开发，但前端同事需要访问真实接口时，可以把后端端口暴露出去：

```bash
sealtun expose 8080
```

这样前端只需要把 API Base URL 临时改成 Sealtun 生成的地址，就能直接联调你的本地服务。

如果接口不希望公开访问，还可以开启 Basic Auth：

```bash
export SEALTUN_BASIC_AUTH_PASSWORD='change-me'

sealtun expose 8080 \
  --basic-auth-user admin \
  --basic-auth-password-env SEALTUN_BASIC_AUTH_PASSWORD
```

### 3. Webhook 回调调试

很多第三方平台要求填写公网回调地址，比如：

- GitHub Webhook
- 飞书、钉钉、企业微信机器人
- 支付回调
- OAuth Callback
- AI Agent 工具回调

传统做法通常需要部署一个临时服务。使用 Sealtun 后，可以直接让第三方平台回调你的本地服务。

```bash
sealtun expose 3000
```

然后把生成的 HTTPS 地址填到第三方平台的 Webhook 配置里即可。

## 和普通内网穿透有什么不同

Sealtun 的重点不是重新发明一个隧道服务，而是把隧道能力放进 Sealos 和 Kubernetes 的体系里。

它会自动在你的 Sealos Namespace 中创建并管理：

- Deployment
- Service
- Ingress
- HTTPS 入口
- 隧道代理 Pod

同时还支持：

- Sealos OAuth 登录
- 多 region 切换
- 多 profile 管理
- 自定义域名
- DNS 和证书诊断
- 本地 dashboard
- 远端日志和指标查看
- YAML 声明式配置

也就是说，它更适合已经在 Sealos 上开发、部署和管理应用的用户。

## 一个完整示例

本地启动一个 Web 服务：

```bash
npm run dev
```

登录 Sealos：

```bash
sealtun login
```

暴露本地端口：

```bash
sealtun expose 3000
```

查看已有隧道：

```bash
sealtun list
```

查看隧道状态：

```bash
sealtun inspect <tunnel-id>
```

查看远端日志：

```bash
sealtun logs <tunnel-id>
```

启动本地控制台：

```bash
sealtun dashboard
```

## 什么时候应该用 Sealtun

如果你只是偶尔临时分享一个本地页面，任何 tunnel 工具都可以。

但如果你已经在使用 Sealos，并且希望本地开发、公网调试、域名证书、Kubernetes 资源管理能够连在一起，那么 Sealtun 会更顺手。

它尤其适合：

- Sealos 用户
- Kubernetes 开发者
- 后端和全栈开发者
- 需要频繁调试 Webhook 的团队
- 需要快速给同事或客户演示本地功能的团队

## 开始使用

安装：

```bash
npm install -g sealtun
```

登录：

```bash
sealtun login
```

暴露本地服务：

```bash
sealtun expose 3000
```

项目地址：

```text
https://github.com/gitlayzer/sealtun
```

Sealtun 的目标很简单：让 Sealos 用户可以像使用 cloudflared 或 ngrok 一样，把本地服务快速、安全地暴露到公网，同时保留 Kubernetes 原生的可管理性。

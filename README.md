<p align="center">
  <img src="./assets/sealtun-logo.svg.png" alt="Sealtun logo" width="220">
</p>

# Sealtun

[English](./README_EN.md) · [功能总览](./Features.md) · [QuickStart](./QuickStart.md) · [Changelog](./CHANGELOG.md)

**Sealos Cloud 原生的本地隧道 CLI：一条命令，把本地服务送上公网。**

本地跑着 Web、API、数据库或调试服务，一行命令就能拿到带 HTTPS 的公网地址——发给同事预览、接进 webhook 回调、用手机扫码访问。不习惯命令行，`sealtun ui` 有完整的 Web 控制台。

```bash
npm install -g sealtun
sealtun login          # 无浏览器环境加 --qr，手机扫码授权
sealtun up             # 交互式创建第一条隧道
# => https://sealtun-xxxx-ns-xxxx.sealosgzg.site
```

完整功能见 [Features.md](./Features.md)，上手教程见 [QuickStart.md](./QuickStart.md)。

## 成本说明

Sealtun 本身不收软件费。成本来自 Sealos Cloud 为隧道分配的 Pod（CPU + 内存）、公网端口和网络流量，按小时计费，各 region 单价见 Sealos Cloud 控制台价格页。

压低成本：`tunnel stop` 不用的隧道缩容到 0、YAML `resources` 调低 requests/limits、短时调试隧道加 `--ttl` 自动销毁。

## 许可证

MIT License.

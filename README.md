# sol-grid-bot

🤖 基于 Telegram 的 Solana 链网格交易机器人

## 📋 项目简介

SolGridBot 是一个智能的网格交易机器人，通过 Telegram 接口为用户提供 Solana 链上代币的自动化网格交易服务。用户只需设置价格区间，机器人将在该区间内自动执行低买高卖的网格交易策略，帮助用户在震荡行情中获利。

- 🐛 Bug 报告：[Issues](https://github.com/fachebot/sol-grid-bot/issues)
- 📧 使用交流：[电报群](https://t.me/+sRrZC-LVPAsyOWE1)

## ✨ 功能特性

- 🚀 一键部署：无外部依赖，支持快速独立部署
- 🔗 Sol 链支持：专为 Solana 链优化的交易体验
- 🎯 智能网格交易：在用户设定的价格区间内自动执行低买高卖策略
- 🔗 稳定币交易：使用 USDC 交易代币，避免主币波动风险
- 📱 Telegram 集成：通过 Telegram Bot 提供便捷的用户交互界面
- 📊 实时监控：通过 Telegram Bot 实时查询盈亏情况和历史交易
- 🛡️ 安全可靠：本地部署，私钥不离开用户环境
- 💻 易于部署：支持部署在笔记本、家庭电脑、服务器等环境

## 🏗️ 技术架构

- 开发语言：Go 1.24.1
- 数据库：SQLite3
- 消息接口：Telegram Bot API
- 价格数据：GMGN API/OKX Web3 API
- 架构模式：无外部依赖的单体应用

## 🚀 快速开始

> 📌 **普通用户提示**：如果您不是开发者，可以直接跳过下面的编译步骤，前往 [Release 页面](https://github.com/fachebot/sol-grid-bot/releases) 下载最新版本的可执行文件，然后直接查看 [配置说明](#⚙️-配置说明) 部分，修改配置文件后即可运行。

### 环境要求

- Git
- Go 1.24.1 或更高版本

### 安装部署

**1. 克隆项目**

```bash
git clone https://github.com/fachebot/sol-grid-bot.git
```

**2. 安装依赖**

```bash
go mod tidy
```

**3. 构建项目**

```bash
go build
```

**4. 运行项目**

```bash
# linux
./sol-grid-bot

# windows
./sol-grid-bot.exe
```

> ⚠️ 重要：运行项目前需要创建配置文件，请查看下面的配置说明。

### ⚙️ 配置说明

在运行项目前，需要创建配置文件 `etc/config.yaml`，你可以复制 [etc/config.yaml.sample](etc/config.yaml.sample) 文件到 `etc/config.yaml` 并进行修改：

```yaml
# Solana配置
Solana:
  RpcUrl: "https://api.mainnet-beta.solana.com" # 主网RPC地址
  MaxRetries: 1 # 重试次数
  SlippageBps: 250 # 滑点Bps
  MaxLamports: 5000000 # 每笔交易最高费用上限
  PriorityLevel: medium # 优先等级(medium/high/veryHigh)
  DexAggregator: jup # DEX聚合器(jup/okx/relay)

# Jup配置
Jupiter:
  Url: "https://lite-api.jup.ag" # Jupiter API地址
  Apikey: "" # Jupiter API密钥

# 数据API(gmgn/jupag/okx)
Datapi: gmgn

# Okx配置
OkxWeb3:
  Apikey:
  Secretkey:
  Passphrase:

# 代理服务器配置
Sock5Proxy:
  Host: 127.0.0.1 # 代理服务器地址
  Port: 10808 # 代理服务器端口
  Enable: true # 是否启用代理

# 电报机器人配置
TelegramBot:
  Debug: true
  ApiToken: 7916072799:AAFb-C25RgEAxNClxqeRpTkmO6C8e7FhzLs
  WhiteList:
    - 993021715

# 默认网格设置
DefaultGridSettings:
  OrderSize: 38 # 每格大小
  MaxGridLimit: 15 # 最大网格数量
  StopLossExit: 0 # 止损金额阈值
  TakeProfitExit: 80 # 盈利目标金额
  TakeProfitRatio: 3.5 # 止盈百分比(%)
  EnableAutoExit: true # 跌破自动清仓
  LastKlineVolume: 1000 # 最近交易量
  FiveKlineVolume: 8888 # 最近5分钟交易量
  GlobalTakeProfitRatio: 0 # 全局止盈涨幅(%)
  DropOn: true # 防瀑布开关
  CandlesToCheck: 3 # 防瀑布K线根数
  DropThreshold: 20 # 防瀑布跌幅阈值百分比(%)

# 快速启动网格设置
QuickStartSettings:
  OrderSize: 30 # 每格大小
  MaxGridLimit: 10 # 最大网格数量
  StopLossExit: 0 # 止损金额阈值
  TakeProfitExit: 80 # 盈利目标金额
  TakeProfitRatio: 3.5 # 止盈百分比(%)
  EnableAutoExit: true # 跌破自动清仓
  LastKlineVolume: 3000 # 最近交易量
  FiveKlineVolume: 20000 # 最近5分钟交易量
  UpperPriceBound: 0.0002 # 网格价格上限
  LowerPriceBound: 0.00005 # 网格价格下限
  GlobalTakeProfitRatio: 0 # 全局止盈涨幅(%)
  DropOn: true # 防瀑布开关
  CandlesToCheck: 3 # 防瀑布K线根数
  DropThreshold: 20 # 防瀑布跌幅阈值百分比(%)

# 创建代币策略的必要条件
TokenRequirements:
  MinMarketCap: 200000 # 最小代币市值
  MinHolderCount: 1500 # 最小代币持有人数
  MinTokenAgeMinutes: 240 # 最小代币年龄(分钟)
  MaxTokenAgeMinutes: 960 # 最高代币年龄(分钟)
```

除了 API 密钥需要使用自己的配置外，其他配置项可使用默认值。

#### 获取必要的 API 密钥

**1. SOL RPC URL**

- 访问 [QuickNode](https://www.quicknode.com/)
- 注册免费账户
- 创建 SOL 主网节点
- 获取 RPC URL
- 将 RPC URL 填写到配置文件 `Solana.RpcUrl` 项

**2. OKX Web3 API**

- 访问 [OKX Web3 开发者平台](https://web3.okx.com/zh-hans/build/dev-portal)
- 使用钱包登录并且在账号设置界面绑定邮箱和手机
- 前往 API Key 页面创建一个新的 API Key
- 将 API Key、Secret Key 和 Passphrase 填写到配置文件 `OkxWeb3` 项

**3. Telegram Bot Token**

- 在 Telegram 中找到 [@BotFather](https://t.me/botfather)
- 发送 /newbot 创建新机器人
- 按提示设置机器人名称和用户名
- 获取 Bot Token
- 将 Bot Token 填写到配置文件 `TelegramBot.ApiToken` 项

#### 网络代理配置

由于网络限制可能导致无法访问 Telegram Bot 服务器，用户可以配置 `Sock5Proxy` 代理来解决连接问题：

```yaml
Sock5Proxy:
  Host: 127.0.0.1
  Port: 10808
  Enable: true # 设置为 true 启用代理
```

## ⚠️ 重要注意事项

### 安全风险

- 🔐 私钥安全：请确保私钥安全，建议使用专门的交易钱包
- 💾 数据备份：运行机器人后会自动创建 `data` 文件夹存储机器人数据，删除此文件夹将丢失所有数据，包括私钥，请谨慎操作

### 交易风险

- 💸 网络费用：每次交易都会产生 SOL 网络手续费
- 📈 市场风险：网格交易适合震荡行情，单边行情可能产生损失
- ⏰ 交易延迟：由于使用免费 API 服务，交易可能存在延迟，不适用于高波动代币交易

### 技术限制

- 🔄 API 限制：注意各 API 服务的调用频率限制
- 🛡️ 反爬虫机制：价格数据从 GMGN 和 OKX 抓取，如因反爬虫机制导致价格获取失败，可尝试修改配置文件 `Datapi` 选项

## 📄 免责声明

**本项目仅供学习和研究使用，使用者需自行承担交易风险。请谨慎使用，作者不对任何损失负责。**

在使用本软件进行交易前，请确保您：

- 充分理解网格交易的风险和机制
- 具备相应的技术知识和风险承受能力
- 仅使用您可以承受损失的资金进行交易

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/fachebot/sol-grid-bot/internal/config"
)

// MainUI 主界面
type MainUI struct {
	app        fyne.App
	window     fyne.Window
	deployDir  string
	exePath    string
	configPath string

	// 管理器
	processManager *ProcessManager
	logViewer      *LogViewer
	downloader     *Downloader

	// UI组件 - 顶部控制区域
	statusLabel         *widget.Label
	startBtn            *widget.Button
	stopBtn             *widget.Button
	currentVersionLabel *widget.Label
	latestVersionLabel  *widget.Label
	checkVersionBtn     *widget.Button

	// UI组件 - 配置区域
	configDisplayArea *widget.RichText
	configContainer   *fyne.Container // 配置项容器
	editConfigBtn     *widget.Button

	// UI组件 - 日志区域
	logText         *widget.RichText
	logLevelBtn     *widget.Button
	logClearBtn     *widget.Button
	autoScrollCheck *widget.Check

	// 更新通道
	updateChan chan struct{}

	// 版本信息
	currentVersion string
	latestVersion  string

	// 配置验证状态缓存
	configValidationStatus map[string]string
	// 配置验证额外信息缓存（链ID、bot username等）
	configValidationExtra map[string]string // key: "rpc", "okx", "telegram", value: 验证状态
}

// NewMainUI 创建主界面
func NewMainUI(app fyne.App, deployDir string) *MainUI {
	window := app.NewWindow("SOL网格启动器")
	window.Resize(fyne.NewSize(1200, 800))
	window.CenterOnScreen()

	// 查找可执行文件
	exePath := findExecutable(deployDir)
	configPath := filepath.Join(deployDir, "etc", "config.yaml")

	ui := &MainUI{
		app:                    app,
		window:                 window,
		deployDir:              deployDir,
		exePath:                exePath,
		configPath:             configPath,
		processManager:         NewProcessManager(),
		logViewer:              NewLogViewer(),
		downloader:             NewDownloader(),
		updateChan:             make(chan struct{}, 1), // 缓冲通道，避免阻塞
		currentVersion:         "未知",
		latestVersion:          "未检查",
		configValidationStatus: make(map[string]string), // 初始化验证状态缓存
		configValidationExtra:  make(map[string]string), // 初始化验证额外信息缓存
	}

	// 检测当前版本
	ui.detectCurrentVersion()

	// 设置日志回调
	ui.processManager.SetLogCallback(func(level, message string) {
		// 使用fyne.Do确保在主线程中调用AppendLog
		fyne.Do(func() {
			ui.logViewer.AppendLog(level, message)
		})
	})

	// 创建界面
	ui.createUI()

	// 初始化版本显示（在UI创建后，直接设置，因为此时还在主线程）
	if ui.currentVersionLabel != nil {
		ui.currentVersionLabel.SetText(fmt.Sprintf("📦 当前版本: %s", ui.currentVersion))
	}
	if ui.latestVersionLabel != nil {
		ui.latestVersionLabel.SetText(fmt.Sprintf("🔄 最新版本: %s", ui.latestVersion))
	}

	// 如果可执行文件不存在，自动下载最新版本
	if ui.exePath == "" {
		go func() {
			// 等待一小段时间确保UI完全创建
			time.Sleep(1 * time.Second)
			ui.autoDownloadLatestVersion()
		}()
	} else {
		// 启动时自动检查最新版本（延迟执行，确保UI完全初始化）
		go func() {
			// 等待一小段时间确保UI完全创建
			time.Sleep(1 * time.Second)
			ui.checkLatestVersionSilent()
		}()
	}

	// 启动状态检查（延迟启动，确保UI完全初始化）
	go func() {
		// 等待一小段时间确保UI完全创建
		time.Sleep(500 * time.Millisecond)
		ui.startStatusChecker()
	}()

	return ui
}

// findExecutable 查找可执行文件
func findExecutable(deployDir string) string {
	// 尝试查找可执行文件
	exeName := "sol-grid-bot"
	if os.Getenv("GOOS") == "windows" || filepath.Ext(os.Args[0]) == ".exe" {
		exeName = "sol-grid-bot.exe"
	}

	// 检查当前目录
	exePath := filepath.Join(deployDir, exeName)
	if _, err := os.Stat(exePath); err == nil {
		return exePath
	}

	// 检查常见位置
	paths := []string{
		filepath.Join(deployDir, "bin", exeName),
		filepath.Join(deployDir, "build", exeName),
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// createUI 创建界面
func (ui *MainUI) createUI() {
	// 顶部控制区域
	topControl := ui.createTopControlArea()

	// 配置区域
	configArea := ui.createConfigArea()

	// 日志区域
	logArea := ui.createLogArea()

	// 使用Border布局：顶部控制、中间配置、底部日志
	// 添加整体内边距，使界面更美观
	content := container.NewBorder(
		topControl, // 顶部
		logArea,    // 底部
		nil,        // 左侧
		nil,        // 右侧
		configArea, // 中间
	)

	// 添加整体内边距
	paddedContent := container.NewPadded(content)

	ui.window.SetContent(paddedContent)
}

// createTopControlArea 创建顶部控制区域
func (ui *MainUI) createTopControlArea() fyne.CanvasObject {
	// 左侧：启动和停止按钮（使用图标和更好的样式）
	ui.startBtn = widget.NewButton("▶ 启动", func() {
		ui.startProgram()
	})
	ui.startBtn.Importance = widget.HighImportance

	ui.stopBtn = widget.NewButton("⏸ 停止", func() {
		ui.stopProgram()
	})
	ui.stopBtn.Importance = widget.MediumImportance

	// 中间：状态显示（使用卡片样式）
	ui.statusLabel = widget.NewLabel("状态: 已停止")
	ui.statusLabel.Alignment = fyne.TextAlignCenter
	ui.statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	// 右侧：版本显示（水平布局）
	ui.currentVersionLabel = widget.NewLabel(fmt.Sprintf("📦 当前版本: %s", ui.currentVersion))

	ui.latestVersionLabel = widget.NewLabel(fmt.Sprintf("🔄 最新版本: %s", ui.latestVersion))

	// 检查版本按钮
	ui.checkVersionBtn = widget.NewButton("检查更新", func() {
		ui.checkLatestVersion()
	})
	ui.checkVersionBtn.Importance = widget.MediumImportance

	// 初始状态
	ui.updateButtonStates()
	ui.updateStatus()

	// 左侧：控制按钮和状态
	leftGroup := container.NewHBox(
		ui.startBtn,
		ui.stopBtn,
		widget.NewSeparator(),
		ui.statusLabel,
	)

	// GitHub链接按钮
	githubBtn := widget.NewButton("🔗 GitHub", func() {
		OpenURL("https://github.com/fachebot/sol-grid-bot")
	})
	githubBtn.Importance = widget.LowImportance

	// 右侧：版本信息和检查按钮
	rightGroup := container.NewHBox(
		ui.currentVersionLabel,
		widget.NewSeparator(),
		ui.latestVersionLabel,
		widget.NewSeparator(),
		ui.checkVersionBtn,
		widget.NewSeparator(),
		githubBtn,
	)

	// 使用Border布局，左侧按钮和状态，右侧版本信息
	controlContent := container.NewBorder(
		nil, nil,
		leftGroup,  // 左侧
		rightGroup, // 右侧
		nil,        // 中间留空
	)

	// 使用卡片包装，添加标题和内边距
	topCard := widget.NewCard("", "", container.NewPadded(controlContent))

	return container.NewPadded(topCard)
}

// createConfigArea 创建配置区域
func (ui *MainUI) createConfigArea() fyne.CanvasObject {
	// 配置项容器
	ui.configContainer = container.NewVBox()

	// 启动时默认验证一次
	ui.updateConfigDisplayWithValidation(true)

	configScroll := container.NewScroll(container.NewPadded(ui.configContainer))
	configScroll.SetMinSize(fyne.NewSize(400, 290)) // 增加高度以避免滚动条

	configDisplayCard := widget.NewCard("核心配置", "", configScroll)

	// 操作按钮（使用卡片包装）
	ui.editConfigBtn = widget.NewButton("📝 打开配置", func() {
		ui.openConfigFile()
	})
	ui.editConfigBtn.Importance = widget.HighImportance

	validateConfigBtn := widget.NewButton("🔍 验证配置", func() {
		ui.updateConfigDisplayWithValidation(true)
	})
	validateConfigBtn.Importance = widget.MediumImportance

	configHelpBtn := widget.NewButton("📖 配置说明", func() {
		OpenURL("https://github.com/fachebot/sol-grid-bot/blob/main/etc/config.yaml.sample")
	})
	configHelpBtn.Importance = widget.LowImportance

	rightButtons := container.NewVBox(
		container.NewPadded(ui.editConfigBtn),
		container.NewPadded(validateConfigBtn),
		container.NewPadded(configHelpBtn),
	)
	rightButtonsCard := widget.NewCard("操作", "", rightButtons)

	// 使用HSplit布局：核心配置显示区域 | 右侧按钮
	fullSplit := container.NewHSplit(
		container.NewPadded(configDisplayCard),
		container.NewPadded(rightButtonsCard),
	)
	fullSplit.SetOffset(0.8) // 核心配置占80%

	// 整体布局：使用卡片包装整个配置区域
	configCard := widget.NewCard("⚙️ 配置管理", "", container.NewPadded(fullSplit))

	return container.NewPadded(configCard)
}

// updateConfigDisplay 更新配置显示区域
func (ui *MainUI) updateConfigDisplay() {
	ui.updateConfigDisplayWithValidation(false)
}

// updateConfigDisplayWithValidation 更新配置显示区域，可选择是否验证
func (ui *MainUI) updateConfigDisplayWithValidation(shouldValidate bool) {
	// 在后台goroutine中加载配置和准备数据
	go func() {
		cfg, err := config.LoadFromFile(ui.configPath)

		var configItems []fyne.CanvasObject

		if err != nil {
			// 配置文件未找到 - 在主线程中创建GUI组件
			fyne.Do(func() {
				errorText := widget.NewRichTextFromMarkdown("⚠️ **配置文件未找到**\n\n" +
					"配置文件 `etc/config.yaml` 不存在。\n\n" +
					"**解决方法：**\n" +
					"1. 点击右侧的\"📝 打开配置\"按钮\n" +
					"2. 系统会自动从 `etc/config.yaml.sample` 创建配置文件\n" +
					"3. 使用系统默认编辑器编辑配置文件\n" +
					"4. 保存后返回此界面验证配置\n\n" +
					"**需要配置的项目：**\n" +
					"- 🌐 Solana RPC URL（Solana网络RPC地址）\n" +
					"- 🔑 OKX API（API Key、Secret Key、Passphrase）\n" +
					"- 💬 Telegram Bot（Bot Token）")
				configItems = append(configItems, errorText)

				if ui.configContainer != nil {
					ui.configContainer.Objects = configItems
					ui.configContainer.Refresh()
				}
			})
			return
		} else {
			// 准备配置数据（在后台线程中）
			rpcUrl := cfg.Solana.RpcUrl
			okxApikey := cfg.OkxWeb3.Apikey
			telegramToken := cfg.TelegramBot.ApiToken

			// 如果需要验证，在后台线程中执行验证
			if shouldValidate {
				if rpcUrl != "" {
					go func() {
						validator := &Validator{}
						err := validator.ValidateSolanaRPC(rpcUrl)
						if err != nil {
							ui.configValidationStatus["rpc"] = "❌ 验证失败: " + err.Error()
						} else {
							ui.configValidationStatus["rpc"] = "✅ 验证成功"
						}
						fyne.Do(func() {
							ui.updateConfigDisplay()
						})
					}()
				}

				if okxApikey != "" {
					go func() {
						validator := &Validator{}
						if err := validator.ValidateOKX(cfg.OkxWeb3.Apikey, cfg.OkxWeb3.Secretkey, cfg.OkxWeb3.Passphrase); err != nil {
							ui.configValidationStatus["okx"] = "❌ 验证失败: " + err.Error()
						} else {
							ui.configValidationStatus["okx"] = "✅ 验证成功"
						}
						fyne.Do(func() {
							ui.updateConfigDisplay()
						})
					}()
				}

				if telegramToken != "" {
					go func() {
						validator := &Validator{}
						result := validator.ValidateTelegramWithResult(telegramToken)
						if result.Error != nil {
							ui.configValidationStatus["telegram"] = "❌ 验证失败: " + result.Error.Error()
							delete(ui.configValidationExtra, "telegram_username")
						} else {
							ui.configValidationStatus["telegram"] = "✅ 验证成功"
							ui.configValidationExtra["telegram_username"] = fmt.Sprintf("Bot: @%s", result.Username)
						}
						fyne.Do(func() {
							ui.updateConfigDisplay()
						})
					}()
				}
			}

			// 在主线程中创建GUI组件
			fyne.Do(func() {
				// Solana RPC URL
				rpcItem := ui.createConfigItem("🌐 Solana RPC URL", rpcUrl, "rpc", false, func() {
					ui.showRPCConfigDialog()
				}, nil)
				configItems = append(configItems, rpcItem)
				configItems = append(configItems, widget.NewSeparator())

				// OKX API
				okxItem := ui.createConfigItem("🔑 OKX API", okxApikey, "okx", false, func() {
					ui.showOKXConfigDialog()
				}, nil)
				configItems = append(configItems, okxItem)
				configItems = append(configItems, widget.NewSeparator())

				// Telegram Bot
				telegramItem := ui.createConfigItem("💬 Telegram Bot", telegramToken, "telegram", false, func() {
					ui.showTelegramConfigDialog()
				}, nil)
				configItems = append(configItems, telegramItem)

				if ui.configContainer != nil {
					ui.configContainer.Objects = configItems
					ui.configContainer.Refresh()
				}
			})
		}
	}()
}

// createConfigItem 创建配置项显示（包含内容和按钮）
func (ui *MainUI) createConfigItem(title, value, key string, shouldValidate bool, onConfigClick func(), onValidate func()) fyne.CanvasObject {
	var displayText string
	var statusText string

		if value == "" {
			displayText = fmt.Sprintf("❌ **%s**: 未配置", title)
			delete(ui.configValidationStatus, key)
			delete(ui.configValidationExtra, key+"_username")
		} else {
			// 构建配置值显示文本
			valueText := maskSensitiveInfo(value)

			// 获取验证状态和额外信息
			var status string
			var usernameExtra string

			if shouldValidate {
				// 执行验证
				onValidate()
				if s, ok := ui.configValidationStatus[key]; ok {
					status = s
					if status == "✅ 验证成功" {
						if extra, ok := ui.configValidationExtra[key+"_username"]; ok {
							usernameExtra = extra
						}
					}
				}
			} else {
				// 使用缓存的验证状态
				if s, ok := ui.configValidationStatus[key]; ok {
					status = s
					if status == "✅ 验证成功" {
						if extra, ok := ui.configValidationExtra[key+"_username"]; ok {
							usernameExtra = extra
						}
					}
				}
			}

			// 将username添加到配置值后面
			if usernameExtra != "" {
				// 提取username部分（去掉"Bot: @"前缀）
				usernameValue := strings.TrimPrefix(usernameExtra, "Bot: @")
				valueText += fmt.Sprintf(" (@%s)", usernameValue)
			}

		displayText = fmt.Sprintf("**%s**\n```\n%s\n```\n", title, valueText)

		// 状态文本只显示验证状态，不包含额外信息
		if status != "" {
			statusText = status
		} else {
			statusText = "⏸️ 未验证"
		}
	}

	contentText := widget.NewRichTextFromMarkdown(displayText + statusText)

	// 创建修改按钮
	configBtn := widget.NewButton("修改", onConfigClick)
	configBtn.Importance = widget.LowImportance // 使用低重要性，显示为更柔和的样式

	// 使用HSplit布局：内容在左，按钮在右
	split := container.NewHSplit(
		container.NewPadded(contentText),
		container.NewPadded(container.NewCenter(configBtn)), // 垂直居中按钮
	)
	split.SetOffset(0.95) // 按钮只占5%的区域
	return split
}

// maskSensitiveInfo 掩码敏感信息
func maskSensitiveInfo(s string) string {
	if len(s) <= 20 {
		// 如果长度小于等于20，显示全部
		return s
	}
	// 显示前后各10个字符
	return s[:10] + "..." + s[len(s)-10:]
}

// showRPCConfigDialog 显示RPC配置dialog
func (ui *MainUI) showRPCConfigDialog() {
	cfg, _ := config.LoadFromFile(ui.configPath)

	title := widget.NewRichTextFromMarkdown("## 🌐 配置 Solana RPC 地址")
	description := widget.NewRichTextFromMarkdown(`
需要Solana链的RPC地址，您可以选择以下任一服务商注册获取：
	`)

	alchemyBtn := widget.NewButton("注册 Alchemy", func() {
		OpenURL("https://www.alchemy.com/")
	})

	quicknodeBtn := widget.NewButton("注册 QuickNode", func() {
		OpenURL("https://www.quicknode.com/")
	})

	rpcEntry := widget.NewEntry()
	if cfg != nil && cfg.Solana.RpcUrl != "" {
		rpcEntry.SetText(cfg.Solana.RpcUrl)
	}
	rpcEntry.SetPlaceHolder("请输入Solana RPC地址，例如: https://...")

	validateStatus := widget.NewLabel("")
	validateStatus.Hide()

	var rpcValid bool
	validateBtn := widget.NewButton("验证", func() {
		validateStatus.Show()
		validateStatus.SetText("验证中...")
		go func() {
			rpcUrl := rpcEntry.Text
			if rpcUrl == "" {
				fyne.Do(func() {
					validateStatus.SetText("请输入RPC地址")
					validateStatus.Importance = widget.WarningImportance
					rpcValid = false
				})
				return
			}

			validator := &Validator{}
			err := validator.ValidateSolanaRPC(rpcUrl)
			if err != nil {
				fyne.Do(func() {
					validateStatus.SetText("✗ 验证失败: " + err.Error())
					validateStatus.Importance = widget.DangerImportance
					rpcValid = false
				})
			} else {
				// 验证成功，但不自动保存
				fyne.Do(func() {
					validateStatus.SetText("✓ 验证成功")
					validateStatus.Importance = widget.SuccessImportance
					rpcValid = true
				})
			}
		}()
	})

	var d *dialog.CustomDialog
	var rpcUrlToSave string

	saveBtn := widget.NewButton("保存", func() {
		if !rpcValid {
			dialog.ShowError(fmt.Errorf("请先验证RPC地址"), ui.window)
			return
		}
		// 保存配置
		rpcUrlToSave = rpcEntry.Text
		configurator := NewConfigurator(ui.deployDir)
		configurator.UpdateConfig(func(cfg *config.Config) {
			cfg.Solana.RpcUrl = rpcUrlToSave
		})
		// 刷新主界面配置显示
		ui.updateConfigDisplayWithValidation(true)
		d.Hide()
	})

	content := container.NewVBox(
		title,
		widget.NewSeparator(),
		description,
		container.NewHBox(alchemyBtn, quicknodeBtn),
		widget.NewSeparator(),
		widget.NewLabel("RPC地址:"),
		rpcEntry,
		container.NewHBox(validateBtn, validateStatus),
	)

	// 底部按钮区域（水平居中）
	bottomButtons := container.NewHBox(
		saveBtn,
		widget.NewButton("关闭", func() {
			d.Hide()
		}),
	)

	// 使用Border布局，将按钮放在底部并水平居中
	fullContent := container.NewBorder(
		nil, // 顶部
		container.NewPadded(container.NewCenter(bottomButtons)), // 底部按钮（水平居中）
		nil, // 左侧
		nil, // 右侧
		container.NewScroll(container.NewPadded(content)), // 中间内容
	)

	d = dialog.NewCustom("配置 RPC URL", "", fullContent, ui.window)
	d.Resize(fyne.NewSize(1000, 800))
	d.Show()
}

// showOKXConfigDialog 显示OKX配置dialog
func (ui *MainUI) showOKXConfigDialog() {
	cfg, _ := config.LoadFromFile(ui.configPath)

	title := widget.NewRichTextFromMarkdown("## 🔑 配置 OKX Web3 API")

	// 重要提醒
	importantNotice := widget.NewRichTextFromMarkdown(`
⚠️ **重要提醒：**

在创建 API 密钥之前，请确保您的开发者平台账户已经：
- ✅ **绑定邮箱地址**
- ✅ **绑定手机号码**

如果未绑定邮箱和手机号码，API 密钥可能无法正常使用或功能受限。
	`)

	description := widget.NewRichTextFromMarkdown(`
访问 [OKX Web3 开发者平台](https://web3.okx.com/zh-hans/build/dev-portal) 注册并创建API密钥
	`)

	okxLinkBtn := widget.NewButton("打开 OKX Web3 开发者平台", func() {
		OpenURL("https://web3.okx.com/zh-hans/build/dev-portal")
	})

	apikeyEntry := widget.NewEntry()
	secretkeyEntry := widget.NewPasswordEntry()
	passphraseEntry := widget.NewPasswordEntry()

	if cfg != nil {
		if cfg.OkxWeb3.Apikey != "" {
			apikeyEntry.SetText(cfg.OkxWeb3.Apikey)
		}
		if cfg.OkxWeb3.Secretkey != "" {
			secretkeyEntry.SetText(cfg.OkxWeb3.Secretkey)
		}
		if cfg.OkxWeb3.Passphrase != "" {
			passphraseEntry.SetText(cfg.OkxWeb3.Passphrase)
		}
	}

	apikeyEntry.SetPlaceHolder("API Key")
	secretkeyEntry.SetPlaceHolder("Secret Key")
	passphraseEntry.SetPlaceHolder("Passphrase")

	validateStatus := widget.NewLabel("")
	validateStatus.Hide()

	var okxValid bool
	validateBtn := widget.NewButton("验证", func() {
		validateStatus.Show()
		validateStatus.SetText("验证中...")
		go func() {
			apikey := apikeyEntry.Text
			secretkey := secretkeyEntry.Text
			passphrase := passphraseEntry.Text

			if apikey == "" || secretkey == "" || passphrase == "" {
				fyne.Do(func() {
					validateStatus.SetText("✗ 请填写完整的OKX API信息")
					validateStatus.Importance = widget.WarningImportance
					okxValid = false
				})
				return
			}

			validator := &Validator{}
			err := validator.ValidateOKX(apikey, secretkey, passphrase)
			if err != nil {
				fyne.Do(func() {
					validateStatus.SetText("✗ 验证失败: " + err.Error())
					validateStatus.Importance = widget.DangerImportance
					okxValid = false
				})
			} else {
				// 验证成功，但不自动保存
				fyne.Do(func() {
					validateStatus.SetText("✓ 验证成功")
					validateStatus.Importance = widget.SuccessImportance
					okxValid = true
				})
			}
		}()
	})

	var d *dialog.CustomDialog

	saveBtn := widget.NewButton("保存", func() {
		if !okxValid {
			dialog.ShowError(fmt.Errorf("请先验证OKX API密钥"), ui.window)
			return
		}
		// 保存配置
		apikey := apikeyEntry.Text
		secretkey := secretkeyEntry.Text
		passphrase := passphraseEntry.Text
		configurator := NewConfigurator(ui.deployDir)
		configurator.UpdateConfig(func(cfg *config.Config) {
			cfg.OkxWeb3.Apikey = apikey
			cfg.OkxWeb3.Secretkey = secretkey
			cfg.OkxWeb3.Passphrase = passphrase
		})
		// 刷新主界面配置显示
		ui.updateConfigDisplayWithValidation(true)
		d.Hide()
	})

	content := container.NewVBox(
		title,
		widget.NewSeparator(),
		importantNotice,
		widget.NewSeparator(),
		description,
		container.NewPadded(okxLinkBtn),
		widget.NewSeparator(),
		widget.NewLabel("API Key:"),
		apikeyEntry,
		widget.NewLabel("Secret Key:"),
		secretkeyEntry,
		widget.NewLabel("Passphrase:"),
		passphraseEntry,
		container.NewHBox(validateBtn, validateStatus),
	)

	// 底部按钮区域（水平居中）
	bottomButtons := container.NewHBox(
		saveBtn,
		widget.NewButton("关闭", func() {
			d.Hide()
		}),
	)

	// 使用Border布局，将按钮放在底部并水平居中
	fullContent := container.NewBorder(
		nil, // 顶部
		container.NewPadded(container.NewCenter(bottomButtons)), // 底部按钮（水平居中）
		nil, // 左侧
		nil, // 右侧
		container.NewScroll(container.NewPadded(content)), // 中间内容
	)

	d = dialog.NewCustom("配置 OKX API", "", fullContent, ui.window)
	d.Resize(fyne.NewSize(1000, 800))
	d.Show()
}

// showTelegramConfigDialog 显示Telegram配置dialog
func (ui *MainUI) showTelegramConfigDialog() {
	cfg, _ := config.LoadFromFile(ui.configPath)

	title := widget.NewRichTextFromMarkdown("## 💬 配置 Telegram Bot")

	// 详细的创建流程说明
	stepsGuide := widget.NewRichTextFromMarkdown(`
**📋 创建 Telegram Bot 的详细步骤：**

**步骤 1：打开 BotFather**
- 点击下方"打开 Telegram BotFather"按钮
- 或在 Telegram 中搜索 @BotFather

**步骤 2：创建新 Bot**
- 在 BotFather 对话框中发送命令：` + "`/newbot`" + `
- 按照提示输入 Bot 的名称（例如：My Trading Bot）
- 然后输入 Bot 的用户名（必须以 bot 结尾，例如：my_trading_bot）

**步骤 3：获取 Token**
- BotFather 会返回一个 Token，格式类似：` + "`123456789:ABCdefGHIjklMNOpqrsTUVwxyz`" + `
- 复制这个 Token

**步骤 4：粘贴 Token**
- 将复制的 Token 粘贴到下方的"Bot Token"输入框中
- 点击"验证"按钮验证 Token 是否有效

**步骤 5：保存配置**
- 验证成功后，点击"保存"按钮保存配置
	`)

	telegramLinkBtn := widget.NewButton("打开 Telegram BotFather", func() {
		OpenURL("https://t.me/botfather")
	})

	tokenEntry := widget.NewPasswordEntry()
	whitelistEntry := widget.NewEntry()

	if cfg != nil {
		if cfg.TelegramBot.ApiToken != "" {
			tokenEntry.SetText(cfg.TelegramBot.ApiToken)
		}
		if len(cfg.TelegramBot.WhiteList) > 0 {
			whitelistStr := ""
			for i, id := range cfg.TelegramBot.WhiteList {
				if i > 0 {
					whitelistStr += ", "
				}
				whitelistStr += fmt.Sprintf("%d", id)
			}
			whitelistEntry.SetText(whitelistStr)
		}
	}

	tokenEntry.SetPlaceHolder("Bot Token")
	whitelistEntry.SetPlaceHolder("白名单用户ID（可选，多个用逗号分隔，留空表示所有人可用）")

	validateStatus := widget.NewLabel("")
	validateStatus.Hide()

	var telegramValid bool
	validateBtn := widget.NewButton("验证", func() {
		validateStatus.Show()
		validateStatus.SetText("验证中...")
		go func() {
			token := tokenEntry.Text
			if token == "" {
				fyne.Do(func() {
					validateStatus.SetText("✗ 请填写Telegram Bot Token")
					validateStatus.Importance = widget.WarningImportance
					telegramValid = false
				})
				return
			}

			validator := &Validator{}
			result := validator.ValidateTelegramWithResult(token)
			if result.Error != nil {
				fyne.Do(func() {
					validateStatus.SetText("✗ 验证失败: " + result.Error.Error())
					validateStatus.Importance = widget.DangerImportance
					telegramValid = false
				})
			} else {
				// 验证成功，但不自动保存
				fyne.Do(func() {
					statusMsg := fmt.Sprintf("✓ 验证成功\nBot: @%s", result.Username)
					validateStatus.SetText(statusMsg)
					validateStatus.Importance = widget.SuccessImportance
					telegramValid = true
				})
			}
		}()
	})

	var d *dialog.CustomDialog

	saveBtn := widget.NewButton("保存", func() {
		if !telegramValid {
			dialog.ShowError(fmt.Errorf("请先验证Telegram Bot Token"), ui.window)
			return
		}
		// 保存配置
		token := tokenEntry.Text
		var whitelist []int64
		whitelistStr := strings.TrimSpace(whitelistEntry.Text)
		if whitelistStr != "" {
			parts := strings.Split(whitelistStr, ",")
			for _, part := range parts {
				id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
				if err == nil {
					whitelist = append(whitelist, id)
				}
			}
		}
		configurator := NewConfigurator(ui.deployDir)
		configurator.UpdateConfig(func(cfg *config.Config) {
			cfg.TelegramBot.ApiToken = token
			cfg.TelegramBot.WhiteList = whitelist
		})
		// 刷新主界面配置显示
		ui.updateConfigDisplayWithValidation(true)
		d.Hide()
	})

	content := container.NewVBox(
		title,
		widget.NewSeparator(),
		stepsGuide,
		container.NewPadded(telegramLinkBtn),
		widget.NewSeparator(),
		widget.NewLabel("Bot Token:"),
		tokenEntry,
		widget.NewLabel("白名单（可选）:"),
		whitelistEntry,
		container.NewHBox(validateBtn, validateStatus),
	)

	// 底部按钮区域（水平居中）
	bottomButtons := container.NewHBox(
		saveBtn,
		widget.NewButton("关闭", func() {
			d.Hide()
		}),
	)

	// 使用Border布局，将按钮放在底部并水平居中
	fullContent := container.NewBorder(
		nil, // 顶部
		container.NewPadded(container.NewCenter(bottomButtons)), // 底部按钮（水平居中）
		nil, // 左侧
		nil, // 右侧
		container.NewScroll(container.NewPadded(content)), // 中间内容
	)

	d = dialog.NewCustom("配置 Telegram Bot", "", fullContent, ui.window)
	d.Resize(fyne.NewSize(1000, 800))
	d.Show()
}

// openConfigFile 使用系统默认编辑器打开配置文件
func (ui *MainUI) openConfigFile() {
	configPath := ui.configPath

	// 检查文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// 如果配置文件不存在，尝试从示例文件创建
		configurator := NewConfigurator(ui.deployDir)
		if err := configurator.CopySampleConfig(); err != nil {
			dialog.ShowError(fmt.Errorf("配置文件不存在且创建失败: %w", err), ui.window)
			return
		}
		// 配置文件创建成功，显示提示并刷新显示
		dialog.ShowInformation("配置文件已创建", "已从示例文件创建配置文件，请编辑后保存。", ui.window)
		fyne.Do(func() {
			ui.updateConfigDisplay()
		})
	}

	// 使用系统默认编辑器打开文件
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Windows: 使用 start 命令打开文件
		cmd = exec.Command("cmd", "/c", "start", "", configPath)
	case "darwin":
		// macOS: 使用 open 命令
		cmd = exec.Command("open", configPath)
	default:
		// Linux: 尝试使用 xdg-open，如果失败则尝试其他编辑器
		cmd = exec.Command("xdg-open", configPath)
	}

	if err := cmd.Start(); err != nil {
		dialog.ShowError(fmt.Errorf("打开配置文件失败: %w", err), ui.window)
		return
	}

	// 文件打开后，等待一段时间后刷新显示并验证
	go func() {
		// 等待用户编辑文件（给一些时间）
		time.Sleep(2 * time.Second)
		// 持续监控文件修改，直到文件不再变化
		var lastModTime time.Time
		for i := 0; i < 10; i++ { // 最多检查10次
			time.Sleep(1 * time.Second)
			info, err := os.Stat(configPath)
			if err != nil {
				continue
			}
			if info.ModTime().After(lastModTime) {
				lastModTime = info.ModTime()
				// 文件被修改了，继续等待
				continue
			}
			// 文件不再变化，刷新显示并验证
			break
		}
		fyne.Do(func() {
			ui.updateConfigDisplayWithValidation(true) // 配置修改后验证
		})
	}()
}

// createLogArea 创建日志区域
func (ui *MainUI) createLogArea() fyne.CanvasObject {
	// 创建日志显示区域
	ui.logText = widget.NewRichText()
	ui.logText.Wrapping = fyne.TextWrapOff // 禁用自动换行，保持日志在同一行
	ui.logViewer.SetLogTextWidget(ui.logText)

	// 日志级别选择
	levelSelect := widget.NewSelect([]string{"ALL", "INFO", "WARN", "ERROR", "DEBUG"}, func(level string) {
		ui.logViewer.SetLogLevelFilter(level)
	})
	levelSelect.SetSelected("ALL")

	// 工具栏按钮（使用图标）
	ui.logClearBtn = widget.NewButton("🗑️ 清空", func() {
		ui.logViewer.ClearLogs()
	})
	ui.logClearBtn.Importance = widget.MediumImportance

	ui.autoScrollCheck = widget.NewCheck("自动滚动", func(checked bool) {
		ui.logViewer.SetAutoScroll(checked)
	})
	ui.autoScrollCheck.SetChecked(true)

	// 工具栏（水平布局，使用卡片包装）
	toolbar := container.NewHBox(
		widget.NewLabel("📊 级别:"),
		levelSelect,
		widget.NewSeparator(),
		ui.logClearBtn,
		widget.NewSeparator(),
		ui.autoScrollCheck,
	)
	toolbarCard := widget.NewCard("", "", container.NewPadded(toolbar))

	// 日志显示区域（可滚动，紧凑布局）
	logScroll := container.NewScroll(ui.logText)
	logScroll.SetMinSize(fyne.NewSize(0, 300))
	// 设置滚动方向为双向
	logScroll.Direction = container.ScrollBoth

	// 将滚动容器传递给LogViewer，用于自动滚动
	ui.logViewer.SetLogScrollContainer(logScroll)

	// 在后台加载日志文件（避免阻塞界面创建）
	go func() {
		// 尝试从文件加载日志
		ui.loadLogsFromFile()

		// 启动文件监控（如果文件存在）
		logFilePath := filepath.Join(ui.deployDir, "logs", "gridbot.log")
		if _, err := os.Stat(logFilePath); err == nil {
			ui.logViewer.WatchLogFile(logFilePath, func(line string) {
				// 尝试解析日志级别，默认为INFO
				level := "INFO"
				if strings.Contains(line, "ERROR") || strings.Contains(line, "error") {
					level = "ERROR"
				} else if strings.Contains(line, "WARN") || strings.Contains(line, "warn") {
					level = "WARN"
				} else if strings.Contains(line, "DEBUG") || strings.Contains(line, "debug") {
					level = "DEBUG"
				}
				// 使用fyne.Do确保在主线程中调用AppendLog
				fyne.Do(func() {
					ui.logViewer.AppendLog(level, line)
				})
			})
		}
	}()

	// 整体布局：使用卡片包装整个日志区域
	logContent := container.NewBorder(
		container.NewPadded(toolbarCard), // 顶部工具栏
		nil,                              // 底部
		nil,                              // 左侧
		nil,                              // 右侧
		logScroll,                        // 中间日志显示
	)

	logCard := widget.NewCard("📋 运行日志", "", container.NewPadded(logContent))

	return container.NewPadded(logCard)
}

// updateStatus 更新状态显示
func (ui *MainUI) updateStatus() {
	// 检查运行状态（在非GUI线程中执行）
	isRunning := ui.processManager.IsRunning()

	// 使用fyne.Do确保GUI操作在主线程中执行
	fyne.Do(func() {
		if ui.statusLabel != nil {
			if isRunning {
				ui.statusLabel.SetText("🟢 状态: 运行中")
				ui.statusLabel.Importance = widget.HighImportance
			} else {
				ui.statusLabel.SetText("🔴 状态: 已停止")
				ui.statusLabel.Importance = widget.MediumImportance
			}
		}
		ui.updateButtonStates()
	})
}

// updateButtonStates 更新按钮状态
// 注意：此方法应该在fyne.Do()中调用，确保在主线程中执行
func (ui *MainUI) updateButtonStates() {
	// 检查按钮是否已初始化
	if ui.startBtn == nil || ui.stopBtn == nil {
		return
	}

	// 检查运行状态
	isRunning := ui.processManager.IsRunning()

	// 直接更新按钮状态（已经在fyne.Do()中调用）
	if !isRunning {
		ui.startBtn.Enable()
		ui.stopBtn.Disable()
	} else {
		ui.startBtn.Disable()
		ui.stopBtn.Enable()
	}
}

// detectCurrentVersion 检测当前版本
func (ui *MainUI) detectCurrentVersion() {
	// 尝试从可执行文件名中提取版本号
	if ui.exePath != "" {
		exeName := filepath.Base(ui.exePath)

		// 文件名格式可能是: sol-grid-bot-v1.0.0-windows-amd64.exe
		// 或者: sol-grid-bot-windows-amd64.exe (无版本号)
		if strings.Contains(exeName, "sol-grid-bot-") {
			// 移除 .exe 扩展名
			nameWithoutExt := strings.TrimSuffix(exeName, ".exe")
			// 移除 .tar.gz 等扩展名
			nameWithoutExt = strings.TrimSuffix(nameWithoutExt, ".tar.gz")
			nameWithoutExt = strings.TrimSuffix(nameWithoutExt, ".zip")

			// 分割文件名
			parts := strings.Split(nameWithoutExt, "-")

			// 查找版本号（格式为 v1.0.0 或 1.0.0）
			foundVersion := false
			for _, part := range parts {
				// 检查是否是版本号格式（v开头或纯数字版本号）
				if strings.HasPrefix(part, "v") && len(part) > 1 {
					// 验证是否是有效的版本号格式（v1.0.0）
					if strings.Count(part, ".") >= 1 || len(part) >= 4 {
						ui.currentVersion = part
						foundVersion = true
						break
					}
				} else if strings.Count(part, ".") >= 1 && len(part) >= 3 {
					// 可能是纯数字版本号（1.0.0）
					ui.currentVersion = "v" + part
					foundVersion = true
					break
				}
			}

			if !foundVersion {
				// 如果文件名是 sol-grid-bot.exe 或 sol-grid-bot-windows-amd64.exe，可能是开发版本
				ui.currentVersion = "开发版"
			}
		} else if exeName == "sol-grid-bot.exe" || exeName == "sol-grid-bot" {
			// 简单的文件名，可能是开发版本
			ui.currentVersion = "开发版"
		} else {
			// 其他情况，显示文件名
			ui.currentVersion = "未知"
		}
	} else {
		ui.currentVersion = "未安装"
	}
}

// autoDownloadLatestVersion 自动下载最新版本（当可执行文件不存在时）
func (ui *MainUI) autoDownloadLatestVersion() {
	// 更新状态显示
	fyne.Do(func() {
		if ui.statusLabel != nil {
			ui.statusLabel.SetText("⏬ 正在下载最新版本...")
		}
		if ui.currentVersionLabel != nil {
			ui.currentVersionLabel.SetText("📦 当前版本: 下载中...")
		}
	})

	ctx := context.Background()

	// 获取最新release版本
	release, err := ui.downloader.GetLatestRelease(ctx)
	if err != nil {
		fyne.Do(func() {
			if ui.statusLabel != nil {
				ui.statusLabel.SetText("❌ 下载失败")
			}
			if ui.currentVersionLabel != nil {
				ui.currentVersionLabel.SetText("📦 当前版本: 下载失败")
			}
			dialog.ShowError(fmt.Errorf("获取最新版本失败: %w", err), ui.window)
		})
		return
	}

	// 获取适合当前平台的文件
	asset, err := ui.downloader.GetAssetForCurrentPlatform(release)
	if err != nil {
		fyne.Do(func() {
			if ui.statusLabel != nil {
				ui.statusLabel.SetText("❌ 下载失败")
			}
			if ui.currentVersionLabel != nil {
				ui.currentVersionLabel.SetText("📦 当前版本: 下载失败")
			}
			dialog.ShowError(fmt.Errorf("未找到适合当前平台的文件: %w", err), ui.window)
		})
		return
	}

	// 下载文件
	fyne.Do(func() {
		if ui.statusLabel != nil {
			ui.statusLabel.SetText(fmt.Sprintf("⏬ 正在下载 %s...", asset.Name))
		}
	})

	archivePath, err := ui.downloader.DownloadAsset(ctx, asset, ui.deployDir, func(current, total int64) {
		// 下载进度回调（可选，不显示进度条）
	})
	if err != nil {
		fyne.Do(func() {
			if ui.statusLabel != nil {
				ui.statusLabel.SetText("❌ 下载失败")
			}
			if ui.currentVersionLabel != nil {
				ui.currentVersionLabel.SetText("📦 当前版本: 下载失败")
			}
			dialog.ShowError(fmt.Errorf("下载失败: %w", err), ui.window)
		})
		return
	}

	// 解压文件
	fyne.Do(func() {
		if ui.statusLabel != nil {
			ui.statusLabel.SetText("📦 正在解压文件...")
		}
	})

	exePath, err := ui.downloader.ExtractFile(archivePath, ui.deployDir)
	if err != nil {
		fyne.Do(func() {
			if ui.statusLabel != nil {
				ui.statusLabel.SetText("❌ 解压失败")
			}
			if ui.currentVersionLabel != nil {
				ui.currentVersionLabel.SetText("📦 当前版本: 解压失败")
			}
			dialog.ShowError(fmt.Errorf("解压失败: %w", err), ui.window)
		})
		return
	}

	// 更新exePath和版本信息
	ui.exePath = exePath
	ui.currentVersion = release.TagName
	ui.latestVersion = release.TagName

	// 更新UI显示
	fyne.Do(func() {
		if ui.statusLabel != nil {
			ui.statusLabel.SetText("✅ 下载完成")
		}
		if ui.currentVersionLabel != nil {
			ui.currentVersionLabel.SetText(fmt.Sprintf("📦 当前版本: %s", ui.currentVersion))
		}
		if ui.latestVersionLabel != nil {
			ui.latestVersionLabel.SetText(fmt.Sprintf("🔄 最新版本: %s", ui.latestVersion))
		}
		// 延迟恢复状态显示
		go func() {
			time.Sleep(2 * time.Second)
			ui.updateStatus()
		}()
		dialog.ShowInformation("下载完成", fmt.Sprintf("已成功下载并解压版本 %s\n可执行文件: %s", release.TagName, exePath), ui.window)
	})

	// 清理下载的压缩包
	if archivePath != "" {
		os.Remove(archivePath)
	}
}

// checkLatestVersion 检查最新版本
func (ui *MainUI) checkLatestVersion() {
	checkLatestVersionWithDialog := func(showDialog bool) {
		// 使用 fyne.Do 确保在主线程中更新 GUI
		fyne.Do(func() {
			if ui.checkVersionBtn != nil {
				ui.checkVersionBtn.Disable()
				ui.checkVersionBtn.SetText("检查中...")
			}

			if ui.latestVersionLabel != nil {
				ui.latestVersionLabel.SetText("🔄 最新版本: 检查中...")
			}
		})

		go func() {
			ctx := context.Background()
			release, err := ui.downloader.GetLatestRelease(ctx)
			if err != nil {
				fyne.Do(func() {
					ui.latestVersion = "检查失败"
					if ui.latestVersionLabel != nil {
						ui.latestVersionLabel.SetText(fmt.Sprintf("🔄 最新版本: %s", ui.latestVersion))
					}
					if ui.checkVersionBtn != nil {
						ui.checkVersionBtn.SetText("检查更新")
						ui.checkVersionBtn.Enable()
					}
					if showDialog {
						dialog.ShowError(fmt.Errorf("获取最新版本失败: %w", err), ui.window)
					}
				})
				return
			}

			fyne.Do(func() {
				ui.latestVersion = release.TagName
				if ui.latestVersionLabel != nil {
					ui.latestVersionLabel.SetText(fmt.Sprintf("🔄 最新版本: %s", ui.latestVersion))
				}
				if ui.checkVersionBtn != nil {
					ui.checkVersionBtn.SetText("检查更新")
					ui.checkVersionBtn.Enable()
				}

				// 只有在手动点击或发现新版本时才显示对话框
				if showDialog {
					// 如果当前版本不是最新版本，显示提示
					if ui.currentVersion != release.TagName && ui.currentVersion != "开发版" && ui.currentVersion != "未安装" {
						dialog.ShowInformation("版本更新",
							fmt.Sprintf("发现新版本: %s\n当前版本: %s\n\n程序会在启动时自动下载最新版本。",
								release.TagName, ui.currentVersion), ui.window)
					} else if ui.currentVersion == release.TagName {
						dialog.ShowInformation("版本检查", "您已安装最新版本！", ui.window)
					}
				} else {
					// 自动检查时，只在发现新版本时显示提示
					if ui.currentVersion != release.TagName && ui.currentVersion != "开发版" && ui.currentVersion != "未安装" {
						dialog.ShowInformation("版本更新",
							fmt.Sprintf("发现新版本: %s\n当前版本: %s\n\n程序会在启动时自动下载最新版本。",
								release.TagName, ui.currentVersion), ui.window)
					}
				}
			})
		}()
	}

	// 如果是从按钮点击调用，显示对话框
	checkLatestVersionWithDialog(true)
}

// checkLatestVersionSilent 静默检查最新版本（不显示对话框，除非有新版本）
func (ui *MainUI) checkLatestVersionSilent() {
	// 使用 fyne.Do 确保在主线程中更新 GUI
	fyne.Do(func() {
		if ui.checkVersionBtn != nil {
			ui.checkVersionBtn.Disable()
			ui.checkVersionBtn.SetText("检查中...")
		}

		if ui.latestVersionLabel != nil {
			ui.latestVersionLabel.SetText("🔄 最新版本: 检查中...")
		}
	})

	go func() {
		ctx := context.Background()
		release, err := ui.downloader.GetLatestRelease(ctx)
		if err != nil {
			fyne.Do(func() {
				ui.latestVersion = "检查失败"
				if ui.latestVersionLabel != nil {
					ui.latestVersionLabel.SetText(fmt.Sprintf("🔄 最新版本: %s", ui.latestVersion))
				}
				if ui.checkVersionBtn != nil {
					ui.checkVersionBtn.SetText("检查更新")
					ui.checkVersionBtn.Enable()
				}
				// 静默检查失败时不显示错误对话框
			})
			return
		}

		fyne.Do(func() {
			ui.latestVersion = release.TagName
			if ui.latestVersionLabel != nil {
				ui.latestVersionLabel.SetText(fmt.Sprintf("🔄 最新版本: %s", ui.latestVersion))
			}
			if ui.checkVersionBtn != nil {
				ui.checkVersionBtn.SetText("检查更新")
				ui.checkVersionBtn.Enable()
			}

			// 静默检查时，只在发现新版本时显示提示
			if ui.currentVersion != release.TagName && ui.currentVersion != "开发版" && ui.currentVersion != "未安装" {
				dialog.ShowInformation("版本更新",
					fmt.Sprintf("发现新版本: %s\n当前版本: %s\n\n程序会在启动时自动下载最新版本。",
						release.TagName, ui.currentVersion), ui.window)
			}
		})
	}()
}

// startProgram 启动程序
func (ui *MainUI) startProgram() {
	if ui.exePath == "" {
		dialog.ShowError(fmt.Errorf("可执行文件未找到，请先完成安装配置"), ui.window)
		return
	}

	// 检查配置
	if !ui.checkConfigBeforeStart() {
		return
	}

	// 启动程序
	if err := ui.processManager.Start(ui.exePath, ui.deployDir); err != nil {
		dialog.ShowError(fmt.Errorf("启动失败: %w", err), ui.window)
		return
	}

	ui.updateStatus()
	dialog.ShowInformation("启动成功", "程序已启动", ui.window)
}

// stopProgram 停止程序
func (ui *MainUI) stopProgram() {
	if err := ui.processManager.Stop(); err != nil {
		dialog.ShowError(fmt.Errorf("停止失败: %w", err), ui.window)
		return
	}

	ui.updateStatus()
	dialog.ShowInformation("停止成功", "程序已停止", ui.window)
}

// checkConfigBeforeStart 启动前检查配置
func (ui *MainUI) checkConfigBeforeStart() bool {
	cfg, err := config.LoadFromFile(ui.configPath)
	if err != nil {
		dialog.ShowError(fmt.Errorf("配置文件加载失败: %w", err), ui.window)
		return false
	}

	// 检查配置项是否存在
	var missingConfigs []string
	if cfg.Solana.RpcUrl == "" {
		missingConfigs = append(missingConfigs, "Solana RPC URL")
	}
	if cfg.OkxWeb3.Apikey == "" {
		missingConfigs = append(missingConfigs, "OKX API")
	}
	if cfg.TelegramBot.ApiToken == "" {
		missingConfigs = append(missingConfigs, "Telegram Bot")
	}

	if len(missingConfigs) > 0 {
		dialog.ShowError(fmt.Errorf("以下配置项未配置: %s", strings.Join(missingConfigs, ", ")), ui.window)
		return false
	}

	// 检查配置验证状态
	var failedValidations []string
	var unvalidatedConfigs []string

	// 检查Solana RPC URL验证状态
	if status, ok := ui.configValidationStatus["rpc"]; ok {
		if !strings.Contains(status, "✅ 验证成功") {
			failedValidations = append(failedValidations, "Solana RPC URL")
		}
	} else {
		unvalidatedConfigs = append(unvalidatedConfigs, "Solana RPC URL")
	}

	// 检查OKX API验证状态
	if status, ok := ui.configValidationStatus["okx"]; ok {
		if !strings.Contains(status, "✅ 验证成功") {
			failedValidations = append(failedValidations, "OKX API")
		}
	} else {
		unvalidatedConfigs = append(unvalidatedConfigs, "OKX API")
	}

	// 检查Telegram Bot验证状态
	if status, ok := ui.configValidationStatus["telegram"]; ok {
		if !strings.Contains(status, "✅ 验证成功") {
			failedValidations = append(failedValidations, "Telegram Bot")
		}
	} else {
		unvalidatedConfigs = append(unvalidatedConfigs, "Telegram Bot")
	}

	// 如果有验证失败的配置项
	if len(failedValidations) > 0 {
		dialog.ShowError(fmt.Errorf("以下配置项验证失败: %s\n\n请检查配置后重新验证。", strings.Join(failedValidations, ", ")), ui.window)
		return false
	}

	// 如果有未验证的配置项
	if len(unvalidatedConfigs) > 0 {
		dialog.ShowError(fmt.Errorf("以下配置项未验证: %s\n\n请点击\"🔍 验证配置\"按钮进行验证。", strings.Join(unvalidatedConfigs, ", ")), ui.window)
		return false
	}

	return true
}

// loadLogsFromFile 从文件加载日志
func (ui *MainUI) loadLogsFromFile() {
	logFilePath := filepath.Join(ui.deployDir, "logs", "gridbot.log")
	if err := ui.logViewer.LoadLogsFromFile(logFilePath); err != nil {
		// 文件不存在或读取失败，不显示错误
	}
}

// startStatusChecker 启动状态检查器
func (ui *MainUI) startStatusChecker() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// 启动更新处理器
	go ui.updateProcessor()

	for range ticker.C {
		// 非阻塞发送更新请求
		select {
		case ui.updateChan <- struct{}{}:
		default:
			// 通道已满，跳过本次更新
		}
	}
}

// updateProcessor 处理更新请求
func (ui *MainUI) updateProcessor() {
	for range ui.updateChan {
		// 使用 fyne.Do 确保在主 GUI 线程中执行
		fyne.Do(func() {
			ui.updateStatus()
			ui.updateConfigDisplay()
		})
	}
}

// Show 显示界面
func (ui *MainUI) Show() {
	ui.window.ShowAndRun()
}

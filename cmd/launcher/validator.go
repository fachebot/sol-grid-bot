package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/fachebot/sol-grid-bot/internal/dexagg/okxweb3"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Validator 配置验证器
type Validator struct{}

// ValidateSolanaRPC 验证Solana RPC URL
func (v *Validator) ValidateSolanaRPC(rpcUrl string) error {
	ctx := context.Background()

	// 创建Solana RPC客户端
	opts := &jsonrpc.RPCClientOpts{}
	rpcClient := rpc.NewWithCustomRPCClient(jsonrpc.NewClientWithOpts(rpcUrl, opts))

	// 尝试调用 GetHealth 或 GetVersion 来验证连接
	_, err := rpcClient.GetHealth(ctx)
	if err != nil {
		// 如果 GetHealth 失败，尝试 GetVersion
		_, err = rpcClient.GetVersion(ctx)
		if err != nil {
			return fmt.Errorf("无法连接到Solana RPC服务器: %w", err)
		}
	}

	// 关闭连接
	if err := rpcClient.Close(); err != nil {
		// 忽略关闭错误，因为我们已经验证了连接
	}

	return nil
}

// ValidateOKX 验证OKX API密钥
func (v *Validator) ValidateOKX(apikey, secretkey, passphrase string) error {
	ctx := context.Background()

	// 创建OKX客户端（使用dexagg包中的客户端）
	client := okxweb3.NewClient(apikey, secretkey, passphrase, nil)

	// 调用简单API验证
	_, err := client.GetSupportedChains(ctx)
	if err != nil {
		return fmt.Errorf("OKX API验证失败: %w", err)
	}

	return nil
}

// TelegramValidationResult Telegram验证结果
type TelegramValidationResult struct {
	Username string
	Error    error
}

// ValidateTelegram 验证Telegram Bot Token
func (v *Validator) ValidateTelegram(token string) error {
	result := v.ValidateTelegramWithResult(token)
	return result.Error
}

// ValidateTelegramWithResult 验证Telegram Bot Token并返回bot信息
func (v *Validator) ValidateTelegramWithResult(token string) TelegramValidationResult {
	// 创建Telegram Bot API客户端
	botApi, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return TelegramValidationResult{
			Error: fmt.Errorf("无法创建Telegram Bot客户端: %w", err),
		}
	}

	// 验证token并获取bot信息
	bot, err := botApi.GetMe()
	if err != nil {
		return TelegramValidationResult{
			Error: fmt.Errorf("Telegram Bot Token验证失败: %w", err),
		}
	}

	return TelegramValidationResult{
		Username: bot.UserName,
		Error:    nil,
	}
}

// OpenURL 在默认浏览器中打开URL
func OpenURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

package wallethandler

import (
	"context"
	"fmt"
	"math/big"

	"github.com/fachebot/sol-grid-bot/internal/ent"
	"github.com/fachebot/sol-grid-bot/internal/svc"
	"github.com/fachebot/sol-grid-bot/internal/utils"
	"github.com/fachebot/sol-grid-bot/internal/utils/solanautil"

	"github.com/gagliardetto/solana-go"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func GetUserWallet(ctx context.Context, svcCtx *svc.ServiceContext, userId int64) (*ent.Wallet, error) {
	w, err := svcCtx.WalletModel.FindByUserId(ctx, userId)
	if err != nil {
		if !ent.IsNotFound(err) {
			return nil, err
		}

		privateKey := solana.NewWallet().PrivateKey
		pk, err := svcCtx.HashEncoder.Encryption(privateKey.String())
		if err != nil {
			return nil, err
		}

		args := ent.Wallet{
			UserId:     userId,
			Account:    privateKey.PublicKey().String(),
			PrivateKey: pk,
		}
		w, err = svcCtx.WalletModel.Save(ctx, args)
		if err != nil {
			return nil, err
		}
	}

	return w, nil
}

func DisplayWalletMenu(ctx context.Context, svcCtx *svc.ServiceContext, botApi *tgbotapi.BotAPI, userId int64, update tgbotapi.Update) error {
	// 确保生成账户
	w, err := GetUserWallet(ctx, svcCtx, userId)
	if err != nil {
		return err
	}

	// 查询账户余额
	balance, err := solanautil.GetBalance(ctx, svcCtx.SolanaRpc, w.Account)
	if err != nil {
		balance = big.NewInt(0)
	}

	// 查询USDC余额
	usdcBalance, decimals, err := solanautil.GetTokenBalance(ctx, svcCtx.SolanaRpc, solanautil.USDC, w.Account)
	if err != nil {
		usdcBalance = big.NewInt(0)
	}

	// 回复钱包菜单
	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ 返回", "/home"),
			tgbotapi.NewInlineKeyboardButtonData("刷新余额", WalletHomeHandler{}.FormatPath()),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚠️ 导出钱包私钥", KeyExportHandler{}.FormatPath(w.Account)),
		),
	)
	text := fmt.Sprintf("Solana 网格机器人 | 钱包管理\n\n💳 我的钱包:\n`%s`\n\n💰    SOL余额: `%s`\n💰 USDC余额: `%s`",
		w.Account, solanautil.ParseSOL(balance).Truncate(5), solanautil.ParseUnits(usdcBalance, decimals).Truncate(5))
	text = text + fmt.Sprintf("\n\n[OKX](https://web3.okx.com/zh-hant/portfolio/%s/analysis?chainIndex=501) | [GMGN](https://gmgn.ai/sol/address/%s) | [Solscan](https://solscan.io/account/%s)", w.Account, w.Account, w.Account)
	_, err = utils.ReplyMessage(botApi, update, text, markup)
	return err
}

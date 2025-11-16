package swap

import (
	"context"
	"errors"
	"math/big"

	"github.com/fachebot/sol-grid-bot/internal/dexagg/jupiter"
	"github.com/fachebot/sol-grid-bot/internal/dexagg/okxweb3"
	"github.com/fachebot/sol-grid-bot/internal/dexagg/relaylink"
	"github.com/fachebot/sol-grid-bot/internal/ent"
	"github.com/fachebot/sol-grid-bot/internal/ent/settings"
	"github.com/fachebot/sol-grid-bot/internal/logger"
	"github.com/fachebot/sol-grid-bot/internal/svc"
	"github.com/fachebot/sol-grid-bot/internal/utils/solanautil"

	"github.com/gagliardetto/solana-go"
)

type SwapService struct {
	svcCtx   *svc.ServiceContext
	userId   int64
	wallet   *solana.Wallet
	settings *ent.Settings
}

func NewSwapService(svcCtx *svc.ServiceContext, userId int64) *SwapService {
	return &SwapService{svcCtx: svcCtx, userId: userId}
}

func (s *SwapService) Quote(ctx context.Context, inputToken, outputToken string, amount *big.Int, exit ...bool) (SwapTransaction, error) {
	userWallet, err := s.getUserWallet(ctx)
	if err != nil {
		return nil, err
	}

	userSettings, err := s.getUserSettings(ctx)
	if err != nil {
		return nil, err
	}

	slippageBps := int(userSettings.SlippageBps)
	if outputToken == solanautil.USDC && userSettings.SellSlippageBps != nil {
		slippageBps = int(*userSettings.SellSlippageBps)
		if len(exit) > 0 && exit[0] && userSettings.ExitSlippageBps != nil {
			// 如果是清仓交易，使用清仓滑点
			slippageBps = int(*userSettings.ExitSlippageBps)
		}
	}

	switch userSettings.DexAggregator {
	case settings.DexAggregatorOkx:
		okxClient := okxweb3.NewClient(
			s.svcCtx.Config.OkxWeb3.Apikey,
			s.svcCtx.Config.OkxWeb3.Secretkey,
			s.svcCtx.Config.OkxWeb3.Passphrase,
			s.svcCtx.TransportProxy,
		)
		quoteResponse, err := okxClient.Quote(
			ctx,
			okxweb3.SolanaChainIndex,
			userWallet.PublicKey().String(),
			inputToken,
			outputToken,
			amount,
			slippageBps,
		)
		if err != nil {
			return nil, err
		}
		return NewOkxSwapTransaction(s, quoteResponse, userWallet.PublicKey().String()), nil
	case settings.DexAggregatorJup:
		jupConf := s.svcCtx.Config.Jupiter
		jupClient := jupiter.NewJupiterClient(jupConf.Url, jupConf.Apikey, s.svcCtx.TransportProxy)
		quoteResponse, err := jupClient.Quote(ctx, inputToken, outputToken, amount, slippageBps)
		if err != nil {
			return nil, err
		}
		return NewJupSwapTransaction(s, quoteResponse, userWallet.PublicKey().String()), nil
	case settings.DexAggregatorRelay:
		user := userWallet.PublicKey().String()
		relaylinkClient := relaylink.NewRelaylinkClient(s.svcCtx.TransportProxy)
		quoteResponse, err := relaylinkClient.Quote(ctx, relaylink.SolanaChainID, user, inputToken, outputToken, amount, slippageBps)
		if err != nil {
			return nil, err
		}
		return NewRelaySwapTransaction(s, quoteResponse, userWallet.PublicKey().String()), nil
	default:
		return nil, errors.New("unsupported aggregator")
	}

}

func (s *SwapService) getUserWallet(ctx context.Context) (*solana.Wallet, error) {
	if s.wallet != nil {
		return s.wallet, nil
	}

	w, err := s.svcCtx.WalletModel.FindByUserId(ctx, s.userId)
	if err != nil {
		logger.Errorf("[SwapService] 查询用户钱包失败, userId: %d, %v", s.userId, err)
		return nil, err
	}

	pk, err := s.svcCtx.HashEncoder.Decryption(w.PrivateKey)
	if err != nil {
		logger.Errorf("[SwapService] 解密用户私钥失败, userId: %d, %v", s.userId, err)
		return nil, err
	}

	wallet, err := solana.WalletFromPrivateKeyBase58(pk)
	if err != nil {
		logger.Errorf("[SwapService] 解析用户私钥失败, userId: %d, %v", s.userId, err)
		return nil, err
	}

	s.wallet = wallet

	return wallet, nil
}

func (s *SwapService) getUserSettings(ctx context.Context) (*ent.Settings, error) {
	if s.settings != nil {
		return s.settings, nil
	}

	v, err := s.svcCtx.SettingsModel.FindByUserId(ctx, s.userId)
	if err == nil {
		s.settings = v
		return v, nil
	}

	if !ent.IsNotFound(err) {
		logger.Errorf("[SwapService] 查询用户设置失败, userId: %d, %v", s.userId, err)
		return nil, err
	}

	c := s.svcCtx.Config.Solana
	ret := ent.Settings{
		UserId:        s.userId,
		MaxRetries:    int64(c.MaxRetries),
		SlippageBps:   c.SlippageBps,
		MaxLamports:   int64(c.MaxLamports),
		PriorityLevel: settings.PriorityLevel(c.PriorityLevel),
		DexAggregator: settings.DexAggregator(c.DexAggregator),
	}

	s.settings = &ret

	return &ret, nil
}

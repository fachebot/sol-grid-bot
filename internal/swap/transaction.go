package swap

import (
	"context"
	"math/big"

	"github.com/fachebot/sol-grid-bot/internal/dexagg/jupiter"
	"github.com/fachebot/sol-grid-bot/internal/dexagg/okxweb3"
	"github.com/fachebot/sol-grid-bot/internal/dexagg/relaylink"

	"github.com/shopspring/decimal"
)

type SwapTransaction interface {
	Signer() string
	OutAmount() *big.Int
	SlippageBps() int
	Swap(ctx context.Context) (string, error)
}

type OkxSwapTransaction struct {
	quote   *okxweb3.SwapInstruction
	service *SwapService
	signer  string
}

func NewOkxSwapTransaction(service *SwapService, quoteResponse *okxweb3.SwapInstruction, signer string) *OkxSwapTransaction {
	return &OkxSwapTransaction{
		quote:   quoteResponse,
		service: service,
		signer:  signer,
	}
}

func (tx *OkxSwapTransaction) Signer() string {
	return tx.signer
}

func (tx *OkxSwapTransaction) OutAmount() *big.Int {
	return tx.quote.RouterResult.ToTokenAmount.BigInt()
}

func (tx *OkxSwapTransaction) SlippageBps() int {
	return int(tx.quote.Tx.Slippage.Mul(decimal.NewFromInt(10000)).RoundUp(0).IntPart())
}

func (tx *OkxSwapTransaction) Swap(ctx context.Context) (string, error) {
	userWallet, err := tx.service.getUserWallet(ctx)
	if err != nil {
		return "", err
	}

	userSettings, err := tx.service.getUserSettings(ctx)
	if err != nil {
		return "", err
	}

	svcCtx := tx.service.svcCtx
	okxClient := okxweb3.NewClient(
		svcCtx.Config.OkxWeb3.Apikey,
		svcCtx.Config.OkxWeb3.Secretkey,
		svcCtx.Config.OkxWeb3.Passphrase,
		svcCtx.TransportProxy,
	)
	return okxClient.SendSwapTransaction(ctx, svcCtx, userWallet, tx.quote, userSettings.PriorityLevel, uint(userSettings.MaxRetries))
}

type JupSwapTransaction struct {
	quote   *jupiter.QuoteResponse
	service *SwapService
	signer  string
}

func NewJupSwapTransaction(service *SwapService, quoteResponse *jupiter.QuoteResponse, signer string) *JupSwapTransaction {
	return &JupSwapTransaction{
		quote:   quoteResponse,
		service: service,
		signer:  signer,
	}
}

func (tx *JupSwapTransaction) Signer() string {
	return tx.signer
}

func (tx *JupSwapTransaction) OutAmount() *big.Int {
	return tx.quote.OutAmount.BigInt()
}

func (tx *JupSwapTransaction) SlippageBps() int {
	return tx.quote.SlippageBps
}

func (tx *JupSwapTransaction) Swap(ctx context.Context) (string, error) {
	userWallet, err := tx.service.getUserWallet(ctx)
	if err != nil {
		return "", err
	}

	userSettings, err := tx.service.getUserSettings(ctx)
	if err != nil {
		return "", err
	}

	jupConf := tx.service.svcCtx.Config.Jupiter
	jupClient := jupiter.NewJupiterClient(jupConf.Url, jupConf.Apikey, tx.service.svcCtx.TransportProxy)
	swapResponse, err := jupClient.Swap(ctx, userWallet.PublicKey().String(), tx.quote, jupiter.PriorityLevel(userSettings.PriorityLevel), userSettings.MaxLamports)
	if err != nil {
		return "", err
	}

	return jupClient.SendSwapTransaction(ctx, tx.service.svcCtx, userWallet, swapResponse, uint(userSettings.MaxRetries))
}

type RelaySwapTransaction struct {
	quote   *relaylink.QuoteResponse
	service *SwapService
	signer  string
}

func NewRelaySwapTransaction(service *SwapService, quoteResponse *relaylink.QuoteResponse, signer string) *RelaySwapTransaction {
	return &RelaySwapTransaction{
		quote:   quoteResponse,
		service: service,
		signer:  signer,
	}
}

func (tx *RelaySwapTransaction) Signer() string {
	return tx.signer
}

func (tx *RelaySwapTransaction) OutAmount() *big.Int {
	return tx.quote.Details.CurrencyOut.Amount.BigInt()
}

func (tx *RelaySwapTransaction) SlippageBps() int {
	return int(tx.quote.Details.SlippageTolerance.Origin.Percent.RoundUp(0).IntPart())
}

func (tx *RelaySwapTransaction) Swap(ctx context.Context) (string, error) {
	userWallet, err := tx.service.getUserWallet(ctx)
	if err != nil {
		return "", err
	}

	userSettings, err := tx.service.getUserSettings(ctx)
	if err != nil {
		return "", err
	}

	relayClient := relaylink.NewRelaylinkClient(tx.service.svcCtx.TransportProxy)
	return relayClient.SendSwapTransaction(ctx, tx.service.svcCtx, userWallet, tx.quote, userSettings.PriorityLevel, uint(userSettings.MaxRetries))
}

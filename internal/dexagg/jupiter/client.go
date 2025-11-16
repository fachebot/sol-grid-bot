package jupiter

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"

	"github.com/fachebot/sol-grid-bot/internal/svc"
	"github.com/fachebot/sol-grid-bot/internal/utils/solanautil"

	"github.com/carlmjohnson/requests"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

const (
	WrappedSOL = "So11111111111111111111111111111111111111112"
)

type JupiterClient struct {
	apiUrl         string
	apiKey         string
	transportProxy *http.Transport
}

func NewJupiterClient(apiUrl, apiKey string, transportProxy *http.Transport) *JupiterClient {
	client := &JupiterClient{
		apiUrl:         apiUrl,
		apiKey:         apiKey,
		transportProxy: transportProxy,
	}
	return client
}

func (client *JupiterClient) TokenStats(ctx context.Context, tokenAddress string) (*TokenInfo, error) {
	httpClient := new(http.Client)
	if client.transportProxy != nil {
		httpClient.Transport = client.transportProxy
	}

	var errRes *ErrorResponse
	var response []TokenInfo
	url := fmt.Sprintf("%s/tokens/v2/search?query=%s", client.apiUrl, tokenAddress)
	err := requests.URL(url).
		Client(httpClient).
		Header("x-api-key", client.apiKey).
		ErrorJSON(&errRes).
		ToJSON(&response).
		Fetch(ctx)
	if err != nil {
		if errRes != nil {
			return nil, errRes
		}
		return nil, err
	}

	if len(response) == 0 {
		return nil, errors.New("token not found")
	}

	stats := &response[0]
	if stats.FDV.IsZero() {
		stats.FDV = stats.UsdPrice.Mul(stats.TotalSupply)
	}
	if stats.MCap.IsZero() {
		stats.MCap = stats.UsdPrice.Mul(stats.CircSupply)
	}

	return stats, nil
}

func (client *JupiterClient) Quote(ctx context.Context, inputToken, outputToken string, amount *big.Int, slippageBps int) (*QuoteResponse, error) {
	params := make(url.Values)
	params.Set("inputMint", inputToken)
	params.Set("outputMint", outputToken)
	params.Set("amount", amount.String())
	params.Set("slippageBps", strconv.Itoa(slippageBps))
	params.Set("swapMode", "ExactIn")

	httpClient := new(http.Client)
	if client.transportProxy != nil {
		httpClient.Transport = client.transportProxy
	}

	var errRes *ErrorResponse
	var response QuoteResponse
	err := requests.URL(fmt.Sprintf("%s/swap/v1/quote?%s", client.apiUrl, params.Encode())).
		Client(httpClient).
		Header("x-api-key", client.apiKey).
		ErrorJSON(&errRes).
		ToJSON(&response).
		Fetch(ctx)
	if err != nil {
		if errRes != nil {
			return nil, errRes
		}
		return nil, err
	}
	return &response, nil
}

func (client *JupiterClient) Swap(ctx context.Context, userWalletAddress string, quote *QuoteResponse, priorityLevel PriorityLevel, maxLamports int64) (*SwapResponse, error) {
	payload := SwapRequest{
		UserPublicKey: userWalletAddress,
		QuoteResponse: *quote,
		PrioritizationFeeLamports: PrioritizationFeeLamports{
			PriorityLevelWithMaxLamports: &PriorityLevelWithMaxLamports{
				MaxLamports:   maxLamports,
				PriorityLevel: priorityLevel,
			},
		},
		DynamicComputeUnitLimit: true,
	}

	httpClient := new(http.Client)
	if client.transportProxy != nil {
		httpClient.Transport = client.transportProxy
	}

	var errRes *ErrorResponse
	var response SwapResponse
	err := requests.URL(fmt.Sprintf("%s/swap/v1/swap", client.apiUrl)).
		Client(httpClient).
		Header("x-api-key", client.apiKey).
		Post().
		BodyJSON(payload).
		ErrorJSON(&errRes).
		ToJSON(&response).
		Fetch(ctx)
	if err != nil {
		if errRes != nil {
			return nil, errRes
		}
		return nil, err
	}

	if response.SimulationError != nil {
		return nil, fmt.Errorf("code: %s, msg: %s", response.SimulationError.ErrorCode, response.SimulationError.Error)
	}

	return &response, nil
}

func (client *JupiterClient) SendSwapTransaction(ctx context.Context, svcCtx *svc.ServiceContext, wallet *solana.Wallet, swapResponse *SwapResponse, maxRetries uint) (string, error) {
	latestBlockhash, err := svcCtx.SolanaRpc.GetLatestBlockhash(ctx, "")
	if err != nil {
		return "", fmt.Errorf("could not get latest blockhash: %w", err)
	}

	signedTx, err := solanautil.NewTransactionFromBase64(swapResponse.SwapTransaction)
	if err != nil {
		return "", fmt.Errorf("could not deserialize swap transaction: %w", err)
	}
	signedTx.Message.RecentBlockhash = latestBlockhash.Value.Blockhash

	tx, hash, err := solanautil.SignTransaction(wallet, &signedTx)
	if err != nil {
		return "", fmt.Errorf("could not sign swap transaction: %w", err)
	}

	sig, err := svcCtx.SolanaRpc.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{
		MaxRetries:          &maxRetries,
		MinContextSlot:      &latestBlockhash.Context.Slot,
		SkipPreflight:       true,
		PreflightCommitment: rpc.CommitmentProcessed,
	})
	if err != nil {
		return hash, fmt.Errorf("could not send transaction: %w", err)
	}

	return sig.String(), nil
}

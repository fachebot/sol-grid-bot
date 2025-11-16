package relaylink

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"

	"github.com/fachebot/sol-grid-bot/internal/ent/settings"
	"github.com/fachebot/sol-grid-bot/internal/svc"
	"github.com/fachebot/sol-grid-bot/internal/utils/solanautil"

	"github.com/carlmjohnson/requests"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
)

const (
	BscChainID    = 56
	SolanaChainID = 792703809
)

var (
	SOL = "11111111111111111111111111111111"
	BSC = common.HexToAddress("0x0000000000000000000000000000000000000000")
	ETH = common.HexToAddress("0x0000000000000000000000000000000000000000")
)

type RelaylinkClient struct {
	transportProxy   *http.Transport
	chainsCache      map[int64]Chain
	chainsCacheMutex sync.Mutex
}

func NewRelaylinkClient(transportProxy *http.Transport) *RelaylinkClient {
	client := &RelaylinkClient{
		transportProxy: transportProxy,
	}
	return client
}

func (client *RelaylinkClient) GetChains(ctx context.Context) ([]Chain, error) {
	httpClient := new(http.Client)
	if client.transportProxy != nil {
		httpClient.Transport = client.transportProxy
	}

	var chains Chains
	err := requests.URL("https://api.relay.link/chains").
		Client(httpClient).
		ToJSON(&chains).
		Fetch(ctx)

	return chains.Chains, err
}

func (client *RelaylinkClient) GetChainsByID(ctx context.Context, chainId int64) (Chain, bool) {
	client.chainsCacheMutex.Lock()
	defer client.chainsCacheMutex.Unlock()

	if client.chainsCache == nil {
		chains, err := client.GetChains(ctx)
		if err != nil {
			return Chain{}, false
		}

		client.chainsCache = make(map[int64]Chain)
		for _, chain := range chains {
			client.chainsCache[chain.ID] = chain
		}
	}

	chain, ok := client.chainsCache[chainId]
	return chain, ok
}

func (client *RelaylinkClient) Quote(ctx context.Context, chainId int64, user, inputToken, outputToken string, amount *big.Int, slippageBps int) (*QuoteResponse, error) {
	chain, ok := client.GetChainsByID(ctx, chainId)
	if !ok {
		return nil, errors.New("unsupported chain")
	}

	params := map[string]any{
		"user":                user,
		"originChainId":       chain.ID,
		"destinationChainId":  chain.ID,
		"originCurrency":      inputToken,
		"destinationCurrency": outputToken,
		"amount":              amount.String(),
		"tradeType":           "EXACT_INPUT",
		"slippageTolerance":   slippageBps,
	}

	httpClient := new(http.Client)
	if client.transportProxy != nil {
		httpClient.Transport = client.transportProxy
	}

	var errRes *ErrorResponse
	var response QuoteResponse
	err := requests.URL("https://api.relay.link/quote").
		Method(http.MethodPost).
		Client(httpClient).
		BodyJSON(params).
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

func (client *RelaylinkClient) SendSwapTransaction(ctx context.Context, svcCtx *svc.ServiceContext, wallet *solana.Wallet, swapResponse *QuoteResponse, priorityLevel settings.PriorityLevel, maxRetries uint) (string, error) {

	latestBlockhash, err := svcCtx.SolanaRpc.GetLatestBlockhash(ctx, "")
	if err != nil {
		return "", fmt.Errorf("could not get latest blockhash: %w", err)
	}

	if len(swapResponse.Steps) != 1 ||
		len(swapResponse.Steps[0].Items) != 1 ||
		len(swapResponse.Steps[0].Items[0].Data.SolInstructions) == 0 {
		return "", errors.New("instructions not found")
	}

	// 获取优先费
	priorityFee, err := getPriorityFeeByLevel(ctx, priorityLevel)
	if err != nil {
		return "", err
	}
	setComputeUnitPriceIx := computebudget.NewSetComputeUnitPriceInstruction(
		priorityFee,
	).Build()

	// 创建交易指令
	instructions := []solana.Instruction{setComputeUnitPriceIx}
	solInstructions := swapResponse.Steps[0].Items[0].Data.SolInstructions
	for _, instrData := range solInstructions {
		var accounts []*solana.AccountMeta
		for _, key := range instrData.Keys {
			accounts = append(accounts, &solana.AccountMeta{
				PublicKey:  solana.MustPublicKeyFromBase58(key.Pubkey),
				IsSigner:   key.IsSigner,
				IsWritable: key.IsWritable,
			})
		}

		data, err := hex.DecodeString(instrData.Data)
		if err != nil {
			return "", err
		}

		instruction := solana.NewInstruction(
			solana.MustPublicKeyFromBase58(instrData.ProgramId),
			accounts,
			data,
		)

		instructions = append(instructions, instruction)
	}

	tables, err := svcCtx.LookuptableCache.GetAddressLookupTables(
		ctx, swapResponse.Steps[0].Items[0].Data.SolAddressLookupTableAddresses)
	if err != nil {
		return "", err
	}

	// 创建交易
	tx, err := solana.NewTransaction(
		instructions,
		latestBlockhash.Value.Blockhash,
		solana.TransactionAddressTables(tables),
		solana.TransactionPayer(wallet.PublicKey()),
	)
	if err != nil {
		return "", fmt.Errorf("could not deserialize swap transaction: %w", err)
	}

	// 签名交易
	signedTx, hash, err := solanautil.SignTransaction(wallet, tx)
	if err != nil {
		return "", fmt.Errorf("could not sign swap transaction: %w", err)
	}

	// 发送交易
	sig, err := svcCtx.SolanaRpc.SendTransactionWithOpts(ctx, signedTx, rpc.TransactionOpts{
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

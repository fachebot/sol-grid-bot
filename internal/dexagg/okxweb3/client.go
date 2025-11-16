package okxweb3

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/fachebot/sol-grid-bot/internal/ent/settings"
	"github.com/fachebot/sol-grid-bot/internal/svc"
	"github.com/fachebot/sol-grid-bot/internal/utils/solanautil"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

const (
	SolanaChainIndex = "501"
)

type Client struct {
	apiKey     string
	secretKey  []byte
	passphrase string
	client     *http.Client
}

func NewClient(apiKey, secretKey, passphrase string, transportProxy *http.Transport) *Client {
	httpClient := new(http.Client)
	if transportProxy != nil {
		httpClient.Transport = transportProxy
	}

	return &Client{
		apiKey:     apiKey,
		secretKey:  []byte(secretKey),
		passphrase: passphrase,
		client:     httpClient,
	}
}

func (client *Client) GetSupportedChains(ctx context.Context) ([]ChainInfo, error) {
	res, err := client.request(ctx, http.MethodGet, "/api/v5/wallet/chain/supported-chains", true, nil, nil)
	if err != nil {
		return nil, err
	}

	var chains []ChainInfo
	if err = client.toJSON(res, &chains); err != nil {
		return nil, err
	}
	return chains, nil
}

func (client *Client) GetRealtimePrice(ctx context.Context, chainIndex string, tokenAddresses []string) (map[string]decimal.Decimal, error) {
	if len(tokenAddresses) == 0 {
		return nil, nil
	}

	params := []map[string]string{}
	for _, tokenAddress := range tokenAddresses {
		params = append(params, map[string]string{
			"chainIndex":   chainIndex,
			"tokenAddress": tokenAddress,
		})
	}
	res, err := client.request(ctx, http.MethodPost, "/api/v5/wallet/token/real-time-price", true, nil, params)
	if err != nil {
		return nil, err
	}

	var realtimePrices []RealtimePrice
	if err = client.toJSON(res, &realtimePrices); err != nil {
		return nil, err
	}

	result := lo.SliceToMap(realtimePrices, func(item RealtimePrice) (string, decimal.Decimal) {
		return item.TokenAddress, item.Price
	})
	return result, nil
}

func (client *Client) GetAllTokenBalancesByAddress(ctx context.Context, chainIndex string, address string) ([]TokenBalance, error) {
	if chainIndex == "" || address == "" {
		return nil, errors.New("chainIndex and address cannot be empty")
	}

	params := map[string]string{
		"address":          address,
		"chains":           chainIndex,
		"excludeRiskToken": "0",
	}
	res, err := client.request(ctx, http.MethodGet, "/api/v5/dex/balance/all-token-balances-by-address", true, params, nil)
	if err != nil {
		return nil, err
	}

	var tokenAssets []TokenAssets
	if err = client.toJSON(res, &tokenAssets); err != nil {
		return nil, err
	}

	if len(tokenAssets) == 0 {
		return nil, nil
	}
	return tokenAssets[0].TokenAssets, nil
}

func (client *Client) Quote(ctx context.Context, chainIndex string, userWalletAddress, inputToken, outputToken string, amount *big.Int, slippageBps int) (*SwapInstruction, error) {
	params := map[string]string{
		"chainIndex":        chainIndex,
		"amount":            amount.String(),
		"fromTokenAddress":  inputToken,
		"toTokenAddress":    outputToken,
		"slippage":          decimal.NewFromInt(int64(slippageBps)).Div(decimal.NewFromInt(10000)).String(),
		"userWalletAddress": userWalletAddress,
	}

	res, err := client.request(ctx, http.MethodGet, "/api/v5/dex/aggregator/swap-instruction", true, params, nil)
	if err != nil {
		return nil, err
	}

	var swapInstruction SwapInstruction
	if err = client.toJSON(res, &swapInstruction); err != nil {
		return nil, err
	}

	return &swapInstruction, nil
}

func (client *Client) SendSwapTransaction(ctx context.Context, svcCtx *svc.ServiceContext, wallet *solana.Wallet, swapInstruction *SwapInstruction, priorityLevel settings.PriorityLevel, maxRetries uint) (string, error) {
	latestBlockhash, err := svcCtx.SolanaRpc.GetLatestBlockhash(ctx, "")
	if err != nil {
		return "", fmt.Errorf("could not get latest blockhash: %w", err)
	}

	// 创建交易指令
	instructions := []solana.Instruction{}
	for _, instrData := range swapInstruction.InstructionLists {
		var accounts []*solana.AccountMeta
		for _, key := range instrData.Accounts {
			accounts = append(accounts, &solana.AccountMeta{
				PublicKey:  solana.MustPublicKeyFromBase58(key.Pubkey),
				IsSigner:   key.IsSigner,
				IsWritable: key.IsWritable,
			})
		}

		data, err := base64.StdEncoding.DecodeString(instrData.Data)
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

	tables, err := svcCtx.LookuptableCache.GetAddressLookupTables(ctx, swapInstruction.AddressLookupTableAccount)
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

func (client *Client) sign(method, path, body string) (string, string) {
	format := "2006-01-02T15:04:05.999Z07:00"
	t := time.Now().UTC().Format(format)
	ts := fmt.Sprint(t)
	s := ts + method + path + body
	p := []byte(s)
	h := hmac.New(sha256.New, client.secretKey)
	h.Write(p)
	return ts, base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func (client *Client) toJSON(res *http.Response, v interface{}) error {
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return errors.New("failed to read response body: " + err.Error())
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("error response: %s", string(resBody))
	}

	var resData restResponse
	if err = json.Unmarshal(resBody, &resData); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resData.Code != "0" {
		return fmt.Errorf("error code %s: %s", resData.Code, resData.Msg)
	}

	if err = json.Unmarshal(resData.Data, v); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return nil
}

func (client *Client) request(ctx context.Context, method, path string, private bool, urlParams map[string]string, jsonParams any) (*http.Response, error) {
	u := fmt.Sprintf("https://web3.okx.com%s", path)
	var (
		r    *http.Request
		err  error
		j    []byte
		body string
	)
	if method == http.MethodGet {
		r, err = http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}

		if len(urlParams) > 0 {
			q := r.URL.Query()
			for k, v := range urlParams {
				q.Add(k, strings.ReplaceAll(v, "\"", ""))
			}
			r.URL.RawQuery = q.Encode()
			if len(urlParams) > 0 {
				path += "?" + r.URL.RawQuery
			}
		}
	} else {
		j, err = json.Marshal(jsonParams)
		if err != nil {
			return nil, err
		}
		body = string(j)
		if body == "{}" {
			body = ""
		}
		r, err = http.NewRequestWithContext(ctx, method, u, bytes.NewBuffer(j))
		if err != nil {
			return nil, err
		}
		r.Header.Add("Content-Type", "application/json")
	}
	if err != nil {
		return nil, err
	}
	if private {
		timestamp, sign := client.sign(method, path, body)
		r.Header.Add("OK-ACCESS-KEY", client.apiKey)
		r.Header.Add("OK-ACCESS-PASSPHRASE", client.passphrase)
		r.Header.Add("OK-ACCESS-SIGN", sign)
		r.Header.Add("OK-ACCESS-TIMESTAMP", timestamp)
	}

	return client.client.Do(r)
}

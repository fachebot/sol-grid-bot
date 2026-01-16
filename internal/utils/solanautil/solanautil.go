package solanautil

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	bin "github.com/gagliardetto/binary"
	token_metadata "github.com/gagliardetto/metaplex-go/clients/token-metadata"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/tidwall/gjson"
)

var ErrTxNotFound = errors.New("tx not found")

const (
	USDC         = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	USDCDecimals = 6
)

type ProgramError struct {
	label string
}

func (e *ProgramError) Error() string {
	return "Program Error: " + e.label
}

type TokenBalanceChange struct {
	Pre    decimal.Decimal
	Post   decimal.Decimal
	Change decimal.Decimal
}

func IsProgramError(err error) bool {
	if err == nil {
		return false
	}
	var e *ProgramError
	return errors.As(err, &e)
}

func ParseSOL(value *big.Int) decimal.Decimal {
	return ParseUnits(value, 9)
}

func ParseUnits(value *big.Int, decimals uint8) decimal.Decimal {
	mul := decimal.NewFromFloat(float64(10)).Pow(decimal.NewFromInt32(int32(decimals)))
	num, _ := decimal.NewFromString(value.String())
	result := num.DivRound(mul, int32(decimals)).Truncate(int32(decimals))
	return result
}

func FormatSOL(amount decimal.Decimal) *big.Int {
	return FormatUnits(amount, 9)
}

func FormatUnits(amount decimal.Decimal, decimals uint8) *big.Int {
	mul := decimal.NewFromFloat(float64(10)).Pow(decimal.NewFromInt32(int32(decimals)))
	result := amount.Mul(mul)

	wei := big.NewInt(0)
	wei.SetString(result.String(), 10)
	return wei
}

func NewTransactionFromBase64(txStr string) (solana.Transaction, error) {
	txBytes, err := base64.StdEncoding.DecodeString(txStr)
	if err != nil {
		return solana.Transaction{}, fmt.Errorf("could not decode transaction: %w", err)
	}

	tx, err := solana.TransactionFromDecoder(bin.NewBinDecoder(txBytes))
	if err != nil {
		return solana.Transaction{}, fmt.Errorf("could not deserialize transaction: %w", err)
	}

	return *tx, nil
}

func SignTransaction(wallet *solana.Wallet, tx *solana.Transaction) (*solana.Transaction, string, error) {
	txMessageBytes, err := tx.Message.MarshalBinary()
	if err != nil {
		return nil, "", fmt.Errorf("could not serialize transaction: %w", err)
	}

	signature, err := wallet.PrivateKey.Sign(txMessageBytes)
	if err != nil {
		return nil, "", fmt.Errorf("could not sign transaction: %w", err)
	}

	tx.Signatures = []solana.Signature{signature}

	return tx, signature.String(), nil
}

func GetBalance(ctx context.Context, solanaRpc *rpc.Client, ownerAddress string) (*big.Int, error) {
	owner, err := solana.PublicKeyFromBase58(ownerAddress)
	if err != nil {
		return nil, err
	}

	balance, err := solanaRpc.GetBalance(ctx, owner, rpc.CommitmentFinalized)
	if err != nil {
		return nil, err
	}
	return big.NewInt(0).SetUint64(balance.Value), err
}

func GetSignatureStatuses(ctx context.Context, solanaRpc *rpc.Client, txHash string) (*rpc.GetSignatureStatusesResult, error) {
	sig, err := solana.SignatureFromBase58(txHash)
	if err != nil {
		return nil, err
	}

	status, err := solanaRpc.GetSignatureStatuses(ctx, true, sig)
	if err != nil {
		return nil, err
	}

	if len(status.Value) == 0 {
		return nil, fmt.Errorf("could not confirm transaction: no valid status")
	}

	return status, nil
}

func GetTokenMint(ctx context.Context, solanaRpc *rpc.Client, tokenAddress string) (*token.Mint, error) {
	account, err := solana.PublicKeyFromBase58(tokenAddress)
	if err != nil {
		return nil, err
	}

	ata, err := solanaRpc.GetAccountInfo(ctx, account)
	if err != nil {
		return nil, err
	}

	var mint *token.Mint
	if err := bin.NewBinDecoder(ata.Value.Data.GetBinary()).Decode(&mint); err != nil {
		return nil, err
	}

	return mint, nil
}

func GetAccountInfoJSONParsed(ctx context.Context, solanaRpc *rpc.Client, account solana.PublicKey) (out *rpc.GetAccountInfoResult, err error) {
	return solanaRpc.GetAccountInfoWithOpts(
		ctx,
		account,
		&rpc.GetAccountInfoOpts{
			Encoding:   solana.EncodingJSONParsed,
			Commitment: "",
			DataSlice:  nil,
		},
	)
}

func GetTokenMeta(ctx context.Context, solanaRpc *rpc.Client, tokenAddress string) (*token_metadata.Metadata, error) {
	account, err := solana.PublicKeyFromBase58(tokenAddress)
	if err != nil {
		return nil, err
	}

	mintAta, err := GetAccountInfoJSONParsed(ctx, solanaRpc, account)
	if err != nil {
		return nil, err
	}

	tokenProgramID := mintAta.Value.Owner
	if tokenProgramID == solana.Token2022ProgramID {
		var tokenData TokenData
		err = json.Unmarshal(mintAta.Value.Data.GetRawJSON(), &tokenData)
		if err != nil {
			return nil, err
		}

		for _, ext := range tokenData.Parsed.Info.Extensions {
			if ext.Extension == "tokenMetadata" {
				var state TokenMetadataState
				err := json.Unmarshal(ext.State, &state)
				if err != nil {
					return nil, err
				}

				meta := &token_metadata.Metadata{
					Data: token_metadata.Data{
						Name:   state.Name,
						Symbol: state.Symbol,
						Uri:    state.URI,
					},
					Mint: solana.MustPublicKeyFromBase58(state.Mint),
				}
				return meta, nil
			}
		}
		return nil, fmt.Errorf("tokenMetadata extension not found")
	}

	metaAddress, _, err := solana.FindTokenMetadataAddress(account)
	if err != nil {
		return nil, err
	}

	ata, err := solanaRpc.GetAccountInfo(ctx, metaAddress)
	if err != nil {
		return nil, err
	}

	var meta *token_metadata.Metadata
	decoder := bin.NewBorshDecoder(ata.Value.Data.GetBinary())
	if err = decoder.Decode(&meta); err != nil {
		return nil, err
	}

	meta.Data.Name = strings.TrimRight(meta.Data.Name, "\u0000")
	meta.Data.Symbol = strings.TrimRight(meta.Data.Symbol, "\u0000")

	return meta, nil
}

func FindAssociatedTokenAddress(owner, mint, programID solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{
		owner[:],
		programID[:],
		mint[:],
	},
		solana.SPLAssociatedTokenAccountProgramID,
	)
}

func GetTokenBalance(ctx context.Context, solanaRpc *rpc.Client, tokenAddress, ownerAddress string) (*big.Int, uint8, error) {
	owner, err := solana.PublicKeyFromBase58(ownerAddress)
	if err != nil {
		return nil, 0, err
	}

	mint, err := solana.PublicKeyFromBase58(tokenAddress)
	if err != nil {
		return nil, 0, err
	}

	mintAta, err := GetAccountInfoJSONParsed(ctx, solanaRpc, mint)
	if err != nil {
		return nil, 0, err
	}

	tokenProgramID := mintAta.Value.Owner
	ata, _, err := FindAssociatedTokenAddress(owner, mint, tokenProgramID)
	if err != nil {
		return nil, 0, err
	}

	v := gjson.GetBytes(mintAta.Value.Data.GetRawJSON(), "parsed.info.decimals")
	if !v.Exists() {
		return nil, 0, fmt.Errorf("token decimals not found")
	}
	decimals := uint8(v.Uint())

	account, err := solanaRpc.GetTokenAccountBalance(ctx, ata, rpc.CommitmentFinalized)
	if err != nil {
		if strings.Contains(err.Error(), "could not find account") {
			return big.NewInt(0), decimals, nil
		}
		return nil, 0, err
	}

	balance, err := decimal.NewFromString(account.Value.Amount)
	if err != nil {
		return nil, 0, err
	}

	return balance.BigInt(), account.Value.Decimals, nil
}

func GetTokenBalanceChanges(ctx context.Context, solanaRpc *rpc.Client, hash, ownerAddress string) (map[string]TokenBalanceChange, error) {
	txSig, err := solana.SignatureFromBase58(hash)
	if err != nil {
		return nil, err
	}

	owner, err := solana.PublicKeyFromBase58(ownerAddress)
	if err != nil {
		return nil, err
	}

	maxSupportedTransactionVersion := uint64(0)
	tx, err := solanaRpc.GetTransaction(
		context.Background(),
		txSig,
		&rpc.GetTransactionOpts{
			Encoding:                       solana.EncodingBase64,
			MaxSupportedTransactionVersion: &maxSupportedTransactionVersion,
			Commitment:                     rpc.CommitmentConfirmed,
		},
	)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, ErrTxNotFound
		}
		return nil, err
	}

	if tx.Meta.Err != nil {
		dict, ok := tx.Meta.Err.(map[string]any)
		if !ok {
			return nil, &ProgramError{label: fmt.Sprintf("%+v", tx.Meta.Err)}
		}
		return nil, &ProgramError{label: strings.Join(lo.Keys(dict), ", ")}
	}

	changes := make(map[string]TokenBalanceChange)
	for _, balance := range tx.Meta.PreTokenBalances {
		if balance.Owner == nil || *balance.Owner != owner || balance.UiTokenAmount == nil {
			continue
		}

		uiAmount, err := decimal.NewFromString(balance.UiTokenAmount.UiAmountString)
		if err != nil {
			continue
		}

		changes[balance.Mint.String()] = TokenBalanceChange{
			Pre: uiAmount,
		}
	}

	for _, balance := range tx.Meta.PostTokenBalances {
		if balance.Owner == nil || *balance.Owner != owner || balance.UiTokenAmount == nil {
			continue
		}

		uiAmount, err := decimal.NewFromString(balance.UiTokenAmount.UiAmountString)
		if err != nil {
			continue
		}

		change, ok := changes[balance.Mint.String()]
		if !ok {
			change.Pre = decimal.Zero
		}
		change.Post = uiAmount
		change.Change = uiAmount.Sub(change.Pre)
		changes[balance.Mint.String()] = change
	}

	return changes, nil
}

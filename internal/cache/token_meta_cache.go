package cache

import (
	"context"
	"sync"

	"github.com/fachebot/sol-grid-bot/internal/utils/solanautil"

	"github.com/gagliardetto/solana-go/rpc"
)

type TokenMeta struct {
	Name     string
	Symbol   string
	Decimals uint8
	Supply   uint64
}

type TokenMetaCache struct {
	client       *rpc.Client
	tokenMetaMap sync.Map
}

func NewTokenMetaCache(client *rpc.Client) *TokenMetaCache {
	return &TokenMetaCache{client: client}
}

func (c *TokenMetaCache) GetTokenMeta(ctx context.Context, tokenAddress string) (TokenMeta, error) {
	meta, err := c.loadTokenMeta(ctx, tokenAddress)
	if err != nil {
		return TokenMeta{}, err
	}
	return meta, nil
}

func (c *TokenMetaCache) loadTokenMeta(ctx context.Context, tokenAddress string) (TokenMeta, error) {
	val, ok := c.tokenMetaMap.Load(tokenAddress)
	if ok {
		return val.(TokenMeta), nil
	}

	tokenmeta, err := solanautil.GetTokenMeta(ctx, c.client, tokenAddress)
	if err != nil {
		return TokenMeta{}, err
	}

	mint, err := solanautil.GetTokenMint(ctx, c.client, tokenAddress)
	if err != nil {
		return TokenMeta{}, err
	}

	ret := TokenMeta{
		Name:     tokenmeta.Data.Name,
		Symbol:   tokenmeta.Data.Symbol,
		Decimals: mint.Decimals,
		Supply:   mint.Supply,
	}
	c.tokenMetaMap.Store(tokenAddress, ret)

	return ret, nil
}

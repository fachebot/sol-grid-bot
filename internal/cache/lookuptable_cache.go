package cache

import (
	"context"
	"sync"

	"github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/samber/lo"
	"github.com/zeromicro/go-zero/core/fx"
)

type LookuptableCache struct {
	solanaRpc *rpc.Client

	mutex         sync.Mutex
	addressTables map[solana.PublicKey]solana.PublicKeySlice
}

func NewLookuptableCache(solanaRpc *rpc.Client) *LookuptableCache {
	return &LookuptableCache{solanaRpc: solanaRpc, addressTables: map[solana.PublicKey]solana.PublicKeySlice{}}
}

func (cache *LookuptableCache) GetAddressLookupTables(ctx context.Context, adressLookupTableAddresses []string) (map[solana.PublicKey]solana.PublicKeySlice, error) {
	fns := make([]func(), 0)
	result := make(map[solana.PublicKey]solana.PublicKeySlice)
	adressLookupTableAddresses = lo.Uniq(adressLookupTableAddresses)

	var errors []error
	var localMutex sync.Mutex

	for _, address := range adressLookupTableAddresses {
		pk, err := solana.PublicKeyFromBase58(address)
		if err != nil {
			return nil, err
		}

		cache.mutex.Lock()
		addresses, ok := cache.addressTables[pk]
		if ok {
			result[pk] = addresses
			cache.mutex.Unlock()
			continue
		}
		cache.mutex.Unlock()

		fns = append(fns, func() {
			res, err := addresslookuptable.GetAddressLookupTable(ctx, cache.solanaRpc, pk)
			if err != nil {
				localMutex.Lock()
				errors = append(errors, err)
				localMutex.Unlock()
			} else {
				localMutex.Lock()
				result[pk] = res.Addresses
				localMutex.Unlock()

				cache.mutex.Lock()
				cache.addressTables[pk] = res.Addresses
				cache.mutex.Unlock()
			}
		})
	}

	if len(fns) > 0 {
		fx.Parallel(fns...)
	}

	if len(errors) > 0 {
		return nil, errors[0]
	}
	return result, nil
}

package jupiter

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

type Audit struct {
	MintAuthorityDisabled   bool            `json:"mintAuthorityDisabled"`
	FreezeAuthorityDisabled bool            `json:"freezeAuthorityDisabled"`
	TopHoldersPercentage    decimal.Decimal `json:"topHoldersPercentage"`
}

type Stats struct {
	PriceChange       decimal.Decimal `json:"priceChange"`
	LiquidityChange   decimal.Decimal `json:"liquidityChange"`
	VolumeChange      decimal.Decimal `json:"volumeChange"`
	BuyVolume         decimal.Decimal `json:"buyVolume"`
	SellVolume        decimal.Decimal `json:"sellVolume"`
	BuyOrganicVolume  decimal.Decimal `json:"buyOrganicVolume"`
	SellOrganicVolume decimal.Decimal `json:"sellOrganicVolume"`
	NumBuys           int             `json:"numBuys"`
	NumSells          int             `json:"numSells"`
	NumTraders        int             `json:"numTraders"`
	NumOrganicBuyers  int             `json:"numOrganicBuyers"`
	NumNetBuyers      int             `json:"numNetBuyers"`
}

type FirstPool struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
}

type TokenInfo struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Symbol            string          `json:"symbol"`
	Icon              string          `json:"icon"`
	Decimals          int             `json:"decimals"`
	CircSupply        decimal.Decimal `json:"circSupply"`
	TotalSupply       decimal.Decimal `json:"totalSupply"`
	TokenProgram      string          `json:"tokenProgram"`
	FirstPool         FirstPool       `json:"firstPool"`
	HolderCount       int             `json:"holderCount"`
	Audit             Audit           `json:"audit"`
	OrganicScore      decimal.Decimal `json:"organicScore"`
	OrganicScoreLabel string          `json:"organicScoreLabel"`
	IsVerified        bool            `json:"isVerified"`
	Cexes             []string        `json:"cexes"`
	Tags              []string        `json:"tags"`
	FDV               decimal.Decimal `json:"fdv"`
	MCap              decimal.Decimal `json:"mcap"`
	UsdPrice          decimal.Decimal `json:"usdPrice"`
	PriceBlockId      int64           `json:"priceBlockId"`
	Liquidity         decimal.Decimal `json:"liquidity"`
	Stats5m           Stats           `json:"stats5m"`
	Stats1h           Stats           `json:"stats1h"`
	Stats6h           Stats           `json:"stats6h"`
	Stats24h          Stats           `json:"stats24h"`
	CtLikes           int             `json:"ctLikes"`
	SmartCtLikes      int             `json:"smartCtLikes"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

type SwapInfo struct {
	AmmKey     string          `json:"ammKey"`
	Label      string          `json:"label"`
	InputMint  string          `json:"inputMint"`
	OutputMint string          `json:"outputMint"`
	InAmount   decimal.Decimal `json:"inAmount"`
	OutAmount  decimal.Decimal `json:"outAmount"`
	FeeAmount  decimal.Decimal `json:"feeAmount"`
	FeeMint    string          `json:"feeMint"`
}

type RoutePlan struct {
	SwapInfo SwapInfo `json:"swapInfo"`
	Percent  int      `json:"percent"`
	Bps      int      `json:"bps"`
}

type PlatformFee struct {
	Amount decimal.Decimal `json:"amount"`
	FeeBps int             `json:"feeBps"`
}

type QuoteResponse struct {
	InputMint            string          `json:"inputMint"`
	InAmount             decimal.Decimal `json:"inAmount"`
	OutputMint           string          `json:"outputMint"`
	OutAmount            decimal.Decimal `json:"outAmount"`
	OtherAmountThreshold decimal.Decimal `json:"otherAmountThreshold"`
	SwapMode             string          `json:"swapMode"`
	SlippageBps          int             `json:"slippageBps"`
	PlatformFee          *PlatformFee    `json:"platformFee"`
	PriceImpactPct       decimal.Decimal `json:"priceImpactPct"`
	RoutePlan            []RoutePlan     `json:"routePlan"`
}

type PriorityLevel string

var (
	PriorityLevelNone     = PriorityLevel("")
	PriorityLevelMedium   = PriorityLevel("medium")   // 25%
	PriorityLevelHigh     = PriorityLevel("high")     // 50%
	PriorityLevelVeryHigh = PriorityLevel("veryHigh") // 75%
)

type PriorityLevelWithMaxLamports struct {
	PriorityLevel PriorityLevel `json:"priorityLevel"`
	MaxLamports   int64         `json:"maxLamports"`
}

type PrioritizationFeeLamports struct {
	JitoTipLamports              int64                         `json:"jitoTipLamports,omitempty"`
	PriorityLevelWithMaxLamports *PriorityLevelWithMaxLamports `json:"priorityLevelWithMaxLamports,omitempty"`
}

type SwapRequest struct {
	UserPublicKey             string                    `json:"userPublicKey"`
	QuoteResponse             QuoteResponse             `json:"quoteResponse"`
	PrioritizationFeeLamports PrioritizationFeeLamports `json:"prioritizationFeeLamports"`
	DynamicComputeUnitLimit   bool                      `json:"dynamicComputeUnitLimit"`
}

type SimulationError struct {
	ErrorCode string `json:"errorCode"`
	Error     string `json:"error"`
}

type SwapResponse struct {
	SwapTransaction           string           `json:"swapTransaction"`
	LastValidBlockHeight      int64            `json:"lastValidBlockHeight"`
	PrioritizationFeeLamports int64            `json:"prioritizationFeeLamports"`
	SimulationError           *SimulationError `json:"simulationError"`
}

type ErrorResponse struct {
	Message   string `json:"error"`
	ErrorCode string `json:"errorCode"`
}

func (e *ErrorResponse) Error() string {
	return fmt.Sprintf("message: %s, errorCode: %s", e.Message, e.ErrorCode)
}

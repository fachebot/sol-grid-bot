package config

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

type Solana struct {
	RpcUrl        string `yaml:"RpcUrl"`
	MaxRetries    uint   `yaml:"MaxRetries"`
	SlippageBps   int    `yaml:"SlippageBps"`
	MaxLamports   int64  `yaml:"MaxLamports"`
	PriorityLevel string `yaml:"PriorityLevel"`
	DexAggregator string `yaml:"DexAggregator"`
}

type Jupiter struct {
	Url    string `yaml:"Url"`
	Apikey string `yaml:"Apikey"`
}

type OkxWeb3 struct {
	Apikey     string `yaml:"Apikey"`
	Secretkey  string `yaml:"Secretkey"`
	Passphrase string `yaml:"Passphrase"`
}

type Sock5Proxy struct {
	Host   string `yaml:"Host"`
	Port   int32  `yaml:"Port"`
	Enable bool   `yaml:"Enable"`
}

type TelegramBot struct {
	Debug     bool    `yaml:"Debug"`
	ApiToken  string  `yaml:"ApiToken"`
	WhiteList []int64 `yaml:"WhiteList"`
}

func (c *TelegramBot) IsWhiteListUser(userId int64) bool {
	if len(c.WhiteList) == 0 {
		return true
	}
	return slices.Contains(c.WhiteList, userId)
}

type DefaultGridSettings struct {
	OrderSize             decimal.Decimal `yaml:"OrderSize"`
	MaxGridLimit          int             `yaml:"MaxGridLimit"`
	StopLossExit          decimal.Decimal `yaml:"StopLossExit"`
	TakeProfitExit        decimal.Decimal `yaml:"TakeProfitExit"`
	TakeProfitRatio       decimal.Decimal `yaml:"TakeProfitRatio"`
	EnableAutoExit        bool            `yaml:"EnableAutoExit"`
	LastKlineVolume       decimal.Decimal `yaml:"LastKlineVolume"`
	FiveKlineVolume       decimal.Decimal `yaml:"FiveKlineVolume"`
	GlobalTakeProfitRatio decimal.Decimal `yaml:"GlobalTakeProfitRatio"`
	DropOn                bool            `yaml:"DropOn"`
	CandlesToCheck        int             `yaml:"CandlesToCheck"`
	DropThreshold         decimal.Decimal `yaml:"DropThreshold"`
}

func (c *DefaultGridSettings) Validate() error {
	if c.MaxGridLimit <= 0 {
		c.MaxGridLimit = 10
	}
	if c.OrderSize.LessThanOrEqual(decimal.Zero) {
		c.OrderSize = decimal.NewFromInt(40)
	}
	if c.TakeProfitRatio.LessThanOrEqual(decimal.Zero) {
		c.TakeProfitRatio = decimal.NewFromFloat(3.5)
	}

	if c.MaxGridLimit <= 0 {
		c.MaxGridLimit = 10
	}
	if c.OrderSize.LessThanOrEqual(decimal.Zero) {
		c.OrderSize = decimal.NewFromInt(40)
	}
	if c.TakeProfitRatio.LessThanOrEqual(decimal.Zero) {
		c.TakeProfitRatio = decimal.NewFromFloat(3.5)
	}

	if c.GlobalTakeProfitRatio.LessThan(decimal.Zero) {
		return errors.New("GlobalTakeProfitRatio 不能小于0")
	}

	if c.CandlesToCheck < 0 {
		c.CandlesToCheck = 0
	}
	if c.DropThreshold.LessThan(decimal.Zero) {
		c.DropThreshold = decimal.Zero
	}

	return nil
}

type QuickStartSettings struct {
	OrderSize             decimal.Decimal `yaml:"OrderSize"`
	MaxGridLimit          int             `yaml:"MaxGridLimit"`
	StopLossExit          decimal.Decimal `yaml:"StopLossExit"`
	TakeProfitExit        decimal.Decimal `yaml:"TakeProfitExit"`
	TakeProfitRatio       decimal.Decimal `yaml:"TakeProfitRatio"`
	EnableAutoExit        bool            `yaml:"EnableAutoExit"`
	LastKlineVolume       decimal.Decimal `yaml:"LastKlineVolume"`
	FiveKlineVolume       decimal.Decimal `yaml:"FiveKlineVolume"`
	UpperPriceBound       decimal.Decimal `yaml:"UpperPriceBound"`
	LowerPriceBound       decimal.Decimal `yaml:"LowerPriceBound"`
	GlobalTakeProfitRatio decimal.Decimal `yaml:"GlobalTakeProfitRatio"`
	DropOn                bool            `yaml:"DropOn"`
	CandlesToCheck        int             `yaml:"CandlesToCheck"`
	DropThreshold         decimal.Decimal `yaml:"DropThreshold"`
}

type TokenRequirements struct {
	MinMarketCap       decimal.Decimal `yaml:"MinMarketCap"`
	MinHolderCount     int             `yaml:"MinHolderCount"`
	MinTokenAgeMinutes int             `yaml:"MinTokenAgeMinutes"`
	MaxTokenAgeMinutes int             `yaml:"MaxTokenAgeMinutes"`
}

type Top100AvgCostFilter struct {
	MinPercentage decimal.Decimal `yaml:"MinPercentage"`
	MaxPercentage decimal.Decimal `yaml:"MaxPercentage"`
}

type Config struct {
	Solana              Solana              `yaml:"Solana"`
	Jupiter             Jupiter             `yaml:"Jupiter"`
	Datapi              string              `yaml:"Datapi"`
	OkxWeb3             OkxWeb3             `yaml:"OkxWeb3"`
	Sock5Proxy          Sock5Proxy          `yaml:"Sock5Proxy"`
	TelegramBot         TelegramBot         `yaml:"TelegramBot"`
	DefaultGridSettings DefaultGridSettings `yaml:"DefaultGridSettings"`
	QuickStartSettings  QuickStartSettings  `yaml:"QuickStartSettings"`
	TokenRequirements   TokenRequirements   `yaml:"TokenRequirements"`
}

func LoadFromFile(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var c Config
	err = yaml.Unmarshal([]byte(data), &c)
	if err != nil {
		return nil, err
	}

	if err = c.DefaultGridSettings.Validate(); err != nil {
		return nil, fmt.Errorf("DefaultGridSettings配置错误: %w", err)
	}

	if c.Datapi != "gmgn" && c.Datapi != "jupag" && c.Datapi != "okx" {
		return nil, errors.New("Datapi配置枚举值范围: gmgn/jupag/okx")
	}

	return &c, nil
}

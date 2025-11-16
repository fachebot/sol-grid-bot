package model

import (
	"context"

	"github.com/fachebot/sol-grid-bot/internal/ent"
	"github.com/fachebot/sol-grid-bot/internal/ent/settings"
)

type SettingsModel struct {
	client *ent.SettingsClient
}

func NewSettingsModel(client *ent.SettingsClient) *SettingsModel {
	return &SettingsModel{client: client}
}

func (model *SettingsModel) Save(ctx context.Context, args ent.Settings) (*ent.Settings, error) {
	return model.client.Create().
		SetUserId(args.UserId).
		SetMaxRetries(args.MaxRetries).
		SetSlippageBps(args.SlippageBps).
		SetNillableSellSlippageBps(args.SellSlippageBps).
		SetNillableExitSlippageBps(args.ExitSlippageBps).
		SetMaxLamports(args.MaxLamports).
		SetPriorityLevel(args.PriorityLevel).
		SetDexAggregator(args.DexAggregator).
		Save(ctx)
}

func (model *SettingsModel) FindByUserId(ctx context.Context, userId int64) (*ent.Settings, error) {
	return model.client.Query().
		Where(settings.UserIdEQ(userId)).
		First(ctx)
}

func (model *SettingsModel) UpdateSlippageBps(ctx context.Context, id int, newValue int) error {
	return model.client.UpdateOneID(id).
		SetSlippageBps(newValue).
		Exec(ctx)
}

func (model *SettingsModel) UpdateSellSlippageBps(ctx context.Context, id int, newValue int) error {
	return model.client.UpdateOneID(id).
		SetSellSlippageBps(newValue).
		Exec(ctx)
}

func (model *SettingsModel) UpdateExitSlippageBps(ctx context.Context, id int, newValue int) error {
	return model.client.UpdateOneID(id).
		SetExitSlippageBps(newValue).
		Exec(ctx)
}

func (model *SettingsModel) UpdateMaxRetries(ctx context.Context, id int, newValue int64) error {
	return model.client.UpdateOneID(id).
		SetMaxRetries(newValue).
		Exec(ctx)
}

func (model *SettingsModel) UpdateMaxLamports(ctx context.Context, id int, newValue int64) error {
	return model.client.UpdateOneID(id).
		SetMaxLamports(newValue).
		Exec(ctx)
}

func (model *SettingsModel) UpdatePriorityLevel(ctx context.Context, id int, newValue settings.PriorityLevel) error {
	return model.client.UpdateOneID(id).
		SetPriorityLevel(newValue).
		Exec(ctx)
}

func (model *SettingsModel) UpdateDexAggregator(ctx context.Context, id int, dexAggregator settings.DexAggregator) error {
	return model.client.UpdateOneID(id).
		SetDexAggregator(dexAggregator).
		Exec(ctx)
}

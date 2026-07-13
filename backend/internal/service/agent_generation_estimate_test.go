package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type agentEstimateRateRepo struct {
	UserGroupRateRepository
	rate *float64
	err  error
}

func (r *agentEstimateRateRepo) GetByUserAndGroup(context.Context, int64, int64) (*float64, error) {
	return r.rate, r.err
}

func TestEstimateImageGenerationCostUsesUserAndGroupPricing(t *testing.T) {
	groupID := int64(9)
	price := 0.4
	userRate := 1.5
	apiKey := &APIKey{
		UserID:  42,
		GroupID: &groupID,
		Group: &Group{
			ID:             groupID,
			RateMultiplier: 1.2,
			ImagePrice2K:   &price,
		},
	}
	svc := NewBillingService(&config.Config{}, nil)
	cost, tier, err := svc.EstimateImageGenerationCost(
		context.Background(), apiKey, &agentEstimateRateRepo{rate: &userRate}, "gpt-image-2", "2048x1152", 2,
	)
	require.NoError(t, err)
	require.Equal(t, ImageBillingSize2K, tier)
	require.InDelta(t, 0.8, cost.TotalCost, 1e-12)
	require.InDelta(t, 1.2, cost.ActualCost, 1e-12)

	apiKey.Group.ImageRateIndependent = true
	apiKey.Group.ImageRateMultiplier = 0.5
	cost, _, err = svc.EstimateImageGenerationCost(
		context.Background(), apiKey, &agentEstimateRateRepo{rate: &userRate}, "gpt-image-2", "2K", 2,
	)
	require.NoError(t, err)
	require.InDelta(t, 0.4, cost.ActualCost, 1e-12)
}

func TestEstimateVideoGenerationCostSharesCreateTaskFormula(t *testing.T) {
	groupID := int64(9)
	pricing := &videoPricingMemoryRepo{rule: &VideoGroupPricingRule{
		GroupID:                  groupID,
		ModelCode:                VideoModelSeedance20,
		Resolution:               VideoResolution720P,
		CreditsPerSecond:         0.2,
		ReferenceVideoMultiplier: 2,
		Enabled:                  true,
		UpdatedAt:                time.Unix(1783900000, 0),
	}}
	svc := NewVideoService(nil, nil, pricing, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	referenceDuration := 3.0
	request := &VideoCreateRequest{
		Model:       VideoModelSeedance20,
		Prompt:      "continue the shot",
		Duration:    5,
		Resolution:  VideoResolution720P,
		AbilityCode: videoAbilityReferenceToVideo,
		Content: []VideoContent{{
			Type:            "video_url",
			Role:            "reference_video",
			VideoURL:        &VideoContentURL{URL: "https://example.invalid/reference.mp4"},
			DurationSeconds: &referenceDuration,
		}},
	}
	estimate, err := svc.EstimateGenerationCost(context.Background(), &APIKey{Group: &Group{
		ID: groupID, Kind: "agent", SystemCode: "yingzo", RateMultiplier: 1.5,
	}}, request, 2)
	require.NoError(t, err)
	require.Equal(t, 16, estimate.BillableSeconds)
	require.InDelta(t, 4.4, estimate.TotalCost, 1e-12)
	require.InDelta(t, 6.6, estimate.ActualCost, 1e-12)
}

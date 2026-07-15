package handler

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type approximateFloat struct {
	want    float64
	epsilon float64
}

type agentPricingSnapshotRateRepo struct {
	service.UserGroupRateRepository
	rate *float64
}

func (r *agentPricingSnapshotRateRepo) GetByUserAndGroup(context.Context, int64, int64) (*float64, error) {
	return r.rate, nil
}

func TestGetAgentPricingSnapshotReturnsEffectiveImageAndVideoPrices(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	userRate := 1.5
	updatedAt := time.Unix(1784000000, 123).UTC()
	h := &AgentHandler{db: db, userGroupRateRepo: &agentPricingSnapshotRateRepo{rate: &userRate}}

	mock.ExpectQuery("SELECT model_code,platform,media_type,resolution,unit_price,rate_multiplier,reference_multiplier").
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"model_code", "platform", "media_type", "resolution", "unit_price", "rate_multiplier", "reference_multiplier"}).
			AddRow("gpt-image-2", "openai", "image", "2K", 0.2, 0.5, 1.0).
			AddRow("seedance-2.0", "seedance", "video", "720p", 0.3, 2.0, 1.25))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/agent/pricing", nil)
	groupID := int64(9)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID: 3, UserID: 7, GroupID: &groupID,
		Group: &service.Group{ID: groupID, Kind: "agent", SystemCode: "yingzo", RateMultiplier: 2, UpdatedAt: updatedAt},
	})

	h.GetAgentPricingSnapshot(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	var snapshot agentPricingSnapshot
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &snapshot))
	require.Equal(t, "1.0.0", snapshot.SchemaVersion)
	require.Equal(t, service.AgentPricingVersion(&service.Group{ID: groupID, SystemCode: "yingzo", UpdatedAt: updatedAt}), snapshot.PricingVersion)
	require.Equal(t, "credit", snapshot.Currency)
	require.WithinDuration(t, time.Now().UTC(), snapshot.FetchedAt, time.Second)
	require.Equal(t, agentPricingSnapshotTTL, snapshot.ValidUntil.Sub(snapshot.FetchedAt))
	require.Len(t, snapshot.Rules, 2)
	require.Equal(t, agentPricingSnapshotRule{Model: "gpt-image-2", Platform: "openai", MediaType: "image", Resolution: "2K", UnitKind: "image", BillingMultiplier: 1.5, ReferenceMultiplier: 1}, agentPricingSnapshotRule{
		Model: snapshot.Rules[0].Model, Platform: snapshot.Rules[0].Platform, MediaType: snapshot.Rules[0].MediaType,
		Resolution: snapshot.Rules[0].Resolution, UnitKind: snapshot.Rules[0].UnitKind,
		BillingMultiplier: snapshot.Rules[0].BillingMultiplier, ReferenceMultiplier: snapshot.Rules[0].ReferenceMultiplier,
	})
	require.InDelta(t, 0.1, snapshot.Rules[0].UnitPrice, 1e-12)
	require.InDelta(t, 0.15, snapshot.Rules[0].EffectiveUnitPrice, 1e-12)
	require.Equal(t, "seedance-2.0", snapshot.Rules[1].Model)
	require.Equal(t, "second", snapshot.Rules[1].UnitKind)
	require.InDelta(t, 0.6, snapshot.Rules[1].UnitPrice, 1e-12)
	require.InDelta(t, 1.2, snapshot.Rules[1].EffectiveUnitPrice, 1e-12)
	require.InDelta(t, 1.25, snapshot.Rules[1].ReferenceMultiplier, 1e-12)
	require.NoError(t, mock.ExpectationsWereMet())
}

func (a approximateFloat) Match(value driver.Value) bool {
	actual, ok := value.(float64)
	return ok && math.Abs(actual-a.want) <= a.epsilon
}

func TestNormalizeAgentModelPricingRules(t *testing.T) {
	rules, err := normalizeAgentModelPricingRules([]agentModelPricingRule{
		{
			Model:          " gpt-image-2 ",
			Platform:       " OpenAI ",
			MediaType:      " IMAGE ",
			Resolution:     " 2k ",
			UnitPrice:      0.08,
			RateMultiplier: 1.25,
			Enabled:        true,
		},
		{
			Model:                     " claude-sonnet-4 ",
			Platform:                  " ANTHROPIC ",
			MediaType:                 " TEXT ",
			Resolution:                "ignored",
			InputPricePerMillion:      3,
			OutputPricePerMillion:     15,
			CacheWritePricePerMillion: 3.75,
			CacheReadPricePerMillion:  0.3,
			RateMultiplier:            0.9,
			Enabled:                   true,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", rules[0].Model)
	require.Equal(t, "openai", rules[0].Platform)
	require.Equal(t, "image", rules[0].MediaType)
	require.Equal(t, "2K", rules[0].Resolution)
	require.Equal(t, "anthropic", rules[1].Platform)
	require.Equal(t, "text", rules[1].MediaType)
	require.Empty(t, rules[1].Resolution)
}

func TestNormalizeAgentModelPricingRulesRejectsInvalidShapes(t *testing.T) {
	validImage := agentModelPricingRule{
		Model: "gpt-image-2", Platform: "openai", MediaType: "image",
		Resolution: "2K", RateMultiplier: 1, ReferenceMultiplier: 1,
	}

	tests := []struct {
		name  string
		rules []agentModelPricingRule
	}{
		{name: "missing image resolution", rules: []agentModelPricingRule{{Model: "gpt-image-2", Platform: "openai", MediaType: "image", RateMultiplier: 1}}},
		{name: "unknown platform", rules: []agentModelPricingRule{{Model: "model", Platform: "other", MediaType: "text", RateMultiplier: 1}}},
		{name: "duplicate tier", rules: []agentModelPricingRule{validImage, validImage}},
		{name: "negative price", rules: []agentModelPricingRule{{Model: "model", Platform: "openai", MediaType: "text", InputPricePerMillion: -1, RateMultiplier: 1}}},
		{name: "non finite multiplier", rules: []agentModelPricingRule{{Model: "model", Platform: "openai", MediaType: "text", RateMultiplier: math.Inf(1)}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeAgentModelPricingRules(tt.rules)
			require.Error(t, err)
		})
	}
}

func TestCompileAgentPricingBuildsExistingBillingRules(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)

	mock.ExpectExec("DELETE FROM channel_model_pricing").
		WithArgs(int64(17)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("DELETE FROM video_group_pricing_rules").
		WithArgs(int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO channel_model_pricing").
		WithArgs(
			int64(17), "claude-sonnet-4",
			approximateFloat{want: 2.5e-6, epsilon: 1e-12},
			approximateFloat{want: 5e-6, epsilon: 1e-12},
			approximateFloat{want: 3.75e-6, epsilon: 1e-12},
			approximateFloat{want: 0.625e-6, epsilon: 1e-12},
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO video_group_pricing_rules").
		WithArgs(
			int64(5), "seedance-2.0", "720p",
			approximateFloat{want: 0.3, epsilon: 1e-12}, 1.2,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO channel_model_pricing").
		WithArgs(int64(17), "gpt-image-2").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(101)))
	mock.ExpectExec("INSERT INTO channel_pricing_intervals").
		WithArgs(int64(101), "2K", approximateFloat{want: 0.2, epsilon: 1e-12}, 0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO channel_pricing_intervals").
		WithArgs(int64(101), "4K", approximateFloat{want: 0.3, epsilon: 1e-12}, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE groups").
		WithArgs(
			int64(5),
			approximateFloat{want: 0.2, epsilon: 1e-12},
			approximateFloat{want: 0.3, epsilon: 1e-12},
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	rules := []agentModelPricingRule{
		{
			Model: "claude-sonnet-4", Platform: "anthropic", MediaType: "text",
			InputPricePerMillion: 2, OutputPricePerMillion: 4,
			CacheWritePricePerMillion: 3, CacheReadPricePerMillion: 0.5,
			RateMultiplier: 1.25, Enabled: true,
		},
		{
			Model: "seedance-2.0", Platform: "seedance", MediaType: "video", Resolution: "720p",
			UnitPrice: 0.2, RateMultiplier: 1.5, ReferenceMultiplier: 1.2, Enabled: true,
		},
		{
			Model: "gpt-image-2", Platform: "openai", MediaType: "image", Resolution: "2K",
			UnitPrice: 0.1, RateMultiplier: 2, ReferenceMultiplier: 1, Enabled: true,
		},
		{
			Model: "gpt-image-2", Platform: "openai", MediaType: "image", Resolution: "4K",
			UnitPrice: 0.2, RateMultiplier: 1.5, ReferenceMultiplier: 1, Enabled: true,
		},
	}

	require.NoError(t, compileAgentPricing(t.Context(), tx, 5, 17, rules))
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

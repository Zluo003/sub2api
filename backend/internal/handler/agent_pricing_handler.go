package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const maxAgentModelPricingRules = 500

type agentModelPricingRule struct {
	ID                        int64   `json:"id,omitempty"`
	Model                     string  `json:"model"`
	Platform                  string  `json:"platform"`
	MediaType                 string  `json:"media_type"`
	Resolution                string  `json:"resolution"`
	UnitPrice                 float64 `json:"unit_price"`
	InputPricePerMillion      float64 `json:"input_price_per_million"`
	OutputPricePerMillion     float64 `json:"output_price_per_million"`
	CacheWritePricePerMillion float64 `json:"cache_write_price_per_million"`
	CacheReadPricePerMillion  float64 `json:"cache_read_price_per_million"`
	RateMultiplier            float64 `json:"rate_multiplier"`
	ReferenceMultiplier       float64 `json:"reference_multiplier"`
	Enabled                   bool    `json:"enabled"`
}

type agentModelPricingRequest struct {
	Items []agentModelPricingRule `json:"items" binding:"required"`
}

type agentPricingSnapshotRule struct {
	Model               string  `json:"model"`
	Platform            string  `json:"platform"`
	MediaType           string  `json:"media_type"`
	Resolution          string  `json:"resolution"`
	UnitKind            string  `json:"unit_kind"`
	UnitPrice           float64 `json:"unit_price"`
	EffectiveUnitPrice  float64 `json:"effective_unit_price"`
	BillingMultiplier   float64 `json:"billing_multiplier"`
	ReferenceMultiplier float64 `json:"reference_multiplier"`
}

type agentPricingSnapshot struct {
	SchemaVersion  string                     `json:"schema_version"`
	PricingVersion string                     `json:"pricing_version"`
	Currency       string                     `json:"currency"`
	Rules          []agentPricingSnapshotRule `json:"rules"`
	FetchedAt      time.Time                  `json:"fetched_at"`
	ValidUntil     time.Time                  `json:"valid_until"`
}

const agentPricingSnapshotTTL = 24 * time.Hour

// GetAgentPricingSnapshot returns the current image/video price table for the
// authenticated Agent installation. It is a display and local-estimation
// contract; the gateway's actual usage record remains the billing authority.
func (h *AgentHandler) GetAgentPricingSnapshot(c *gin.Context) {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil || apiKey.GroupID == nil || !apiKey.Group.IsAgent() {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "invalid_api_key", "message": "Invalid Agent API key"}})
		return
	}

	imageMultiplier := apiKey.Group.RateMultiplier
	if h.userGroupRateRepo != nil {
		userMultiplier, err := h.userGroupRateRepo.GetByUserAndGroup(c.Request.Context(), apiKey.UserID, *apiKey.GroupID)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "agent_pricing_unavailable", "message": "Unable to resolve current user pricing"}})
			return
		}
		if userMultiplier != nil {
			imageMultiplier = *userMultiplier
		}
	}
	if apiKey.Group.ImageRateIndependent {
		imageMultiplier = apiKey.Group.ImageRateMultiplier
	}
	if imageMultiplier < 0 {
		imageMultiplier = 0
	}
	videoMultiplier := apiKey.Group.RateMultiplier
	if videoMultiplier <= 0 {
		videoMultiplier = 1
	}

	rows, err := h.db.QueryContext(c, `
		SELECT model_code,platform,media_type,resolution,unit_price,rate_multiplier,reference_multiplier
		FROM agent_model_pricing
		WHERE group_id=$1 AND enabled=true AND media_type IN ('image','video')
		ORDER BY CASE media_type WHEN 'image' THEN 1 ELSE 2 END,model_code,resolution,id
	`, apiKey.Group.ID)
	if err != nil {
		writeAgentPricingError(c, err)
		return
	}
	defer func() { _ = rows.Close() }()

	rules := make([]agentPricingSnapshotRule, 0)
	for rows.Next() {
		var rule agentPricingSnapshotRule
		var configuredUnitPrice, configuredMultiplier float64
		if err := rows.Scan(
			&rule.Model, &rule.Platform, &rule.MediaType, &rule.Resolution,
			&configuredUnitPrice, &configuredMultiplier, &rule.ReferenceMultiplier,
		); err != nil {
			writeAgentPricingError(c, err)
			return
		}
		rule.UnitPrice = configuredUnitPrice * configuredMultiplier
		switch rule.MediaType {
		case "image":
			rule.UnitKind = "image"
			rule.BillingMultiplier = imageMultiplier
			rule.ReferenceMultiplier = 1
		case "video":
			rule.UnitKind = "second"
			rule.BillingMultiplier = videoMultiplier
			if rule.ReferenceMultiplier <= 0 {
				rule.ReferenceMultiplier = 1
			}
		}
		rule.EffectiveUnitPrice = rule.UnitPrice * rule.BillingMultiplier
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		writeAgentPricingError(c, err)
		return
	}

	fetchedAt := time.Now().UTC()
	c.JSON(http.StatusOK, agentPricingSnapshot{
		SchemaVersion:  "1.0.0",
		PricingVersion: service.AgentPricingVersion(apiKey.Group),
		Currency:       "credit",
		Rules:          rules,
		FetchedAt:      fetchedAt,
		ValidUntil:     fetchedAt.Add(agentPricingSnapshotTTL),
	})
}

func (h *AgentHandler) ListAgentGroupPricing(c *gin.Context) {
	groupID, ok := parseAgentPricingGroupID(c)
	if !ok {
		return
	}
	if err := requireAgentGroup(c.Request.Context(), h.db, groupID); err != nil {
		writeAgentPricingError(c, err)
		return
	}

	rows, err := h.db.QueryContext(c, `
		SELECT id,model_code,platform,media_type,resolution,unit_price,
			input_price_per_million,output_price_per_million,
			cache_write_price_per_million,cache_read_price_per_million,
			rate_multiplier,reference_multiplier,enabled
		FROM agent_model_pricing
		WHERE group_id=$1
		ORDER BY CASE media_type WHEN 'image' THEN 1 WHEN 'video' THEN 2 ELSE 3 END,
			model_code,resolution,id
	`, groupID)
	if err != nil {
		writeAgentPricingError(c, err)
		return
	}
	defer func() { _ = rows.Close() }()

	items := make([]agentModelPricingRule, 0)
	for rows.Next() {
		var rule agentModelPricingRule
		if err := rows.Scan(
			&rule.ID, &rule.Model, &rule.Platform, &rule.MediaType, &rule.Resolution,
			&rule.UnitPrice, &rule.InputPricePerMillion, &rule.OutputPricePerMillion,
			&rule.CacheWritePricePerMillion, &rule.CacheReadPricePerMillion,
			&rule.RateMultiplier, &rule.ReferenceMultiplier, &rule.Enabled,
		); err != nil {
			writeAgentPricingError(c, err)
			return
		}
		items = append(items, rule)
	}
	if err := rows.Err(); err != nil {
		writeAgentPricingError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"group_id": groupID, "items": items})
}

func (h *AgentHandler) UpdateAgentGroupPricing(c *gin.Context) {
	groupID, ok := parseAgentPricingGroupID(c)
	if !ok {
		return
	}
	var input agentModelPricingRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_agent_pricing", "message": "items is required"}})
		return
	}
	if len(input.Items) > maxAgentModelPricingRules {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "too_many_agent_pricing_rules", "message": "Too many Agent pricing rules"}})
		return
	}
	rules, err := normalizeAgentModelPricingRules(input.Items)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_agent_pricing", "message": err.Error()}})
		return
	}

	tx, err := h.db.BeginTx(c, nil)
	if err != nil {
		writeAgentPricingError(c, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireAgentGroup(c.Request.Context(), tx, groupID); err != nil {
		writeAgentPricingError(c, err)
		return
	}
	channelID, err := ensureDedicatedAgentPricingChannel(c.Request.Context(), tx, groupID)
	if err != nil {
		writeAgentPricingError(c, err)
		return
	}
	if err := replaceAgentPricingSource(c.Request.Context(), tx, groupID, rules); err != nil {
		writeAgentPricingError(c, err)
		return
	}
	if err := compileAgentPricing(c.Request.Context(), tx, groupID, channelID, rules); err != nil {
		writeAgentPricingError(c, err)
		return
	}
	if _, err := tx.ExecContext(c, `UPDATE groups SET updated_at=NOW() WHERE id=$1`, groupID); err != nil {
		writeAgentPricingError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeAgentPricingError(c, err)
		return
	}
	if h.channelService != nil {
		h.channelService.InvalidateCache()
	}
	if h.authInvalidator != nil {
		h.authInvalidator.InvalidateAuthCacheByGroupID(c.Request.Context(), groupID)
	}
	c.JSON(http.StatusOK, gin.H{"group_id": groupID, "items": rules})
}

type agentPricingDB interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func requireAgentGroup(ctx context.Context, db agentPricingDB, groupID int64) error {
	var kind string
	var systemCode sql.NullString
	err := db.QueryRowContext(ctx, `SELECT kind,system_code FROM groups WHERE id=$1 AND deleted_at IS NULL`, groupID).Scan(&kind, &systemCode)
	if errors.Is(err, sql.ErrNoRows) {
		return errAgentPricingGroupNotFound
	}
	if err != nil {
		return err
	}
	if kind != "agent" || !systemCode.Valid || strings.TrimSpace(systemCode.String) == "" {
		return errAgentPricingNotAgentGroup
	}
	return nil
}

var (
	errAgentPricingGroupNotFound = errors.New("agent group not found")
	errAgentPricingNotAgentGroup = errors.New("pricing can only be configured for a system Agent group")
	errAgentPricingSharedChannel = errors.New("system Agent pricing requires a dedicated channel")
)

func parseAgentPricingGroupID(c *gin.Context) (int64, bool) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || groupID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_group_id", "message": "Invalid group ID"}})
		return 0, false
	}
	return groupID, true
}

func writeAgentPricingError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "agent_pricing_failed"
	switch {
	case errors.Is(err, errAgentPricingGroupNotFound):
		status, code = http.StatusNotFound, "agent_group_not_found"
	case errors.Is(err, errAgentPricingNotAgentGroup), errors.Is(err, errAgentPricingSharedChannel):
		status, code = http.StatusConflict, "agent_pricing_conflict"
	}
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": err.Error()}})
}

func normalizeAgentModelPricingRules(input []agentModelPricingRule) ([]agentModelPricingRule, error) {
	allowedPlatforms := map[string]bool{
		"anthropic": true, "openai": true, "gemini": true,
		"antigravity": true, "grok": true, "seedance": true,
	}
	seen := make(map[string]struct{}, len(input))
	out := make([]agentModelPricingRule, 0, len(input))
	for i := range input {
		rule := input[i]
		rule.ID = 0
		rule.Model = strings.TrimSpace(rule.Model)
		rule.Platform = strings.ToLower(strings.TrimSpace(rule.Platform))
		rule.MediaType = strings.ToLower(strings.TrimSpace(rule.MediaType))
		rule.Resolution = strings.TrimSpace(rule.Resolution)
		if rule.Model == "" || len(rule.Model) > 120 {
			return nil, fmt.Errorf("rule %d has an invalid model", i+1)
		}
		if !allowedPlatforms[rule.Platform] {
			return nil, fmt.Errorf("rule %d has an invalid platform", i+1)
		}
		switch rule.MediaType {
		case "text":
			rule.Resolution = ""
		case "image":
			rule.Resolution = strings.ToUpper(rule.Resolution)
		case "video":
			rule.Resolution = strings.ToLower(rule.Resolution)
		default:
			return nil, fmt.Errorf("rule %d has an invalid media_type", i+1)
		}
		if rule.MediaType != "text" && rule.Resolution == "" {
			return nil, fmt.Errorf("rule %d requires a resolution", i+1)
		}
		for _, value := range []float64{
			rule.UnitPrice, rule.InputPricePerMillion, rule.OutputPricePerMillion,
			rule.CacheWritePricePerMillion, rule.CacheReadPricePerMillion,
			rule.RateMultiplier, rule.ReferenceMultiplier,
		} {
			if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("rule %d contains an invalid price or multiplier", i+1)
			}
		}
		key := strings.ToLower(rule.MediaType + "\x00" + rule.Model + "\x00" + rule.Resolution)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("rule %d duplicates %s %s %s", i+1, rule.MediaType, rule.Model, rule.Resolution)
		}
		seen[key] = struct{}{}
		out = append(out, rule)
	}
	return out, nil
}

func ensureDedicatedAgentPricingChannel(ctx context.Context, tx *sql.Tx, groupID int64) (int64, error) {
	var channelID int64
	err := tx.QueryRowContext(ctx, `SELECT channel_id FROM channel_groups WHERE group_id=$1 FOR UPDATE`, groupID).Scan(&channelID)
	if errors.Is(err, sql.ErrNoRows) {
		name := fmt.Sprintf("Yingzo Agent Pricing #%d", groupID)
		err = tx.QueryRowContext(ctx, `
			INSERT INTO channels(name,description,status,billing_model_source,restrict_models)
			VALUES($1,'System-managed Yingzo Agent model pricing','active','requested',false)
			ON CONFLICT(name) DO UPDATE SET status='active',billing_model_source='requested',updated_at=NOW()
			RETURNING id
		`, name).Scan(&channelID)
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO channel_groups(channel_id,group_id) VALUES($1,$2) ON CONFLICT(group_id) DO NOTHING`, channelID, groupID); err != nil {
			return 0, err
		}
		if err = tx.QueryRowContext(ctx, `SELECT channel_id FROM channel_groups WHERE group_id=$1`, groupID).Scan(&channelID); err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}
	var groupCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM channel_groups WHERE channel_id=$1`, channelID).Scan(&groupCount); err != nil {
		return 0, err
	}
	if groupCount != 1 {
		return 0, errAgentPricingSharedChannel
	}
	if _, err := tx.ExecContext(ctx, `UPDATE channels SET status='active',billing_model_source='requested',updated_at=NOW() WHERE id=$1`, channelID); err != nil {
		return 0, err
	}
	return channelID, nil
}

func replaceAgentPricingSource(ctx context.Context, tx *sql.Tx, groupID int64, rules []agentModelPricingRule) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM agent_model_pricing WHERE group_id=$1`, groupID); err != nil {
		return err
	}
	for i := range rules {
		rule := &rules[i]
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO agent_model_pricing(
				group_id,model_code,platform,media_type,resolution,unit_price,
				input_price_per_million,output_price_per_million,
				cache_write_price_per_million,cache_read_price_per_million,
				rate_multiplier,reference_multiplier,enabled
			) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			RETURNING id
		`, groupID, rule.Model, rule.Platform, rule.MediaType, rule.Resolution, rule.UnitPrice,
			rule.InputPricePerMillion, rule.OutputPricePerMillion,
			rule.CacheWritePricePerMillion, rule.CacheReadPricePerMillion,
			rule.RateMultiplier, rule.ReferenceMultiplier, rule.Enabled,
		).Scan(&rule.ID); err != nil {
			return err
		}
	}
	return nil
}

func compileAgentPricing(ctx context.Context, tx *sql.Tx, groupID, channelID int64, rules []agentModelPricingRule) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM channel_model_pricing WHERE channel_id=$1`, channelID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM video_group_pricing_rules WHERE group_id=$1`, groupID); err != nil {
		return err
	}

	imageRules := make(map[string][]agentModelPricingRule)
	var image2K, image4K *float64
	for i := range rules {
		rule := rules[i]
		if !rule.Enabled {
			continue
		}
		effectiveMultiplier := rule.RateMultiplier
		switch rule.MediaType {
		case "text":
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO channel_model_pricing(
					channel_id,platform,models,billing_mode,input_price,output_price,
					cache_write_price,cache_read_price
				) VALUES($1,'openai',jsonb_build_array($2::text),'token',$3,$4,$5,$6)
			`, channelID, rule.Model,
				rule.InputPricePerMillion/1_000_000*effectiveMultiplier,
				rule.OutputPricePerMillion/1_000_000*effectiveMultiplier,
				rule.CacheWritePricePerMillion/1_000_000*effectiveMultiplier,
				rule.CacheReadPricePerMillion/1_000_000*effectiveMultiplier,
			); err != nil {
				return err
			}
		case "image":
			imageRules[rule.Model] = append(imageRules[rule.Model], rule)
			if rule.Model == "gpt-image-2" {
				effectivePrice := rule.UnitPrice * effectiveMultiplier
				switch rule.Resolution {
				case "2K":
					price := effectivePrice
					image2K = &price
				case "4K":
					price := effectivePrice
					image4K = &price
				}
			}
		case "video":
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO video_group_pricing_rules(
					group_id,model_code,resolution,credits_per_second,
					reference_video_multiplier,enabled
				) VALUES($1,$2,$3,$4,$5,true)
			`, groupID, rule.Model, rule.Resolution,
				rule.UnitPrice*effectiveMultiplier, rule.ReferenceMultiplier,
			); err != nil {
				return err
			}
		}
	}
	for model, modelRules := range imageRules {
		var pricingID int64
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO channel_model_pricing(channel_id,platform,models,billing_mode)
			VALUES($1,'openai',jsonb_build_array($2::text),'image') RETURNING id
		`, channelID, model).Scan(&pricingID); err != nil {
			return err
		}
		for index, rule := range modelRules {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO channel_pricing_intervals(pricing_id,tier_label,per_request_price,sort_order)
				VALUES($1,$2,$3,$4)
			`, pricingID, rule.Resolution, rule.UnitPrice*rule.RateMultiplier, index); err != nil {
				return err
			}
		}
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE groups
		SET image_price_2k=$2,image_price_4k=$3,image_rate_independent=false,updated_at=NOW()
		WHERE id=$1
	`, groupID, image2K, image4K)
	return err
}

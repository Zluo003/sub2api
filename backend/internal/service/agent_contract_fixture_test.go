package service

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type yingzoAgentContractFixture struct {
	AgentAPIContractVersion     string   `json:"agent_api_contract_version"`
	Models                      []string `json:"models"`
	CapabilityRequiredFields    []string `json:"capability_required_fields"`
	VideoResponseRequiredFields []string `json:"video_response_required_fields"`
	VideoRefundStatusValues     []string `json:"video_refund_status_values"`
}

func TestYingzoAgentContractFixtureMatchesService(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/yingzo-agent-api-contract.json")
	require.NoError(t, err)
	var fixture yingzoAgentContractFixture
	require.NoError(t, json.Unmarshal(raw, &fixture))
	require.Equal(t, AgentAPIContractVersion, fixture.AgentAPIContractVersion)

	capabilities := AgentCapabilities()
	models := make([]string, 0, len(capabilities))
	for model := range capabilities {
		models = append(models, model)
	}
	sort.Strings(models)
	require.Equal(t, fixture.Models, models)

	encodedCapability, err := json.Marshal(capabilities["gemini-3.5-flash"])
	require.NoError(t, err)
	var capabilityFields map[string]any
	require.NoError(t, json.Unmarshal(encodedCapability, &capabilityFields))
	for _, field := range fixture.CapabilityRequiredFields {
		require.Contains(t, capabilityFields, field)
	}

	encodedVideo, err := json.Marshal(videoResponseFromTask(&VideoTask{
		PublicID:  "video_contract",
		Model:     VideoModelSeedance20,
		Status:    VideoTaskStatusQueued,
		CreatedAt: time.Unix(1782700000, 0),
	}))
	require.NoError(t, err)
	var videoFields map[string]any
	require.NoError(t, json.Unmarshal(encodedVideo, &videoFields))
	for _, field := range fixture.VideoResponseRequiredFields {
		require.Contains(t, videoFields, field)
	}
	require.Equal(t, []string{
		VideoRefundStatusNotApplicable,
		VideoRefundStatusPending,
		VideoRefundStatusRefunded,
	}, fixture.VideoRefundStatusValues)
}

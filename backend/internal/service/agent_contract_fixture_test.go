package service

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type yingzoAgentContractFixture struct {
	AgentAPIContractVersion     string   `json:"agent_api_contract_version"`
	Models                      []string `json:"models"`
	Endpoints                   []string `json:"endpoints"`
	VideoResponseRequiredFields []string `json:"video_response_required_fields"`
	VideoRefundStatusValues     []string `json:"video_refund_status_values"`
}

func TestYingzoAgentContractFixtureMatchesService(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/yingzo-agent-api-contract.json")
	require.NoError(t, err)
	var fixture yingzoAgentContractFixture
	require.NoError(t, json.Unmarshal(raw, &fixture))
	require.Equal(t, AgentAPIContractVersion, fixture.AgentAPIContractVersion)

	require.Equal(t, fixture.Models, YingzoAgentModelIDs())
	require.Contains(t, fixture.Endpoints, "GET /v1/models")
	require.NotContains(t, fixture.Endpoints, "GET /api/v1/agent/models/:id/capabilities")
	require.NotContains(t, fixture.Endpoints, "POST /api/v1/agent/media/preflight")

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

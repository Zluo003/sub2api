package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestYingzoMigrationsAreForwardOnly(t *testing.T) {
	for _, name := range []string{
		"161_yingzo_agent_foundations.sql",
		"162_yingzo_agent_generation_quotes.sql",
		"163_yingzo_temporary_asset_metadata.sql",
		"164_yingzo_agent_group_visibility.sql",
		"165_yingzo_agent_group_private.sql",
		"166_yingzo_plugin_releases.sql",
		"167_yingzo_agent_model_pricing.sql",
	} {
		content, err := FS.ReadFile(name)
		require.NoError(t, err)
		sql := strings.ToLower(string(content))
		require.NotContains(t, sql, "+goose down", "%s must use the repository's forward-only migration format", name)
		require.NotContains(t, sql, "drop table", "%s must not contain rollback DDL", name)
	}
}

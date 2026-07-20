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
		"169_require_agent_image_generation.sql",
		"170_yingzo_release_artifacts.sql",
		"171_yingzo_distribution_v2.sql",
		"172_yingzo_prerelease_native_signing.sql",
		"173_yingzo_distribution_schema_3.sql",
		"174_yingzo_release_upload_sessions.sql",
	} {
		content, err := FS.ReadFile(name)
		require.NoError(t, err)
		sql := strings.ToLower(string(content))
		require.NotContains(t, sql, "+goose down", "%s must use the repository's forward-only migration format", name)
		require.NotContains(t, sql, "drop table", "%s must not contain rollback DDL", name)
	}
}

func TestYingzoReleaseUploadSessionsKeepBytesOutOfPostgres(t *testing.T) {
	content, err := FS.ReadFile("174_yingzo_release_upload_sessions.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))

	require.Contains(t, sql, "temp_storage_key text not null")
	require.Contains(t, sql, "received_bytes bigint")
	require.Contains(t, sql, "unique (release_id, target, os, arch)")
	require.Contains(t, sql, "completed_artifact_id uuid references yingzo_release_artifacts(id) on delete set null")
	require.Contains(t, sql, "'completed'")
	require.NotContains(t, sql, "bytea")
	require.NotContains(t, sql, "drop table")
}

func TestYingzoDistributionSchema3MigrationKeepsSchema2AndAddsSchema3(t *testing.T) {
	content, err := FS.ReadFile("173_yingzo_distribution_schema_3.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))
	require.Contains(t, sql, "drop constraint if exists yingzo_releases_distribution_schema_version_check")
	require.Contains(t, sql, "distribution_schema_version in (1, 2, 3)")
	require.Contains(t, sql, "distribution_schema_version in (2, 3) and runtime_protocol > 0")
	require.NotContains(t, sql, "drop table")
	require.NotContains(t, sql, "bytea")
}

func TestYingzoPrereleaseNativeSigningMigrationKeepsStableStrict(t *testing.T) {
	content, err := FS.ReadFile("172_yingzo_prerelease_native_signing.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))

	require.Contains(t, sql, "stable_eligible boolean not null default true")
	require.Contains(t, sql, "where distribution_schema_version = 2")
	require.Contains(t, sql, "and status = 'draft'")
	require.Contains(t, sql, "status <> 'published' or channel <> 'stable' or stable_eligible")
	require.NotContains(t, sql, "bytea")
}

func TestYingzoDistributionV2MigrationPreservesLegacyAndAddsChannelMatrix(t *testing.T) {
	content, err := FS.ReadFile("171_yingzo_distribution_v2.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))

	require.Contains(t, sql, "distribution_schema_version")
	require.Contains(t, sql, "channel in ('stable', 'prerelease')")
	require.Contains(t, sql, "runtime_protocol")
	require.Contains(t, sql, "compatibility jsonb")
	require.Contains(t, sql, "drop index if exists idx_yingzo_releases_single_published")
	require.Contains(t, sql, "where status = 'published'")
	require.Contains(t, sql, "unique (release_id, target, os, arch)")
	require.Contains(t, sql, "set artifact_kind = coalesce(artifact_kind, 'host_package')")
	require.Contains(t, sql, "target = coalesce(target, host_family)")
	require.Contains(t, sql, "signature_status")
	require.NotContains(t, sql, "bytea", "release binaries must remain on the local persistent volume, not in PostgreSQL")
}

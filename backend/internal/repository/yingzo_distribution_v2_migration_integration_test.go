//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestYingzoDistributionV2MigrationBackfillsLegacyAndPublishesPerChannel(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	schema := "yingzo_v2_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = tx.ExecContext(ctx, `CREATE SCHEMA `+schema)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `SET LOCAL search_path TO `+schema)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `CREATE TABLE users (id BIGINT PRIMARY KEY)`)
	require.NoError(t, err)

	for _, migrationName := range []string{"166_yingzo_plugin_releases.sql", "170_yingzo_release_artifacts.sql"} {
		content, readErr := migrations.FS.ReadFile(migrationName)
		require.NoError(t, readErr)
		_, err = tx.ExecContext(ctx, string(content))
		require.NoError(t, err, migrationName)
	}

	legacyID := uuid.New()
	_, err = tx.ExecContext(ctx, `INSERT INTO yingzo_releases(id,version,status,package_filename,storage_backend,storage_key,size_bytes,sha256,published_at) VALUES($1,'0.2.4','published','yingzo-private-0.2.4.tar.gz','local','/volume/legacy.tar.gz',10,$2,NOW())`, legacyID, strings.Repeat("a", 64))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO yingzo_release_artifacts(id,release_id,host_family,package_filename,storage_backend,storage_key,size_bytes,sha256) VALUES($1,$2,'combined','yingzo-private-0.2.4.tar.gz','local','/volume/legacy.tar.gz',10,$3)`, uuid.New(), legacyID, strings.Repeat("a", 64))
	require.NoError(t, err)

	content, err := migrations.FS.ReadFile("171_yingzo_distribution_v2.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err)

	var schemaVersion, runtimeProtocol int
	var channel string
	var compatibility string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT distribution_schema_version,channel,runtime_protocol,compatibility::text FROM yingzo_releases WHERE id=$1`, legacyID).Scan(&schemaVersion, &channel, &runtimeProtocol, &compatibility))
	require.Equal(t, 1, schemaVersion)
	require.Equal(t, "stable", channel)
	require.Zero(t, runtimeProtocol)
	require.Equal(t, "{}", compatibility)

	var kind, target, targetOS, arch, format, contentType, validationStatus string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT artifact_kind,target,os,arch,format,content_type,validation_status FROM yingzo_release_artifacts WHERE release_id=$1`, legacyID).Scan(&kind, &target, &targetOS, &arch, &format, &contentType, &validationStatus))
	require.Equal(t, "host_package", kind)
	require.Equal(t, "combined", target)
	require.Equal(t, "any", targetOS)
	require.Equal(t, "any", arch)
	require.Equal(t, "tar.gz", format)
	require.Equal(t, "application/gzip", contentType)
	require.Equal(t, "validated", validationStatus)

	prereleaseID := uuid.New()
	_, err = tx.ExecContext(ctx, `INSERT INTO yingzo_releases(id,version,status,distribution_schema_version,channel,runtime_protocol,compatibility,published_at) VALUES($1,'0.3.0','published',2,'prerelease',1,'{}',NOW())`, prereleaseID)
	require.NoError(t, err, "one published stable and one published prerelease must coexist")

	_, err = tx.ExecContext(ctx, `SAVEPOINT duplicate_stable`)
	require.NoError(t, err)
	_, duplicateErr := tx.ExecContext(ctx, `INSERT INTO yingzo_releases(id,version,status,channel) VALUES($1,'0.2.5','published','stable')`, uuid.New())
	require.Error(t, duplicateErr)
	_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT duplicate_stable`)
	require.NoError(t, err)

	var indexCount int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pg_indexes WHERE schemaname=$1 AND indexname='idx_yingzo_releases_single_published_per_channel'`, schema).Scan(&indexCount))
	require.Equal(t, 1, indexCount, fmt.Sprintf("channel publication index missing in %s", schema))
}

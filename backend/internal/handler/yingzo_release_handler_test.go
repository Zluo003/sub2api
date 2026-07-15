package handler

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestValidateYingzoOrigin(t *testing.T) {
	require.Equal(t, "https://api-key.cc", mustYingzoOrigin(t, "https://api-key.cc/"))
	require.Equal(t, "http://127.0.0.1:8080", mustYingzoOrigin(t, "http://127.0.0.1:8080"))
	_, err := validateYingzoOrigin("http://example.com")
	require.ErrorContains(t, err, "HTTPS")
	_, err = validateYingzoOrigin("https://example.com/path")
	require.Error(t, err)
}

func TestTemporaryAssetPublicURLUsesCleanExtensionPath(t *testing.T) {
	id := uuid.New()
	got := temporaryAssetPublicURL("https://api-key.cc", id, "image/png")
	require.Equal(t, "https://api-key.cc/media/"+id.String()+"/asset.png", got)
	require.NotContains(t, got, "?")
	require.Equal(t, ".mp4", canonicalMediaExtension("video/mp4"))
	require.Equal(t, ".mp3", canonicalMediaExtension("audio/mpeg"))
}

func TestYingzoInstallPromptUsesVerifiedHostCommands(t *testing.T) {
	release := &yingzoRelease{Version: "0.1.3", PackageFilename: "yingzo-private-0.1.3.tar.gz", SHA256: strings.Repeat("a", 64)}
	codex := yingzoInstallPrompt("codex", release, "https://api-key.cc/download/package.tar.gz")
	require.Contains(t, codex, "codex plugin marketplace add")
	require.Contains(t, codex, "codex plugin add yingzo@yingzo-private --json")
	require.Contains(t, codex, "保留现有 ~/.yingzo/auth.json")
	require.Contains(t, codex, "yingzo-private-0.1.3/marketplace")
	require.NotContains(t, codex, "private-beta")
	claude := yingzoInstallPrompt("claude-code", release, "https://api-key.cc/download/package.tar.gz")
	require.Contains(t, claude, "claude plugin marketplace add")
	require.Contains(t, claude, "claude plugin install yingzo@yingzo-private --scope user")
	require.Contains(t, claude, "yingzo-private-0.1.3/marketplace")
	require.NotContains(t, claude, "private-beta")
}

func TestValidateYingzoArchiveRequiresBothMarketplacesAndDistribution(t *testing.T) {
	version := "0.1.3"
	packageFilename := "yingzo-private-" + version + ".tar.gz"
	root := strings.TrimSuffix(packageFilename, ".tar.gz") + "/marketplace/"
	archive := filepath.Join(t.TempDir(), "yingzo.tar.gz")
	writeYingzoTestArchive(t, archive, map[string]string{
		root + ".agents/plugins/marketplace.json":         "{}",
		root + ".claude-plugin/marketplace.json":          "{}",
		root + "plugins/yingzo/.codex-plugin/plugin.json": "{}",
		root + "plugins/yingzo/distribution.json":         "{}",
	})
	require.NoError(t, validateYingzoArchive(archive, packageFilename))

	unsafe := filepath.Join(t.TempDir(), "unsafe.tar.gz")
	writeYingzoTestArchive(t, unsafe, map[string]string{"../escape": "bad"})
	require.ErrorContains(t, validateYingzoArchive(unsafe, packageFilename), "unsafe path")
}

func TestYingzoLegacyBetaArchiveRemainsCompatible(t *testing.T) {
	version := "0.1.2"
	packageFilename := "yingzo-private-beta-" + version + ".tar.gz"
	root := strings.TrimSuffix(packageFilename, ".tar.gz") + "/marketplace/"
	archive := filepath.Join(t.TempDir(), packageFilename)
	writeYingzoTestArchive(t, archive, map[string]string{
		root + ".agents/plugins/marketplace.json":         "{}",
		root + ".claude-plugin/marketplace.json":          "{}",
		root + "plugins/yingzo/.codex-plugin/plugin.json": "{}",
		root + "plugins/yingzo/distribution.json":         "{}",
	})
	require.NoError(t, validateYingzoArchive(archive, packageFilename))
	require.Contains(t, yingzoInstallPrompt("codex", &yingzoRelease{
		Version: version, PackageFilename: packageFilename, SHA256: strings.Repeat("b", 64),
	}, "https://api-key.cc/download/legacy.tar.gz"), "yingzo-private-beta-0.1.2/marketplace")
}

func TestValidateYingzoPackageFilename(t *testing.T) {
	stable, err := validateYingzoPackageFilename("yingzo-private-0.1.3.tar.gz", "0.1.3")
	require.NoError(t, err)
	require.Equal(t, "yingzo-private-0.1.3.tar.gz", stable)

	legacy, err := validateYingzoPackageFilename("yingzo-private-beta-0.1.2.tar.gz", "0.1.2")
	require.NoError(t, err)
	require.Equal(t, "yingzo-private-beta-0.1.2.tar.gz", legacy)

	_, err = validateYingzoPackageFilename("sub2api-linux-amd64.tar.gz", "0.1.3")
	require.Error(t, err)
}

func mustYingzoOrigin(t *testing.T, raw string) string {
	t.Helper()
	value, err := validateYingzoOrigin(raw)
	require.NoError(t, err)
	return value
}

func writeYingzoTestArchive(t *testing.T, target string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(target)
	require.NoError(t, err)
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range entries {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(content)), Typeflag: tar.TypeReg}))
		_, err = tarWriter.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	require.NoError(t, file.Close())
}

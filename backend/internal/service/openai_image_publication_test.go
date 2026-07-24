package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type recordingImagePublisher struct {
	urls    []string
	inputs  []string
	formats []string
	err     error
}

func (p *recordingImagePublisher) PublishGeneratedImage(
	_ context.Context,
	_ TemporaryAssetOwner,
	_ string,
	encodedImage string,
	outputFormat string,
) (string, error) {
	p.inputs = append(p.inputs, encodedImage)
	p.formats = append(p.formats, outputFormat)
	if p.err != nil {
		return "", p.err
	}
	index := len(p.inputs) - 1
	if index < len(p.urls) {
		return p.urls[index], nil
	}
	return "https://media.example.com/media/result/asset.png", nil
}

func imagePublicationTestContext(publisher OpenAIImageResultPublisher) context.Context {
	return WithOpenAIImageURLPublication(
		context.Background(),
		publisher,
		TemporaryAssetOwner{UserID: 11, APIKeyID: 22, GroupID: 33},
		"https://api.example.com",
	)
}

func TestTransformOpenAIImagesURLResponsePublishesBase64(t *testing.T) {
	publisher := &recordingImagePublisher{urls: []string{"https://api.example.com/media/one/asset.png"}}
	body := []byte(`{"created":1,"output_format":"png","data":[{"b64_json":"aW1hZ2U=","revised_prompt":"kept"}]}`)

	transformed, err := transformOpenAIImagesURLResponse(imagePublicationTestContext(publisher), body)
	require.NoError(t, err)
	require.Equal(t, "https://api.example.com/media/one/asset.png", gjson.GetBytes(transformed, "data.0.url").String())
	require.False(t, gjson.GetBytes(transformed, "data.0.b64_json").Exists())
	require.Equal(t, "kept", gjson.GetBytes(transformed, "data.0.revised_prompt").String())
	require.NotContains(t, string(transformed), "data:image/")
	require.Equal(t, []string{"aW1hZ2U="}, publisher.inputs)
	require.Equal(t, []string{"png"}, publisher.formats)
}

func TestTransformOpenAIImagesURLResponsePreservesHTTPURL(t *testing.T) {
	publisher := &recordingImagePublisher{}
	body := []byte(`{"data":[{"url":"https://cdn.example.com/result.png"}]}`)

	transformed, err := transformOpenAIImagesURLResponse(imagePublicationTestContext(publisher), body)
	require.NoError(t, err)
	require.JSONEq(t, string(body), string(transformed))
	require.Empty(t, publisher.inputs)
}

func TestOpenAIImagesURLStreamSkipsPartialAndPublishesCompleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	publisher := &recordingImagePublisher{urls: []string{"https://api.example.com/media/final/asset.png"}}
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	request = request.WithContext(imagePublicationTestContext(publisher))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = request

	upstream := strings.Join([]string{
		"event: image_generation.partial_image\n",
		"data: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"cGFydGlhbA==\"}\n",
		"\n",
		"event: image_generation.completed\n",
		"data: {\"type\":\"image_generation.completed\",\"b64_json\":\"ZmluYWw=\",\"output_format\":\"png\"}\n",
		"\n",
	}, "")
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstream)),
	}

	service := &OpenAIGatewayService{}
	_, count, _, _, err := service.handleOpenAIImagesStreamingResponse(response, c, time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NotContains(t, recorder.Body.String(), "partial_image")
	require.NotContains(t, recorder.Body.String(), "b64_json")
	require.NotContains(t, recorder.Body.String(), "data:image/")
	require.Contains(t, recorder.Body.String(), "https://api.example.com/media/final/asset.png")
	require.Equal(t, []string{"ZmluYWw="}, publisher.inputs)
}

func TestOpenAIImagesURLPublicationFailureReturnsBillableResultWithoutFalseSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	publisher := &recordingImagePublisher{err: errors.New("storage unavailable")}
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	request = request.WithContext(imagePublicationTestContext(publisher))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = request
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"created":1,"data":[{"b64_json":"ZmluYWw="}]}`)),
	}

	service := &OpenAIGatewayService{}
	_, count, _, err := service.handleOpenAIImagesNonStreamingResponse(response, c)
	require.ErrorIs(t, err, ErrOpenAIImagePublication)
	require.Equal(t, 1, count)
	require.Empty(t, recorder.Body.String())
}

func TestTemporaryAssetPublisherStoresGeneratedPNG(t *testing.T) {
	for _, key := range []string{
		"FILE_SERVICE_PUBLIC_BASE_URL", "FILE_SERVICE_RETENTION_HOURS",
		"FILE_SERVICE_DAILY_MAX_COUNT", "FILE_SERVICE_DAILY_MAX_BYTES",
		"FILE_SERVICE_S3_BUCKET", "AGENT_ASSETS_S3_BUCKET",
	} {
		t.Setenv(key, "")
	}
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	dataDir := t.TempDir()
	storage := NewFileStorageService(
		db,
		nil,
		nil,
		nil,
		&config.Config{Pricing: config.PricingConfig{DataDir: dataDir}},
	)
	publisher := NewTemporaryAssetPublisher(db, storage)
	pngBytes := onePixelPNG(t)
	encoded := base64.StdEncoding.EncodeToString(pngBytes)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\), COALESCE\\(SUM\\(size_bytes\\), 0\\)").
		WithArgs(int64(22), int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"count", "bytes"}).AddRow(0, 0))
	mock.ExpectExec("INSERT INTO temporary_assets").
		WithArgs(
			sqlmock.AnyArg(), int64(11), int64(22), int64(33), sqlmock.AnyArg(),
			"local", sqlmock.AnyArg(), sqlmock.AnyArg(), "image", "image/png",
			int64(len(pngBytes)), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	resultURL, err := publisher.PublishGeneratedImage(
		context.Background(),
		TemporaryAssetOwner{UserID: 11, APIKeyID: 22, GroupID: 33},
		"https://api.example.com",
		encoded,
		"png",
	)
	require.NoError(t, err)
	require.Regexp(t, `^https://api\.example\.com/media/[0-9a-f-]+/asset\.png$`, resultURL)
	require.NoError(t, mock.ExpectationsWereMet())

	entries, err := os.ReadDir(storage.LocalPath())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	stored, err := os.ReadFile(filepath.Join(storage.LocalPath(), entries[0].Name(), "object"))
	require.NoError(t, err)
	require.Equal(t, pngBytes, stored)
}

func onePixelPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	require.NoError(t, png.Encode(&buffer, img))
	return buffer.Bytes()
}

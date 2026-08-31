package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ycyapiMaxImageBytes         int64 = 30_000_000
	ycyapiMaxAudioBytes         int64 = 15_000_000
	ycyapiMaxFireflyVideoBytes  int64 = 50_000_000
	ycyapiMaxLeonardoVideoBytes int64 = 200_000_000
)

func (s *VideoService) createYCYAPIUpstreamTask(ctx context.Context, account *Account, body map[string]any) (*videoUpstreamCreateResult, error) {
	endpoint, err := videoAccountEndpoint(account)
	if err != nil {
		return nil, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, videoAccountDuration(account, "request_timeout_ms", videoAccountDefaultDuration(account, "request_timeout_ms")))
	defer cancel()

	requestBody, contentType, uploadDone, err := s.buildYCYAPICreateRequest(reqCtx, body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, requestBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(account.GetCredential("api_key")))
	req.Header.Set("Content-Type", contentType)

	resp, requestErr := s.doUpstream(req, account)
	if uploadDone != nil {
		if closer, ok := requestBody.(io.Closer); ok {
			_ = closer.Close()
		}
		if uploadErr := <-uploadDone; uploadErr != nil && requestErr == nil {
			if resp != nil {
				_ = resp.Body.Close()
			}
			return nil, &videoUpstreamError{StatusCode: 0, Err: uploadErr}
		}
	}
	if requestErr != nil {
		return nil, &videoUpstreamError{StatusCode: 0, Err: requestErr}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &videoUpstreamError{StatusCode: resp.StatusCode, Body: respBody}
	}
	var payload map[string]any
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, &videoUpstreamError{StatusCode: resp.StatusCode, Body: respBody, Err: err}
	}
	id := firstNonEmptyVideoString(stringFromMap(payload, "task_id"), stringFromMap(payload, "id"))
	if id == "" {
		return nil, &videoUpstreamError{StatusCode: resp.StatusCode, Body: respBody, Err: errors.New("missing upstream task id")}
	}
	return &videoUpstreamCreateResult{ID: id}, nil
}

func (s *VideoService) buildYCYAPICreateRequest(ctx context.Context, body map[string]any) (io.Reader, string, <-chan error, error) {
	cleanBody := cloneMap(body)
	if cleanBody == nil {
		cleanBody = map[string]any{}
	}
	media := ycyapiContentFromAny(body[ycyapiContentBodyKey])
	delete(cleanBody, ycyapiContentBodyKey)
	if len(media) == 0 {
		raw, err := json.Marshal(cleanBody)
		return bytes.NewReader(raw), "application/json", nil, err
	}

	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	contentType := multipartWriter.FormDataContentType()
	done := make(chan error, 1)
	go func() {
		err := s.writeYCYAPIMultipart(ctx, multipartWriter, cleanBody, media)
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
		done <- err
	}()
	return reader, contentType, done, nil
}

func (s *VideoService) writeYCYAPIMultipart(ctx context.Context, writer *multipart.Writer, fields map[string]any, media []VideoContent) error {
	keys := make([]string, 0, len(fields))
	for key, value := range fields {
		if value != nil {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		value, err := ycyapiFormValue(fields[key])
		if err != nil {
			return err
		}
		if err := writer.WriteField(key, value); err != nil {
			return err
		}
	}

	upstreamModel, _ := fields["model"].(string)
	var videoBytes int64
	for index, item := range media {
		field, rawURL := ycyapiMediaFieldAndURL(item)
		if field == "" || rawURL == "" {
			return fmt.Errorf("invalid YCYAPI media reference at index %d", index)
		}
		response, err := s.fetchYCYAPIReference(ctx, rawURL)
		if err != nil {
			return err
		}
		limit := ycyapiReferenceByteLimit(upstreamModel, field)
		if response.ContentLength > limit && response.ContentLength >= 0 {
			_ = response.Body.Close()
			return fmt.Errorf("YCYAPI %s reference exceeds the size limit", field)
		}
		buffered := bufio.NewReader(response.Body)
		sample, _ := buffered.Peek(512)
		mediaType := ycyapiReferenceMediaType(response.Header.Get("Content-Type"), sample)
		filename := ycyapiReferenceFilename(rawURL, field, mediaType, index)
		part, err := writer.CreatePart(ycyapiFileHeader(field, filename, mediaType))
		if err != nil {
			_ = response.Body.Close()
			return err
		}
		written, copyErr := io.Copy(part, io.LimitReader(buffered, limit+1))
		closeErr := response.Body.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if written == 0 || written > limit {
			return fmt.Errorf("YCYAPI %s reference exceeds the size limit", field)
		}
		if field == "videos" {
			videoBytes += written
			if videoBytes > limit {
				return errors.New("YCYAPI video references exceed the aggregate size limit")
			}
		}
	}
	return nil
}

func (s *VideoService) fetchYCYAPIReference(ctx context.Context, rawURL string) (*http.Response, error) {
	client := s.videoReferenceClient
	if client == nil {
		if err := validateGeneratedVideoURL(ctx, rawURL, false); err != nil {
			return nil, errors.New("YCYAPI media reference URL is invalid")
		}
		client = newSSRFSafeHTTPClient(10 * time.Minute)
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("YCYAPI media reference exceeded the redirect limit")
			}
			return validateGeneratedVideoURL(req.Context(), req.URL.String(), false)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(rawURL), nil)
	if err != nil {
		return nil, errors.New("YCYAPI media reference URL is invalid")
	}
	req.Header.Set("Accept", "image/*,video/*,audio/*,application/octet-stream;q=0.8")
	req.Header.Set("User-Agent", "Sub2API-YCYAPI-Adapter/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("YCYAPI media reference download failed")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("YCYAPI media reference download returned HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func ycyapiContentFromAny(value any) []VideoContent {
	switch typed := value.(type) {
	case []VideoContent:
		return typed
	default:
		return nil
	}
}

func ycyapiMediaFieldAndURL(item VideoContent) (string, string) {
	switch item.Type {
	case "image_url":
		if item.ImageURL == nil {
			return "", ""
		}
		field := "images"
		if item.Role == "first_frame" {
			field = "first_frame"
		} else if item.Role == "last_frame" {
			field = "last_frame"
		}
		return field, strings.TrimSpace(item.ImageURL.URL)
	case "video_url":
		if item.VideoURL != nil {
			return "videos", strings.TrimSpace(item.VideoURL.URL)
		}
	case "audio_url":
		if item.AudioURL != nil {
			return "audios", strings.TrimSpace(item.AudioURL.URL)
		}
	}
	return "", ""
}

func ycyapiReferenceByteLimit(upstreamModel, field string) int64 {
	switch field {
	case "first_frame", "last_frame", "images":
		return ycyapiMaxImageBytes
	case "audios":
		return ycyapiMaxAudioBytes
	case "videos":
		if strings.TrimSpace(upstreamModel) == videoYCYAPISeedance25Model {
			return ycyapiMaxLeonardoVideoBytes
		}
		return ycyapiMaxFireflyVideoBytes
	default:
		return ycyapiMaxImageBytes
	}
}

func ycyapiFormValue(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case bool:
		return strconv.FormatBool(typed), nil
	case int:
		return strconv.Itoa(typed), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case json.Number:
		return typed.String(), nil
	default:
		raw, err := json.Marshal(value)
		return string(raw), err
	}
}

func ycyapiReferenceMediaType(declared string, sample []byte) string {
	if parsed, _, err := mime.ParseMediaType(declared); err == nil && parsed != "" && parsed != "application/octet-stream" {
		return strings.ToLower(parsed)
	}
	if len(sample) > 0 {
		return http.DetectContentType(sample)
	}
	return "application/octet-stream"
}

func ycyapiReferenceFilename(rawURL, field, mediaType string, index int) string {
	parsed, _ := url.Parse(rawURL)
	filename := path.Base(parsed.Path)
	if filename == "" || filename == "." || filename == "/" {
		filename = field + "-" + strconv.Itoa(index+1)
	}
	filename = strings.ReplaceAll(strings.ReplaceAll(filename, "\r", ""), "\n", "")
	if path.Ext(filename) == "" {
		if extensions, _ := mime.ExtensionsByType(mediaType); len(extensions) > 0 {
			filename += extensions[0]
		}
	}
	return filename
}

func ycyapiFileHeader(field, filename, mediaType string) textproto.MIMEHeader {
	escape := func(value string) string {
		value = strings.ReplaceAll(value, "\\", "\\\\")
		return strings.ReplaceAll(value, "\"", "\\\"")
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escape(field), escape(filename)))
	header.Set("Content-Type", mediaType)
	return header
}

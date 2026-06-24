package services

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const cloudinaryUploadBaseURL = "https://api.cloudinary.com/v1_1"

// CloudinaryStorageService uploads and deletes image assets in Cloudinary.
type CloudinaryStorageService struct {
	cloudName   string
	apiKey      string
	apiSecret   string
	environment string
	httpClient  *http.Client
}

// CloudinaryUploadResult captures the important pieces of a Cloudinary upload response.
type CloudinaryUploadResult struct {
	SecureURL string `json:"secure_url"`
	PublicID  string `json:"public_id"`
	AssetID   string `json:"asset_id"`
}

// NewCloudinaryStorageService builds a Cloudinary client from CLOUDINARY_URL.
func NewCloudinaryStorageService(cloudinaryURL, environment string) (*CloudinaryStorageService, error) {
	if strings.TrimSpace(cloudinaryURL) == "" {
		return nil, errors.New("CLOUDINARY_URL is required")
	}

	parsedURL, err := url.Parse(cloudinaryURL)
	if err != nil {
		return nil, fmt.Errorf("invalid CLOUDINARY_URL: %w", err)
	}

	if parsedURL.Scheme != "cloudinary" {
		return nil, fmt.Errorf("invalid CLOUDINARY_URL scheme: %s", parsedURL.Scheme)
	}

	apiKey := ""
	apiSecret := ""
	if parsedURL.User != nil {
		apiKey = cleanCloudinaryValue(parsedURL.User.Username())
		apiSecret, _ = parsedURL.User.Password()
		apiSecret = cleanCloudinaryValue(apiSecret)
	}
	cloudName := cleanCloudinaryValue(parsedURL.Host)

	if apiKey == "" || apiSecret == "" || cloudName == "" {
		return nil, errors.New("CLOUDINARY_URL must include api key, api secret, and cloud name")
	}

	return &CloudinaryStorageService{
		cloudName:   cloudName,
		apiKey:      apiKey,
		apiSecret:   apiSecret,
		environment: normalizeSegment(environment, "development"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (s *CloudinaryStorageService) UploadLoungePhoto(ctx context.Context, file multipart.File, originalFilename, loungeID string) (CloudinaryUploadResult, error) {
	return s.uploadImage(ctx, file, originalFilename, "lounge-photos", loungeID, "gallery")
}

func (s *CloudinaryStorageService) UploadProductImage(ctx context.Context, file multipart.File, originalFilename, loungeID string) (CloudinaryUploadResult, error) {
	return s.uploadImage(ctx, file, originalFilename, "lounge-products", loungeID, "product")
}

func (s *CloudinaryStorageService) UploadSpecialPackageImage(ctx context.Context, file multipart.File, originalFilename, loungeID string) (CloudinaryUploadResult, error) {
	return s.uploadImage(ctx, file, originalFilename, "lounge-special-packages", loungeID, "package")
}

func (s *CloudinaryStorageService) UploadNICImage(ctx context.Context, file multipart.File, originalFilename, userID, side string) (CloudinaryUploadResult, error) {
	return s.uploadImage(ctx, file, originalFilename, "lounge-owner-nic", userID, normalizeSegment(side, "front"))
}

func (s *CloudinaryStorageService) UploadProfilePhoto(ctx context.Context, file multipart.File, originalFilename, userID string) (CloudinaryUploadResult, error) {
	return s.uploadImage(ctx, file, originalFilename, "profile-photos", userID, "avatar")
}

func (s *CloudinaryStorageService) DeleteImageByURL(ctx context.Context, imageURL string) error {
	publicID, err := extractPublicIDFromURL(imageURL)
	if err != nil {
		return err
	}

	form := url.Values{}
	form.Set("public_id", publicID)
	form.Set("timestamp", strconv.FormatInt(time.Now().Unix(), 10))

	signature := s.signParams(map[string]string{
		"public_id": publicID,
		"timestamp": form.Get("timestamp"),
	})
	form.Set("api_key", s.apiKey)
	form.Set("signature", signature)

	endpoint := fmt.Sprintf("%s/%s/image/destroy", cloudinaryUploadBaseURL, s.cloudName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build cloudinary delete request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete cloudinary image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cloudinary delete failed: %s", strings.TrimSpace(string(body)))
	}

	return nil
}

func (s *CloudinaryStorageService) uploadImage(ctx context.Context, file multipart.File, originalFilename, folderPrefix, entityID, variant string) (CloudinaryUploadResult, error) {
	if file == nil {
		return CloudinaryUploadResult{}, errors.New("image file is required")
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("read upload file: %w", err)
	}

	folder := s.buildFolder(folderPrefix, entityID, variant)
	publicID := fmt.Sprintf("%d-%s", time.Now().UnixNano(), uuid.NewString())

	params := map[string]string{
		"folder":          folder,
		"overwrite":       "false",
		"public_id":       publicID,
		"timestamp":       strconv.FormatInt(time.Now().Unix(), 10),
		"unique_filename": "false",
	}
	signature := s.signParams(params)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("api_key", s.apiKey); err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("write api_key field: %w", err)
	}
	if err := writer.WriteField("timestamp", params["timestamp"]); err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("write timestamp field: %w", err)
	}
	if err := writer.WriteField("signature", signature); err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("write signature field: %w", err)
	}
	if err := writer.WriteField("folder", folder); err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("write folder field: %w", err)
	}
	if err := writer.WriteField("public_id", publicID); err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("write public_id field: %w", err)
	}
	if err := writer.WriteField("overwrite", "false"); err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("write overwrite field: %w", err)
	}
	if err := writer.WriteField("unique_filename", "false"); err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("write unique_filename field: %w", err)
	}

	part, err := writer.CreateFormFile("file", normalizeSegment(originalFilename, "image.jpg"))
	if err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("create file part: %w", err)
	}
	if _, err := part.Write(content); err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("write file content: %w", err)
	}

	if err := writer.Close(); err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("close multipart writer: %w", err)
	}

	endpoint := fmt.Sprintf("%s/%s/image/upload", cloudinaryUploadBaseURL, s.cloudName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("build cloudinary upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("upload to cloudinary: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("read cloudinary response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return CloudinaryUploadResult{}, fmt.Errorf("cloudinary upload failed: %s", strings.TrimSpace(string(responseBody)))
	}

	var result struct {
		SecureURL string `json:"secure_url"`
		PublicID  string `json:"public_id"`
		AssetID   string `json:"asset_id"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return CloudinaryUploadResult{}, fmt.Errorf("decode cloudinary response: %w", err)
	}

	return CloudinaryUploadResult{
		SecureURL: result.SecureURL,
		PublicID:  result.PublicID,
		AssetID:   result.AssetID,
	}, nil
}

func (s *CloudinaryStorageService) buildFolder(folderPrefix, entityID, variant string) string {
	parts := []string{"smarttransit", s.environment, normalizeSegment(folderPrefix, "uploads"), normalizeSegment(entityID, "unknown")}
	if normalizedVariant := normalizeSegment(variant, ""); normalizedVariant != "" {
		parts = append(parts, normalizedVariant)
	}
	return strings.Join(parts, "/")
}

func (s *CloudinaryStorageService) signParams(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		if key == "file" || key == "api_key" || key == "signature" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for index, key := range keys {
		if index > 0 {
			builder.WriteByte('&')
		}
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(params[key])
	}
	builder.WriteString(s.apiSecret)
	checksum := sha1.Sum([]byte(builder.String()))
	return hex.EncodeToString(checksum[:])
}

func extractPublicIDFromURL(imageURL string) (string, error) {
	parsedURL, err := url.Parse(imageURL)
	if err != nil {
		return "", fmt.Errorf("parse cloudinary url: %w", err)
	}

	segments := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	uploadIndex := -1
	for index, segment := range segments {
		if segment == "upload" {
			uploadIndex = index
			break
		}
	}
	if uploadIndex == -1 || uploadIndex+1 >= len(segments) {
		return "", fmt.Errorf("unsupported cloudinary url format: %s", imageURL)
	}

	assetSegments := segments[uploadIndex+1:]
	for len(assetSegments) > 0 && isVersionSegment(assetSegments[0]) {
		assetSegments = assetSegments[1:]
	}
	if len(assetSegments) == 0 {
		return "", fmt.Errorf("unsupported cloudinary url format: %s", imageURL)
	}

	assetPath := strings.Join(assetSegments, "/")
	extension := path.Ext(assetPath)
	return strings.TrimSuffix(assetPath, extension), nil
}

func isVersionSegment(segment string) bool {
	if len(segment) < 2 || segment[0] != 'v' {
		return false
	}
	_, err := strconv.ParseInt(segment[1:], 10, 64)
	return err == nil
}

func normalizeSegment(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		trimmed = fallback
	}
	trimmed = strings.ReplaceAll(trimmed, " ", "-")
	trimmed = strings.ReplaceAll(trimmed, "/", "-")
	trimmed = strings.ReplaceAll(trimmed, "\\", "-")
	return trimmed
}

func cleanCloudinaryValue(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "<")
	trimmed = strings.TrimSuffix(trimmed, ">")
	return strings.TrimSpace(trimmed)
}

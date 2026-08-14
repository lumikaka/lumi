package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/sitesettings"

	"github.com/google/uuid"
)

const (
	TypeCloudflareAIGateway    = "cloudflare_ai_gateway"
	LegacyTypeOpenAICompatible = "openai_compatible"
	TypeAliyunBailian          = "aliyun_bailian"

	BailianTextModel          = "qwen3.7-plus"
	BailianTextModelQwen38Max = "qwen3.8-max"
	BailianImageModel         = "qwen-image-3.0"
)

type Provider struct {
	UUID              string     `json:"uuid"`
	ProviderType      string     `json:"provider_type"`
	DisplayName       string     `json:"display_name"`
	BaseURL           string     `json:"base_url"`
	ImageBaseURL      string     `json:"image_base_url"`
	DefaultModel      string     `json:"default_model"`
	DefaultImageModel string     `json:"default_image_model"`
	AccountID         string     `json:"account_id,omitempty"`
	Region            string     `json:"region,omitempty"`
	WorkspaceID       string     `json:"workspace_id,omitempty"`
	Enabled           bool       `json:"enabled"`
	Configured        bool       `json:"configured"`
	Verified          bool       `json:"verified"`
	Active            bool       `json:"active"`
	Ready             bool       `json:"ready"`
	HasSecret         bool       `json:"has_secret"`
	VerifiedAt        *time.Time `json:"verified_at"`
}

type Resolved struct {
	Provider
	APIKey            string `json:"-"`
	ConfigFingerprint string `json:"-"`
}

// SupportedTextModels returns the selectable text models for a provider, with its default first.
func SupportedTextModels(item Provider) []string {
	models := make([]string, 0, 2)
	defaultModel := strings.TrimSpace(item.DefaultModel)
	if defaultModel != "" {
		models = append(models, defaultModel)
	}
	if item.ProviderType == TypeAliyunBailian && defaultModel != BailianTextModelQwen38Max {
		models = append(models, BailianTextModelQwen38Max)
	}
	return models
}

// CreateInput remains an internal bootstrap helper for tests and local callers.
// Public provider creation was removed; production configuration uses site settings.
type CreateInput struct {
	ProviderType      string `json:"provider_type"`
	AccountID         string `json:"account_id"`
	DefaultModel      string `json:"default_model"`
	DefaultImageModel string `json:"default_image_model"`
	APIKey            string `json:"api_key"`
}

type Service struct {
	settings   *sitesettings.Service
	now        func() time.Time
	identityMu sync.Mutex
}

func NewService(app *appstore.Store, keys sitesettings.MasterKeyStore) *Service {
	return &Service{settings: sitesettings.NewService(app, keys), now: time.Now}
}

func (service *Service) Settings() *sitesettings.Service { return service.settings }

func (service *Service) Close() { service.settings.Close() }

func (service *Service) List(ctx context.Context) ([]Provider, error) {
	if err := service.ensureIdentities(ctx); err != nil {
		return nil, err
	}
	active, err := service.stringValue(ctx, sitesettings.ActiveProviderKey)
	if err != nil {
		return nil, err
	}
	active = normalizeProviderType(active)
	cloudflare, err := service.cloudflare(ctx, active)
	if err != nil {
		return nil, err
	}
	bailian, err := service.bailian(ctx, active)
	if err != nil {
		return nil, err
	}
	return []Provider{cloudflare, bailian}, nil
}

func (service *Service) Get(ctx context.Context, providerUUID string) (Provider, error) {
	if !isUUIDv7(providerUUID) {
		return Provider{}, providerError(CodeInvalidProvider, "Provider UUID 无效", "Provider 资源标识必须是 UUIDv7。", nil)
	}
	items, err := service.List(ctx)
	if err != nil {
		return Provider{}, err
	}
	for _, item := range items {
		if item.UUID == providerUUID {
			return item, nil
		}
	}
	return Provider{}, providerError(CodeProviderNotFound, "Provider 不存在", "该 Provider 类型不受支持。", nil)
}

func (service *Service) Active(ctx context.Context) (Resolved, error) {
	active, err := service.stringValue(ctx, sitesettings.ActiveProviderKey)
	if err != nil {
		return Resolved{}, err
	}
	active = normalizeProviderType(active)
	if active == "none" || active == "" {
		return Resolved{}, providerError(CodeNoActiveProvider, "尚未激活 AI Provider", "请先完成 Provider 配置、连接检查并设为当前 Provider。", nil)
	}
	items, err := service.List(ctx)
	if err != nil {
		return Resolved{}, err
	}
	for _, item := range items {
		if item.ProviderType == active {
			if !item.Ready {
				return Resolved{}, providerError(CodeProviderNotReady, "当前 AI Provider 不可用", "请检查配置、密钥和连接验证状态。", nil)
			}
			return service.Resolve(ctx, item.UUID)
		}
	}
	return Resolved{}, providerError(CodeNoActiveProvider, "尚未激活 AI Provider", "当前 Provider 类型无效。", nil)
}

func (service *Service) Resolve(ctx context.Context, providerUUID string) (Resolved, error) {
	item, err := service.Get(ctx, providerUUID)
	if err != nil {
		return Resolved{}, err
	}
	secretKey := sitesettings.CloudflareAPITokenKey
	if item.ProviderType == TypeAliyunBailian {
		secretKey = sitesettings.BailianAPIKeyKey
	}
	secret, err := service.settings.Secret(ctx, secretKey)
	if err != nil {
		return Resolved{}, providerError(CodeSecretStoreFailed, "无法读取 Provider 密钥", "AES 根密钥或加密设置当前不可用。", err)
	}
	if strings.TrimSpace(secret) == "" {
		return Resolved{}, providerError(CodeSecretMissing, "Provider 密钥缺失", "请在全局设置中保存 API Key。", nil)
	}
	if !item.Configured {
		return Resolved{}, providerError(CodeProviderNotReady, "Provider 配置不完整", "请补全 Provider 配置。", nil)
	}
	fingerprint, err := service.providerFingerprint(ctx, item.ProviderType)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{Provider: item, APIKey: secret, ConfigFingerprint: fingerprint}, nil
}

func (service *Service) Activate(ctx context.Context, providerType string) (Provider, error) {
	providerType = normalizeProviderType(providerType)
	if providerType != TypeCloudflareAIGateway && providerType != TypeAliyunBailian {
		return Provider{}, providerError(CodeInvalidProvider, "Provider 类型无效", "只能激活受支持的 Provider。", nil)
	}
	items, err := service.List(ctx)
	if err != nil {
		return Provider{}, err
	}
	var selected Provider
	for _, item := range items {
		if item.ProviderType == providerType {
			selected = item
			break
		}
	}
	if !selected.Ready {
		return Provider{}, providerError(CodeProviderNotReady, "Provider 尚未就绪", "必须先补全配置并通过连接检查。", nil)
	}
	if _, _, err := service.settings.Update(ctx, map[string]any{sitesettings.ActiveProviderKey: providerType}); err != nil {
		return Provider{}, err
	}
	selected.Active, selected.Enabled = true, true
	return selected, nil
}

func (service *Service) MarkVerified(ctx context.Context, providerUUID string, checkedFingerprints ...string) (Provider, error) {
	item, err := service.Get(ctx, providerUUID)
	if err != nil {
		return Provider{}, err
	}
	if !item.Configured || !item.HasSecret {
		return Provider{}, providerError(CodeProviderNotReady, "Provider 配置不完整", "请先保存完整配置和 API Key。", nil)
	}
	fingerprint := ""
	if len(checkedFingerprints) > 0 {
		fingerprint = strings.TrimSpace(checkedFingerprints[0])
	}
	if fingerprint == "" {
		fingerprint, err = service.providerFingerprint(ctx, item.ProviderType)
		if err != nil {
			return Provider{}, err
		}
	}
	verifiedKey, verifiedAtKey, fingerprintKey := sitesettings.CloudflareVerifiedKey, sitesettings.CloudflareVerifiedAtKey, sitesettings.CloudflareVerifiedFingerprintKey
	if item.ProviderType == TypeAliyunBailian {
		verifiedKey, verifiedAtKey, fingerprintKey = sitesettings.BailianVerifiedKey, sitesettings.BailianVerifiedAtKey, sitesettings.BailianVerifiedFingerprintKey
	}
	now := service.now().UTC()
	if _, _, err := service.settings.UpdateSystem(ctx, map[string]any{verifiedKey: true, verifiedAtKey: now.Format(time.RFC3339Nano), fingerprintKey: fingerprint}); err != nil {
		return Provider{}, err
	}
	return service.Get(ctx, providerUUID)
}

// Create configures the single Cloudflare AI Gateway slot for internal tests.
func (service *Service) Create(ctx context.Context, input CreateInput) (Provider, error) {
	providerType := strings.TrimSpace(input.ProviderType)
	if providerType == "" {
		providerType = TypeCloudflareAIGateway
	}
	providerType = normalizeProviderType(providerType)
	if providerType != TypeCloudflareAIGateway {
		return Provider{}, providerError(CodeInvalidProvider, "Provider 类型无效", "内部 bootstrap 仅支持 cloudflare_ai_gateway。", nil)
	}
	if err := service.ensureIdentities(ctx); err != nil {
		return Provider{}, err
	}
	imageModel := strings.TrimSpace(input.DefaultImageModel)
	if imageModel == "" {
		imageModel = input.DefaultModel
	}
	_, _, err := service.settings.Update(ctx, map[string]any{
		sitesettings.CloudflareAccountIDKey:         input.AccountID,
		sitesettings.CloudflareDefaultModelKey:      input.DefaultModel,
		sitesettings.CloudflareDefaultImageModelKey: imageModel,
		sitesettings.CloudflareAPITokenKey:          input.APIKey,
	})
	if err != nil {
		return Provider{}, err
	}
	item, err := service.cloudflare(ctx, "none")
	if err != nil {
		return Provider{}, err
	}
	if _, err := service.MarkVerified(ctx, item.UUID); err != nil {
		return Provider{}, err
	}
	return service.Activate(ctx, TypeCloudflareAIGateway)
}

func (service *Service) ensureIdentities(ctx context.Context) error {
	service.identityMu.Lock()
	defer service.identityMu.Unlock()
	cloudflare, err := service.stringValue(ctx, sitesettings.CloudflareUUIDKey)
	if err != nil {
		return err
	}
	bailian, err := service.stringValue(ctx, sitesettings.BailianUUIDKey)
	if err != nil {
		return err
	}
	updates := make(map[string]any)
	if !isUUIDv7(cloudflare) {
		value, err := newUUIDv7()
		if err != nil {
			return err
		}
		updates[sitesettings.CloudflareUUIDKey] = value
	}
	if !isUUIDv7(bailian) {
		value, err := newUUIDv7()
		if err != nil {
			return err
		}
		updates[sitesettings.BailianUUIDKey] = value
	}
	if len(updates) > 0 {
		_, _, err = service.settings.UpdateSystem(ctx, updates)
	}
	return err
}

func (service *Service) cloudflare(ctx context.Context, active string) (Provider, error) {
	uuidValue, err := service.stringValue(ctx, sitesettings.CloudflareUUIDKey)
	if err != nil {
		return Provider{}, err
	}
	accountID, _ := service.stringValue(ctx, sitesettings.CloudflareAccountIDKey)
	model, _ := service.stringValue(ctx, sitesettings.CloudflareDefaultModelKey)
	imageModel, _ := service.stringValue(ctx, sitesettings.CloudflareDefaultImageModelKey)
	verified, _ := service.boolValue(ctx, sitesettings.CloudflareVerifiedKey)
	verifiedFingerprint, _ := service.stringValue(ctx, sitesettings.CloudflareVerifiedFingerprintKey)
	verifiedAt, _ := service.timeValue(ctx, sitesettings.CloudflareVerifiedAtKey)
	secret, secretErr := service.settings.Secret(ctx, sitesettings.CloudflareAPITokenKey)
	hasSecret := secretErr == nil && strings.TrimSpace(secret) != ""
	configured := accountID != "" && model != "" && imageModel != "" && hasSecret
	currentFingerprint, _ := service.providerFingerprint(ctx, TypeCloudflareAIGateway)
	verified = verified && verifiedFingerprint != "" && verifiedFingerprint == currentFingerprint
	if !verified {
		verifiedAt = nil
	}
	isActive := active == TypeCloudflareAIGateway
	ready := configured && verified
	baseURL := cloudflareBaseURL(accountID)
	return Provider{
		UUID: uuidValue, ProviderType: TypeCloudflareAIGateway, DisplayName: "Cloudflare AI Gateway",
		BaseURL: baseURL, ImageBaseURL: baseURL, DefaultModel: model, DefaultImageModel: imageModel, AccountID: accountID,
		Configured: configured, Verified: verified, Active: isActive, Ready: ready,
		Enabled: isActive && ready, HasSecret: hasSecret, VerifiedAt: verifiedAt,
	}, nil
}

func (service *Service) bailian(ctx context.Context, active string) (Provider, error) {
	uuidValue, err := service.stringValue(ctx, sitesettings.BailianUUIDKey)
	if err != nil {
		return Provider{}, err
	}
	workspace, _ := service.stringValue(ctx, sitesettings.BailianWorkspaceKey)
	region, _ := service.stringValue(ctx, sitesettings.BailianRegionKey)
	verified, _ := service.boolValue(ctx, sitesettings.BailianVerifiedKey)
	verifiedFingerprint, _ := service.stringValue(ctx, sitesettings.BailianVerifiedFingerprintKey)
	verifiedAt, _ := service.timeValue(ctx, sitesettings.BailianVerifiedAtKey)
	secret, secretErr := service.settings.Secret(ctx, sitesettings.BailianAPIKeyKey)
	hasSecret := secretErr == nil && strings.TrimSpace(secret) != ""
	configured := workspace != "" && validBailianRegion(region) && hasSecret
	currentFingerprint, _ := service.providerFingerprint(ctx, TypeAliyunBailian)
	verified = verified && verifiedFingerprint != "" && verifiedFingerprint == currentFingerprint
	if !verified {
		verifiedAt = nil
	}
	isActive := active == TypeAliyunBailian
	ready := configured && verified
	baseURL, imageURL := "", ""
	if workspace != "" && validBailianRegion(region) {
		host := workspace + "." + region + ".maas.aliyuncs.com"
		baseURL = "https://" + host + "/compatible-mode/v1"
		imageURL = "https://" + host + "/api/v1/services/aigc/multimodal-generation/generation"
	}
	return Provider{
		UUID: uuidValue, ProviderType: TypeAliyunBailian, DisplayName: "阿里云百炼",
		BaseURL: baseURL, ImageBaseURL: imageURL, DefaultModel: BailianTextModel,
		DefaultImageModel: BailianImageModel, Region: region, WorkspaceID: workspace,
		Configured: configured, Verified: verified, Active: isActive, Ready: ready,
		Enabled: isActive && ready, HasSecret: hasSecret, VerifiedAt: verifiedAt,
	}, nil
}

func (service *Service) stringValue(ctx context.Context, key string) (string, error) {
	value, err := service.settings.Value(ctx, key)
	if err != nil || value == nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("site setting %s is not a string", key)
	}
	return strings.TrimSpace(text), nil
}

func (service *Service) boolValue(ctx context.Context, key string) (bool, error) {
	value, err := service.settings.Value(ctx, key)
	if err != nil {
		return false, err
	}
	boolean, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("site setting %s is not a boolean", key)
	}
	return boolean, nil
}

func (service *Service) timeValue(ctx context.Context, key string) (*time.Time, error) {
	value, err := service.settings.Value(ctx, key)
	if err != nil || value == nil {
		return nil, err
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return nil, nil
	}
	return &parsed, nil
}

func validBailianRegion(region string) bool {
	switch region {
	case "cn-beijing", "ap-southeast-1", "eu-central-1", "ap-northeast-1":
		return true
	default:
		return false
	}
}

func (service *Service) providerFingerprint(ctx context.Context, providerType string) (string, error) {
	if providerType == TypeAliyunBailian {
		return service.settings.Fingerprint(ctx, sitesettings.BailianWorkspaceKey, sitesettings.BailianRegionKey, sitesettings.BailianAPIKeyKey)
	}
	return service.settings.Fingerprint(ctx, sitesettings.CloudflareAccountIDKey, sitesettings.CloudflareDefaultModelKey, sitesettings.CloudflareDefaultImageModelKey, sitesettings.CloudflareAPITokenKey)
}

func normalizeProviderType(providerType string) string {
	if strings.TrimSpace(providerType) == LegacyTypeOpenAICompatible {
		return TypeCloudflareAIGateway
	}
	return strings.TrimSpace(providerType)
}

func cloudflareBaseURL(accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return ""
	}
	return "https://api.cloudflare.com/client/v4/accounts/" + accountID + "/ai/v1"
}

func newUUIDv7() (string, error) {
	value, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate UUIDv7: %w", err)
	}
	return value.String(), nil
}

func isUUIDv7(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.Version() == 7
}

func IsNotReady(err error) bool {
	var domainErr *Error
	return errors.As(err, &domainErr) && (domainErr.Code == CodeNoActiveProvider || domainErr.Code == CodeProviderNotReady)
}

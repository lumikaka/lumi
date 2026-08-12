package sitesettings

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"lumi/internal/appstore"

	"gorm.io/gorm"
)

type record struct {
	ID        int64 `gorm:"primaryKey;autoIncrement"`
	Key       string
	Value     string
	UpdatedAt time.Time
}

func (record) TableName() string { return "site_settings" }

type cachedValue struct {
	Raw       string
	UpdatedAt time.Time
}

type Item struct {
	Key          string     `json:"key"`
	Value        any        `json:"value"`
	DefaultValue any        `json:"default_value"`
	IsDefault    bool       `json:"is_default"`
	IsSecret     bool       `json:"is_secret"`
	HasValue     *bool      `json:"has_value,omitempty"`
	MaskedValue  *string    `json:"masked_value,omitempty"`
	SecretState  string     `json:"secret_state,omitempty"`
	UpdatedAt    *time.Time `json:"updated_at"`
}

type Response struct {
	Items    []Item         `json:"items"`
	Settings map[string]any `json:"settings"`
}

type Service struct {
	app    *appstore.Store
	codec  *secretCodec
	mu     sync.RWMutex
	loaded bool
	cache  map[string]cachedValue
	now    func() time.Time
}

func NewService(app *appstore.Store, keys MasterKeyStore) *Service {
	return &Service{app: app, codec: newSecretCodec(keys), cache: make(map[string]cachedValue), now: time.Now}
}

func (service *Service) Close() { service.codec.clear() }

func (service *Service) List(ctx context.Context) (Response, error) {
	rows, err := service.snapshot(ctx)
	if err != nil {
		return Response{}, err
	}
	return service.response(ctx, rows), nil
}

func (service *Service) Value(ctx context.Context, key string) (any, error) {
	definition, ok := definitions[key]
	if !ok {
		return nil, unknownSetting(key)
	}
	if definition.Secret {
		return nil, nil
	}
	rows, err := service.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return effectiveValue(definition, rows[key]), nil
}

func (service *Service) Secret(ctx context.Context, key string) (string, error) {
	definition, ok := definitions[key]
	if !ok || !definition.Secret {
		return "", unknownSetting(key)
	}
	rows, err := service.snapshot(ctx)
	if err != nil {
		return "", err
	}
	row, ok := rows[key]
	if !ok {
		return "", nil
	}
	return service.codec.decrypt(ctx, key, row.Raw)
}

// Fingerprint identifies the exact cached setting rows without decrypting
// secrets. It is used to bind a successful Provider connection check to the
// configuration that was actually checked.
func (service *Service) Fingerprint(ctx context.Context, keys ...string) (string, error) {
	rows, err := service.snapshot(ctx)
	if err != nil {
		return "", err
	}
	keys = append([]string(nil), keys...)
	sort.Strings(keys)
	hash := sha256.New()
	for _, key := range keys {
		definition, ok := definitions[key]
		if !ok {
			return "", unknownSetting(key)
		}
		raw := rows[key].Raw
		if raw == "" {
			encoded, _ := json.Marshal(definition.Default)
			raw = string(encoded)
		}
		_, _ = hash.Write([]byte(key))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(raw))
		_, _ = hash.Write([]byte{0})
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (service *Service) Update(ctx context.Context, settings map[string]any) (Response, []string, error) {
	return service.update(ctx, settings, false)
}

func (service *Service) UpdateSystem(ctx context.Context, settings map[string]any) (Response, []string, error) {
	return service.update(ctx, settings, true)
}

func (service *Service) update(ctx context.Context, settings map[string]any, system bool) (Response, []string, error) {
	if len(settings) == 0 {
		response, err := service.List(ctx)
		return response, nil, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := service.loadLocked(ctx); err != nil {
		return Response{}, nil, err
	}
	normalized := make(map[string]any, len(settings)+2)
	for key, value := range settings {
		definition, ok := definitions[key]
		if !ok {
			return Response{}, nil, unknownSetting(key)
		}
		if !system && (!definition.Public || !definition.Mutable) {
			return Response{}, nil, unknownSetting(key)
		}
		clean, err := definition.Normalize(value)
		if err != nil {
			return Response{}, nil, invalidSetting(key, err)
		}
		normalized[key] = clean
	}
	if !system {
		invalidateVerification(normalized, settings)
	}
	secretExists := hasSecretRows(service.cache)
	encoded := make(map[string]string, len(normalized))
	for key, value := range normalized {
		definition := definitions[key]
		if definition.Secret {
			raw, err := service.codec.encrypt(ctx, key, value.(string), !secretExists)
			if err != nil {
				return Response{}, nil, err
			}
			encoded[key] = raw
			secretExists = true
			continue
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return Response{}, nil, invalidSetting(key, err)
		}
		encoded[key] = string(raw)
	}
	now := service.now().UTC()
	if err := service.app.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for key, raw := range encoded {
			item := record{Key: key, Value: raw, UpdatedAt: now}
			if err := tx.Where("key = ?", key).Assign(map[string]any{"value": raw, "updated_at": now}).FirstOrCreate(&item).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return Response{}, nil, storageError("更新全局设置失败", err)
	}
	for key, raw := range encoded {
		service.cache[key] = cachedValue{Raw: raw, UpdatedAt: now}
	}
	changed := sortedKeys(encoded)
	return service.response(ctx, cloneCache(service.cache)), changed, nil
}

func (service *Service) Reset(ctx context.Context, keys []string) (Response, []string, error) {
	if len(keys) == 0 {
		return Response{}, nil, invalidSetting("", fmt.Errorf("keys cannot be empty"))
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := service.loadLocked(ctx); err != nil {
		return Response{}, nil, err
	}
	unique := make(map[string]struct{}, len(keys)+2)
	for _, key := range keys {
		definition, ok := definitions[key]
		if !ok || !definition.Public || !definition.Mutable {
			return Response{}, nil, unknownSetting(key)
		}
		unique[key] = struct{}{}
	}
	invalidateResetVerification(unique)
	clean := make([]string, 0, len(unique))
	for key := range unique {
		clean = append(clean, key)
	}
	sort.Strings(clean)
	if err := service.app.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Where("key IN ?", clean).Delete(&record{}).Error
	}); err != nil {
		return Response{}, nil, storageError("重置全局设置失败", err)
	}
	for _, key := range clean {
		delete(service.cache, key)
	}
	return service.response(ctx, cloneCache(service.cache)), clean, nil
}

func (service *Service) snapshot(ctx context.Context) (map[string]cachedValue, error) {
	service.mu.RLock()
	if service.loaded {
		copy := cloneCache(service.cache)
		service.mu.RUnlock()
		return copy, nil
	}
	service.mu.RUnlock()
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := service.loadLocked(ctx); err != nil {
		return nil, err
	}
	return cloneCache(service.cache), nil
}

func (service *Service) loadLocked(ctx context.Context) error {
	if service.loaded {
		return nil
	}
	var records []record
	if err := service.app.DB().WithContext(ctx).Order("key ASC").Find(&records).Error; err != nil {
		return storageError("读取全局设置失败", err)
	}
	loaded := make(map[string]cachedValue, len(records))
	for _, item := range records {
		if _, registered := definitions[item.Key]; registered {
			loaded[item.Key] = cachedValue{Raw: item.Value, UpdatedAt: item.UpdatedAt}
		}
	}
	service.cache, service.loaded = loaded, true
	return nil
}

func (service *Service) response(ctx context.Context, rows map[string]cachedValue) Response {
	response := Response{Items: make([]Item, 0, len(orderedDefinitions)), Settings: make(map[string]any)}
	for _, definition := range orderedDefinitions {
		if !definition.Public {
			continue
		}
		row, stored := rows[definition.Key]
		item := Item{Key: definition.Key, DefaultValue: definition.Default, IsDefault: !stored, IsSecret: definition.Secret}
		if stored {
			updatedAt := row.UpdatedAt
			item.UpdatedAt = &updatedAt
		}
		if definition.Secret {
			item.Value = nil
			hasValue, maskedValue := stored, ""
			item.HasValue, item.MaskedValue = &hasValue, &maskedValue
			item.SecretState = "empty"
			if stored {
				value, err := service.codec.decrypt(ctx, definition.Key, row.Raw)
				if err != nil {
					item.SecretState = "unavailable"
				} else {
					item.SecretState = "available"
					maskedValue = maskSecret(value)
				}
			}
			response.Settings[definition.Key] = nil
		} else {
			item.Value = effectiveValue(definition, row)
			response.Settings[definition.Key] = item.Value
		}
		response.Items = append(response.Items, item)
	}
	return response
}

func effectiveValue(definition definition, row cachedValue) any {
	if row.Raw == "" {
		return definition.Default
	}
	var value any
	if err := json.Unmarshal([]byte(row.Raw), &value); err != nil {
		return definition.Default
	}
	clean, err := definition.Normalize(value)
	if err != nil {
		return definition.Default
	}
	return clean
}

func invalidateVerification(normalized map[string]any, original map[string]any) {
	for key := range original {
		switch {
		case strings.HasPrefix(key, "ai_providers.openai_compatible."):
			normalized[CloudflareVerifiedKey], normalized[CloudflareVerifiedAtKey], normalized[CloudflareVerifiedFingerprintKey] = false, nil, nil
		case strings.HasPrefix(key, "ai_providers.aliyun_bailian."):
			normalized[BailianVerifiedKey], normalized[BailianVerifiedAtKey], normalized[BailianVerifiedFingerprintKey] = false, nil, nil
		}
	}
}

func invalidateResetVerification(keys map[string]struct{}) {
	openAI, bailian := false, false
	for key := range keys {
		openAI = openAI || strings.HasPrefix(key, "ai_providers.openai_compatible.")
		bailian = bailian || strings.HasPrefix(key, "ai_providers.aliyun_bailian.")
	}
	if openAI {
		keys[CloudflareVerifiedKey], keys[CloudflareVerifiedAtKey], keys[CloudflareVerifiedFingerprintKey] = struct{}{}, struct{}{}, struct{}{}
	}
	if bailian {
		keys[BailianVerifiedKey], keys[BailianVerifiedAtKey], keys[BailianVerifiedFingerprintKey] = struct{}{}, struct{}{}, struct{}{}
	}
}

func hasSecretRows(rows map[string]cachedValue) bool {
	for key := range rows {
		if definition, ok := definitions[key]; ok && definition.Secret {
			return true
		}
	}
	return false
}

func cloneCache(source map[string]cachedValue) map[string]cachedValue {
	copy := make(map[string]cachedValue, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func maskSecret(value string) string {
	runes := []rune(value)
	if len(runes) > 4 {
		runes = runes[len(runes)-4:]
	}
	return "****" + string(runes)
}

func unknownSetting(key string) error {
	return settingError(CodeUnknownSetting, "未知的全局设置", "未注册设置项："+key, nil)
}

func invalidSetting(key string, cause error) error {
	details := "设置值不符合注册类型或约束。"
	if key != "" {
		details = "设置项 " + key + " 的值无效。"
	}
	return settingError(CodeInvalidSetting, "全局设置无效", details, cause)
}

func storageError(message string, cause error) error {
	var domainErr *Error
	if errors.As(cause, &domainErr) {
		return cause
	}
	return settingError(CodeStorageFailed, message, "无法访问全局 lumi.sqlite。", cause)
}

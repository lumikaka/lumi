package provider

import "lumi/internal/sitesettings"

// NewMemorySecretStore is retained for internal test fixtures. Provider secrets
// are now encrypted site settings and this value is the in-memory master-key
// store used by those fixtures.
func NewMemorySecretStore() *sitesettings.MemoryMasterKeyStore {
	return sitesettings.NewMemoryMasterKeyStore()
}

func NewOSSecretStore() sitesettings.MasterKeyStore {
	return sitesettings.NewOSMasterKeyStore()
}

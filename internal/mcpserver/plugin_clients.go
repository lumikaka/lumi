package mcpserver

import (
	"encoding/json"

	"gorm.io/gorm/clause"
)

// Legacy public UUIDv7 client identifiers, retained for existing connections.
// Current Codex plugins use dynamic registration with their exact callback URI.
const (
	ProductionPluginClientUUID  = "01a0857c-ad68-78a8-8a91-949bf8b33d21"
	DevelopmentPluginClientUUID = "01a0857c-ad68-793a-af24-001222f07f37"
)

func (s *Service) ensurePluginClients() error {
	// Codex inserts its active loopback listener port into this callback. Only
	// the port may change; host, path and query remain exactly registered.
	redirects, _ := json.Marshal([]string{"http://127.0.0.1/callback"})
	for _, spec := range []struct{ uuid, name string }{
		{ProductionPluginClientUUID, "Lumi desktop plugin"},
		{DevelopmentPluginClientUUID, "Lumi development plugin"},
	} {
		client := oauthClient{UUID: spec.uuid, Name: spec.name, RedirectURIs: string(redirects), CreatedAt: s.now().UTC()}
		if err := s.app.DB().Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "uuid"}}, DoUpdates: clause.AssignmentColumns([]string{"name", "redirect_uris"})}).Create(&client).Error; err != nil {
			return err
		}
	}
	return nil
}

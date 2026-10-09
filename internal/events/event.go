// Package events defines the events that trigger flows.
package events

import "time"

// Event types. They match flow trigger.on values.
const (
	AssetUploaded      = "asset.uploaded"
	AssetTrashed       = "asset.trashed"
	AssetDeleted       = "asset.deleted"
	AssetRestored      = "asset.restored"
	DuplicatesResolved = "duplicates.resolved"
	TagAssigned        = "tag.assigned"
	TagRemoved         = "tag.removed"
	Manual             = "manual"
)

// Event is something that happened in Immich.
type Event struct {
	Type     string         `json:"type"`
	Time     time.Time      `json:"time"`
	UserID   string         `json:"userId"`             // user who caused the event
	AssetIDs []string       `json:"assetIds,omitempty"` // flows run once per asset; once without an asset if empty
	Data     map[string]any `json:"data,omitempty"`     // type-specific payload, visible as trigger.data
}

// Package connect stores per-user Immich API keys so flows can act as the asset owner.
package connect

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/serraniel/immich-flows/internal/immich"
)

// ErrNotConnected means the user has not connected immich-flows to their account.
var ErrNotConnected = errors.New("user has not connected immich-flows")

// Permissions granted to the API key created on connect.
var Permissions = []immich.Permission{
	immich.AssetRead, immich.AssetUpdate, immich.AssetDownload,
	immich.TagRead, immich.TagCreate, immich.TagUpdate, immich.TagDelete, immich.TagAsset,
	immich.DuplicateRead, immich.UserRead,
}

const keyName = "immich-flows"

// Manager creates, stores and uses per-user API keys.
type Manager struct {
	db     *sql.DB
	key    []byte
	immich *url.URL
	hc     *http.Client
	now    func() time.Time
}

// New returns a Manager. key is from LoadKey.
func New(db *sql.DB, key []byte, immichURL *url.URL, hc *http.Client) *Manager {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Manager{db: db, key: key, immich: immichURL, hc: hc, now: time.Now}
}

// Status describes a user's connection.
type Status struct {
	Connected bool       `json:"connected"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
}

// Status returns whether userID is connected.
func (m *Manager) Status(ctx context.Context, userID string) (Status, error) {
	var created int64
	err := m.db.QueryRowContext(ctx, `SELECT created_at FROM connections WHERE user_id = ?`, userID).Scan(&created)
	if errors.Is(err, sql.ErrNoRows) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	t := time.UnixMilli(created).UTC()
	return Status{Connected: true, CreatedAt: &t}, nil
}

// Connect creates an API key for userID using the caller's credentials and stores it.
// An existing connection is replaced (its key is deleted in Immich).
func (m *Manager) Connect(ctx context.Context, userID string, caller immich.Auth) error {
	c, err := immich.New(m.immich, caller, m.hc)
	if err != nil {
		return err
	}
	name := keyName
	res, err := c.CreateApiKeyWithResponse(ctx, immich.ApiKeyCreateDto{Name: &name, Permissions: Permissions})
	if err != nil {
		return err
	}
	if err := immich.Check(res, res.Body); err != nil {
		return err
	}
	secret, err := seal(m.key, []byte(res.JSON201.Secret))
	if err != nil {
		return err
	}
	newID := res.JSON201.ApiKey.Id.String()

	oldID, _ := m.keyID(ctx, userID)
	if _, err := m.db.ExecContext(ctx, `
		INSERT INTO connections (user_id, api_key_id, api_key, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET api_key_id = excluded.api_key_id, api_key = excluded.api_key, created_at = excluded.created_at`,
		userID, newID, secret, m.now().UnixMilli()); err != nil {
		return err
	}
	if oldID != "" {
		_ = m.deleteKey(ctx, c, oldID)
	}
	return nil
}

// Disconnect deletes the user's API key in Immich (using the caller's credentials) and locally.
func (m *Manager) Disconnect(ctx context.Context, userID string, caller immich.Auth) error {
	id, err := m.keyID(ctx, userID)
	if err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	c, err := immich.New(m.immich, caller, m.hc)
	if err != nil {
		return err
	}
	if err := m.deleteKey(ctx, c, id); err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, `DELETE FROM connections WHERE user_id = ?`, userID)
	return err
}

// Users returns the IDs of all connected users.
func (m *Manager) Users(ctx context.Context) ([]string, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT user_id FROM connections ORDER BY user_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Client returns an Immich client acting as userID.
func (m *Manager) Client(ctx context.Context, userID string) (*immich.ClientWithResponses, error) {
	var enc []byte
	err := m.db.QueryRowContext(ctx, `SELECT api_key FROM connections WHERE user_id = ?`, userID).Scan(&enc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotConnected
	}
	if err != nil {
		return nil, err
	}
	secret, err := open(m.key, enc)
	if err != nil {
		return nil, fmt.Errorf("decrypt api key (IMMICH_FLOWS_SECRET changed?): %w", err)
	}
	return immich.New(m.immich, immich.APIKey(string(secret)), m.hc)
}

func (m *Manager) keyID(ctx context.Context, userID string) (string, error) {
	var id string
	err := m.db.QueryRowContext(ctx, `SELECT api_key_id FROM connections WHERE user_id = ?`, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (m *Manager) deleteKey(ctx context.Context, c *immich.ClientWithResponses, id string) error {
	var uid openapi_types.UUID
	if err := uid.UnmarshalText([]byte(id)); err != nil {
		return err
	}
	res, err := c.DeleteApiKeyWithResponse(ctx, uid)
	if err != nil {
		return err
	}
	if res.StatusCode() == http.StatusNotFound || res.StatusCode() == http.StatusBadRequest {
		return nil // already gone
	}
	return immich.Check(res, res.Body)
}

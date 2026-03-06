package extensions

import (
	"context"
	"encoding/json"
)

// kvStoreNamespace is the extension_data namespace used for WASM KV storage.
const kvStoreNamespace = "wasm_kv"

// extensionDataKVStore implements KVStore using the existing extension_data table.
// Each key-value pair is stored with namespace "wasm_kv", scoped per campaign
// and extension. This reuses the existing infrastructure without new tables.
type extensionDataKVStore struct {
	repo ExtensionRepository
}

// NewKVStore creates a KV store backed by the extension_data repository.
func NewKVStore(repo ExtensionRepository) KVStore {
	return &extensionDataKVStore{repo: repo}
}

// Get retrieves a value by key for a campaign+extension.
func (s *extensionDataKVStore) Get(ctx context.Context, campaignID, extensionID, key string) (json.RawMessage, error) {
	data, err := s.repo.GetData(ctx, campaignID, extensionID, kvStoreNamespace, key)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return json.RawMessage("null"), nil
	}
	return data.DataValue, nil
}

// Set stores a key-value pair for a campaign+extension.
func (s *extensionDataKVStore) Set(ctx context.Context, campaignID, extensionID, key string, value json.RawMessage) error {
	return s.repo.SetData(ctx, &ExtensionData{
		CampaignID:  campaignID,
		ExtensionID: extensionID,
		Namespace:   kvStoreNamespace,
		DataKey:     key,
		DataValue:   value,
	})
}

// Delete removes a key-value pair for a campaign+extension.
func (s *extensionDataKVStore) Delete(ctx context.Context, campaignID, extensionID, key string) error {
	return s.repo.DeleteDataByKey(ctx, campaignID, extensionID, kvStoreNamespace, key)
}

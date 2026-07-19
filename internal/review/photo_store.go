package review

const photoStoreShardCount = 256

// photoStore is an immutable, sharded photo map. Updating a photo copies only
// its shard and the fixed-size shard index, rather than the whole library.
type photoStore struct {
	shards [photoStoreShardCount]map[string]Photo
	length int
}

func newPhotoStore(photos map[string]Photo) *photoStore {
	store := &photoStore{length: len(photos)}
	for id, photo := range photos {
		shard := photoStoreShard(id)
		if store.shards[shard] == nil {
			store.shards[shard] = make(map[string]Photo)
		}
		store.shards[shard][id] = photo
	}
	return store
}

func photoStoreShard(id string) uint8 {
	var hash uint32 = 2166136261
	for i := 0; i < len(id); i++ {
		hash ^= uint32(id[i])
		hash *= 16777619
	}
	return uint8(hash)
}

func (s *photoStore) Get(id string) (Photo, bool) {
	if s == nil {
		return Photo{}, false
	}
	photo, ok := s.shards[photoStoreShard(id)][id]
	return photo, ok
}

func (s *photoStore) WithChanges(changes map[string]Photo) *photoStore {
	if len(changes) == 0 {
		return s
	}
	next := *s
	copied := [photoStoreShardCount]bool{}
	for id, photo := range changes {
		shard := photoStoreShard(id)
		if !copied[shard] {
			old := next.shards[shard]
			clone := make(map[string]Photo, len(old)+1)
			for existingID, existingPhoto := range old {
				clone[existingID] = existingPhoto
			}
			next.shards[shard] = clone
			copied[shard] = true
		}
		if _, exists := next.shards[shard][id]; !exists {
			next.length++
		}
		next.shards[shard][id] = photo
	}
	return &next
}

func (s *photoStore) Len() int {
	if s == nil {
		return 0
	}
	return s.length
}

func (s *photoStore) Range(fn func(string, Photo) bool) {
	if s == nil {
		return
	}
	for _, shard := range s.shards {
		for id, photo := range shard {
			if !fn(id, photo) {
				return
			}
		}
	}
}

func (s *photoStore) Materialize() map[string]Photo {
	out := make(map[string]Photo, s.Len())
	s.Range(func(id string, photo Photo) bool {
		out[id] = photo
		return true
	})
	return out
}

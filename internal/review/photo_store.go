package review

const photoStoreMaxDepth = 32

type photoStore struct {
	base    map[string]Photo
	parent  *photoStore
	changes map[string]Photo
	length  int
	depth   int
}

func newPhotoStore(photos map[string]Photo) *photoStore {
	base := make(map[string]Photo, len(photos))
	for id, photo := range photos {
		base[id] = photo
	}
	return &photoStore{base: base, length: len(base)}
}

func (s *photoStore) Get(id string) (Photo, bool) {
	for layer := s; layer != nil; layer = layer.parent {
		if photo, ok := layer.changes[id]; ok {
			return photo, true
		}
		if layer.base != nil {
			photo, ok := layer.base[id]
			return photo, ok
		}
	}
	return Photo{}, false
}

func (s *photoStore) WithChanges(changes map[string]Photo) *photoStore {
	if len(changes) == 0 {
		return s
	}
	copyChanges := make(map[string]Photo, len(changes))
	for id, photo := range changes {
		copyChanges[id] = photo
	}
	next := &photoStore{parent: s, changes: copyChanges, length: s.Len(), depth: s.depth + 1}
	if next.depth >= photoStoreMaxDepth {
		return newPhotoStore(next.Materialize())
	}
	return next
}

func (s *photoStore) Len() int {
	if s == nil {
		return 0
	}
	return s.length
}

func (s *photoStore) Range(fn func(string, Photo) bool) {
	seen := make(map[string]struct{}, s.Len())
	for layer := s; layer != nil; layer = layer.parent {
		for id, photo := range layer.changes {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			if !fn(id, photo) {
				return
			}
		}
		if layer.base != nil {
			for id, photo := range layer.base {
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				if !fn(id, photo) {
					return
				}
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

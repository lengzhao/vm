package state

import (
	"hash/maphash"

	"github.com/lengzhao/vm/core"
)

const stateShards = 256

// stateTable is immutable once published. A generation shares every untouched
// shard with its predecessor; readers retain their complete old generation.
// Randomized hashing keeps contract-controlled keys from targeting known shards.
type stateTable[V any] struct {
	seed   maphash.Seed
	shards [stateShards]map[core.StateKey]V
}

func newStateTable[V any]() *stateTable[V] {
	return &stateTable[V]{seed: maphash.MakeSeed()}
}

func (s *stateTable[V]) shard(k core.StateKey) uint64 {
	return maphash.Comparable(s.seed, k) % stateShards
}

func (s *stateTable[V]) get(k core.StateKey) (V, bool) {
	v, ok := s.shards[s.shard(k)][k]
	return v, ok
}

// set is only used while constructing a private, unpublished generation.
func (s *stateTable[V]) set(k core.StateKey, v V) {
	i := s.shard(k)
	if s.shards[i] == nil {
		s.shards[i] = make(map[core.StateKey]V)
	}
	s.shards[i][k] = v
}

func (s *stateTable[V]) apply(check func() error, writes map[core.StateKey]core.StateWrite, project func(core.StateWrite) (V, bool)) (*stateTable[V], error) {
	next := *s
	var copied [stateShards]bool
	for k, w := range writes {
		if err := check(); err != nil {
			return nil, err
		}
		i := s.shard(k)
		if !copied[i] {
			shard := make(map[core.StateKey]V, len(s.shards[i])+1)
			for key, v := range s.shards[i] {
				if err := check(); err != nil {
					return nil, err
				}
				shard[key] = v
			}
			next.shards[i] = shard
			copied[i] = true
		}
		v, keep := project(w)
		if keep {
			next.shards[i][k] = v
		} else {
			delete(next.shards[i], k)
		}
	}
	return &next, nil
}

func stateValue(w core.StateWrite) ([]byte, bool)    { return w.Value, !w.Deleted }
func stateVersion(core.StateWrite) (*revision, bool) { return &revision{}, true }

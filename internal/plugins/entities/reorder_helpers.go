package entities

// reindexForReorder computes the densely-ordered sibling slice after moving
// `moved` to the desired 0-based `targetIndex`. It drops `moved` from the
// current order, clamps the index into [0, len], and reinserts it there.
//
// The result is exactly the order a dense re-sequence (ResequenceSiblings /
// ResequenceNodes / ResequenceChildTypes) should persist as sort_order 0..N-1,
// so the (sort_order, name) tiebreak in the render can never snap a dragged
// item back to its old position — the silent-revert bug class #477 fixed for
// entity rows. This helper is the shared, tested primitive so the folder-node
// and sub-category-type reorder paths behave identically to entity reorder
// (whose reinsert is inlined in ReorderEntity for historical reasons).
//
// If `moved` is absent from `ordered` (e.g. a freshly created row not yet in
// the listed sibling set) it is still inserted at the clamped index.
func reindexForReorder[T comparable](ordered []T, moved T, targetIndex int) []T {
	out := make([]T, 0, len(ordered)+1)
	for _, v := range ordered {
		if v != moved {
			out = append(out, v)
		}
	}

	idx := targetIndex
	if idx < 0 {
		idx = 0
	}
	if idx > len(out) {
		idx = len(out)
	}

	out = append(out, moved) // grow by one, then shift the tail right by one
	copy(out[idx+1:], out[idx:])
	out[idx] = moved
	return out
}

// sameParent reports whether two nullable parent pointers denote the same
// parent — both root (nil) or both the same id. Used to scope a node's sibling
// set to its own parent before a dense re-sequence.
func sameParent(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

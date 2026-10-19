package entities

// showsTokenMaker reports whether a page's picture offers "Make token": the
// page has a picture to start from, and its type (or the page itself)
// declares the token field, so the token lands where the hover card and the
// Foundry module look for it rather than in a field no one sees. Whether the
// viewer may edit is the caller's check.
func showsTokenMaker(entityType *EntityType, entity *Entity) bool {
	if entity == nil || entity.ImagePath == nil || *entity.ImagePath == "" {
		return false
	}
	var typeFields []FieldDefinition
	if entityType != nil {
		typeFields = entityType.Fields
	}
	for _, fd := range MergeFields(typeFields, entity.FieldOverrides) {
		if fd.Key == TokenFieldKey {
			return true
		}
	}
	return false
}

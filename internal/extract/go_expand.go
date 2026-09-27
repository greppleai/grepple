package extract

func expandGoEmbeddedMembers(result *Analysis) {
	for _, key := range sortedKeys(result.PackageDeclarations) {
		expandGoDeclaration(key, result, map[string]bool{})
	}
}

func expandGoDeclaration(key string, result *Analysis, visiting map[string]bool) []Member {
	declaration := result.PackageDeclarations[key]
	if declaration == nil || visiting[key] {
		return nil
	}
	visiting[key] = true
	members := append([]Member(nil), declaration.Members...)
	for _, parent := range sortedKeys(declaration.Extends) {
		parentKey := declaration.PackageID + ":" + parent
		for _, promoted := range expandGoDeclaration(parentKey, result, visiting) {
			if !hasMemberName(members, promoted.Name) {
				members = append(members, promoted)
			}
		}
	}
	delete(visiting, key)
	declaration.Members = members
	return members
}

func hasMemberName(members []Member, name string) bool {
	for _, member := range members {
		if member.Name == name {
			return true
		}
	}
	return false
}

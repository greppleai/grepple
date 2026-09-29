package navigation

// swiftNavigationIndex limits resolution to the source file. Swift imports
// identify modules, not files: resolving them needs SwiftPM/Xcode target and
// module ownership evidence, so matching filenames are not sufficient.
type swiftNavigationIndex struct{ baseLanguageNavigationIndex }

func (*swiftNavigationIndex) filterCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	return filterNavigationCandidates(candidates, func(candidate navigationDeclaration) bool {
		return candidate.file == call.file
	})
}

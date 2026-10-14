package navigation

import "fmt"

func validatePagedRanges(section pagedSection, position, size uint64) (uint64, error) {
	for _, block := range section.Blocks {
		if block.Offset != position || block.Length == 0 || block.Length > maxPagedBlockBytes || position+uint64(block.Length) > size {
			return 0, fmt.Errorf("invalid paged section range")
		}
		position += uint64(block.Length)
	}
	return position, nil
}

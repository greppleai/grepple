package extract

func (parser *classParser) parseGoMetadata(value string, line int) (bool, error) {
	if match := structTagMetadataRE.FindStringSubmatch(value); match != nil {
		if !parser.sawHeader {
			return true, nil
		}
		return true, parser.applyStructTagMetadata(match[1], match[2], match[3], line)
	}
	if match := underlyingMetadataRE.FindStringSubmatch(value); match != nil {
		if !parser.sawHeader {
			return true, nil
		}
		return true, parser.applyUnderlyingMetadata(match[1], match[2], line)
	}
	return false, nil
}

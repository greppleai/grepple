package agentskills

import "os"

func readLocalFile(path string) ([]byte, error) {
	reader, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return readBounded(reader)
}

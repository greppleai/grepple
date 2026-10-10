package analysis

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

func storedContextDigest(path string) ([32]byte, bool, error) {
	var sum [32]byte
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return sum, true, nil
	}
	if err != nil {
		return sum, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return sum, false, err
	}
	if !info.Mode().IsRegular() {
		return sum, false, fmt.Errorf("non-regular navigation context")
	}
	const maximum = 8 << 20
	if info.Size() > maximum {
		return sum, false, fmt.Errorf("navigation context exceeds bound")
	}
	digest := sha256.New()
	count, err := io.Copy(digest, io.LimitReader(file, maximum+1))
	if err != nil {
		return sum, false, err
	}
	if count > maximum {
		return sum, false, fmt.Errorf("navigation context exceeds bound")
	}
	copy(sum[:], digest.Sum(nil))
	return sum, false, nil
}

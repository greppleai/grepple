// Package artifacts implements persisted output-artifact maintenance.
package artifacts

import (
	"io"
	"os"
)

// Dependencies supplies process-owned command services.
type Dependencies struct {
	Stdout            io.Writer
	ArtifactDirectory func() (string, error)
	WorkingDirectory  func() string
}

func (dependencies Dependencies) stdout() io.Writer {
	if dependencies.Stdout != nil {
		return dependencies.Stdout
	}
	return os.Stdout
}
func (dependencies Dependencies) artifactDirectory() (string, error) {
	return dependencies.ArtifactDirectory()
}
func (dependencies Dependencies) workingDirectory() string {
	if dependencies.WorkingDirectory != nil {
		return dependencies.WorkingDirectory()
	}
	value, _ := os.Getwd()
	return value
}

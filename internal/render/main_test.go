package render

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Unsetenv("PI_SESSION_ID")
	os.Exit(m.Run())
}

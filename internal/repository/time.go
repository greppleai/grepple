package repository

import "time"

func isoNow() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

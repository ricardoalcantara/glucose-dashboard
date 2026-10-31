package model

import "time"

// Reading is one glucose sample stored at minute resolution.
// TS and ReceivedAt are UTC.
type Reading struct {
	TS         time.Time
	Mgdl       int
	Source     string
	ReceivedAt time.Time
}

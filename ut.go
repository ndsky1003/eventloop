package taskloop

import "time"

var zeroTime = time.Unix(0, 0)

func IsZero(t time.Time) bool {
	return t.IsZero() || zeroTime.Equal(t)
}

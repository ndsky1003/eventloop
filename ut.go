package task

import "time"

var zeroTime = time.Unix(0, 0)

func is_zero_time(t time.Time) bool {
	return t.IsZero() || zeroTime.Equal(t)
}

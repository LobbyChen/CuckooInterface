package dameon

import (
	"time"
)

type SystemRecorder struct {
	startTime  time.Time
	version    string
	launchType string
}

var record SystemRecorder

func init() {
	record = SystemRecorder{}
	record.startTime = time.Now()
	switch isDaemon() {
	case true:
		record.launchType = "Daemon"
	case false:
		record.launchType = "normal"
	}
}

func GetStartedTime() time.Duration {
	return time.Since(record.startTime)
}
func SetVersion(v string) {
	record.version = v
}
func GetVersion() string {
	return record.version
}

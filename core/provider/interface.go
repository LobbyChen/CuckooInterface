package provider

import "CuckooInterface/core/logger"

type InternalEvent interface {
	InjectAPI(Emit func(eventName, payload string), logger *logger.Logger)
	StartService()
	GetMeta() (id string, events []string)
	StopService()
}

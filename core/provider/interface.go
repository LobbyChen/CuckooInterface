package provider

type InternalEvent interface {
	InjectAPI(Emit func())
}

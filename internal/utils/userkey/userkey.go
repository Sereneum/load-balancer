package userkey

// Param параметр, по которому будут идентифицироваться пользователи
type Param interface {
	Value() string
	Type() string
}

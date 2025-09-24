package amqp091

import (
	"reflect"
	"strings"
)

func TypeName(m any) string {
	return strings.TrimLeft(reflect.TypeOf(m).String(), "*amqp091.")
}

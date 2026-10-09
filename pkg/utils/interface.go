package utils

import (
	"reflect"
	"strings"
)

var InterfaceUtil = newInterfaceUtil()

type interfaceUtil struct{}

func newInterfaceUtil() *interfaceUtil {
	return &interfaceUtil{}
}

func (util *interfaceUtil) Name(v any) string {
	return reflect.TypeOf(v).Name()
}

func (util *interfaceUtil) SnakeName(v any) string {
	name := reflect.TypeOf(v).String()
	a := strings.Split(name, ".")
	name = a[len(a)-1]

	return StringUtil.SnakeCase(StringUtil.LcFirst(name))
}

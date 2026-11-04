package app

import "reflect"

// nn returns v with every nil slice (at any depth, through pointers) replaced by an empty slice, so that DTOs
// always cross the bridge as `[]`, never `null`.
func nn[T any](v T) T {
	walkNonNil(reflect.ValueOf(&v).Elem())
	return v
}

func walkNonNil(v reflect.Value) {
	switch v.Kind() {
	case reflect.Slice:
		if v.IsNil() {
			if v.CanSet() {
				v.Set(reflect.MakeSlice(v.Type(), 0, 0))
			}
			return
		}
		for i := 0; i < v.Len(); i++ {
			walkNonNil(v.Index(i))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if f := v.Field(i); f.CanSet() {
				walkNonNil(f)
			}
		}
	case reflect.Ptr:
		if !v.IsNil() {
			walkNonNil(v.Elem())
		}
	}
}

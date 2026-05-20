package ara

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Row holds a single query result row returned by [Node.QueryRow].
type Row struct {
	m   map[string]any
	err error
}

// Err returns any error that occurred fetching the row, including the case
// where no rows were returned (returns [ErrNoRows]).
func (r *Row) Err() error { return r.err }

// Map returns the raw column map. Returns nil and an error if no row was found.
func (r *Row) Map() (map[string]any, error) { return r.m, r.err }

// Get reads a single named column into dest. dest must be a non-nil pointer.
// Supported dest types: *string, *int, *int64, *float64, *bool, *[]byte, *any.
func (r *Row) Get(col string, dest any) error {
	if r.err != nil {
		return r.err
	}
	val, ok := r.m[col]
	if !ok {
		return fmt.Errorf("ara: column %q not found in row", col)
	}
	return coerce(val, dest)
}

// ErrNoRows is returned by [Row.Err] when a query matched no rows.
var ErrNoRows = fmt.Errorf("ara: no rows in result")

// coerce converts a JSON-decoded value (string, float64, bool, nil, []byte)
// into the pointed-to Go type.
func coerce(src any, dest any) error {
	if dest == nil {
		return fmt.Errorf("ara: Scan destination is nil")
	}
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("ara: Scan destination must be a non-nil pointer")
	}
	elem := rv.Elem()

	if src == nil {
		elem.Set(reflect.Zero(elem.Type()))
		return nil
	}

	switch d := dest.(type) {
	case *string:
		switch v := src.(type) {
		case string:
			*d = v
		case []byte:
			*d = string(v)
		default:
			b, err := json.Marshal(src)
			if err != nil {
				return fmt.Errorf("ara: cannot convert %T to string", src)
			}
			*d = string(b)
		}
	case *int:
		f, err := toFloat(src)
		if err != nil {
			return err
		}
		*d = int(f)
	case *int64:
		f, err := toFloat(src)
		if err != nil {
			return err
		}
		*d = int64(f)
	case *float64:
		f, err := toFloat(src)
		if err != nil {
			return err
		}
		*d = f
	case *bool:
		switch v := src.(type) {
		case bool:
			*d = v
		case float64:
			*d = v != 0
		case string:
			*d = v != "" && v != "0" && v != "false"
		default:
			return fmt.Errorf("ara: cannot convert %T to bool", src)
		}
	case *[]byte:
		switch v := src.(type) {
		case []byte:
			*d = v
		case string:
			*d = []byte(v)
		default:
			b, err := json.Marshal(src)
			if err != nil {
				return fmt.Errorf("ara: cannot convert %T to []byte", src)
			}
			*d = b
		}
	case *any:
		*d = src
	default:
		return fmt.Errorf("ara: unsupported Scan destination type %T", dest)
	}
	return nil
}

func toFloat(src any) (float64, error) {
	switch v := src.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		var f float64
		if _, err := fmt.Sscanf(v, "%g", &f); err != nil {
			return 0, fmt.Errorf("ara: cannot convert %q to number", v)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("ara: cannot convert %T to number", src)
	}
}

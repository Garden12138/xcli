package agentapi

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// StrictDecode rejects duplicate keys at every depth and unpaired escaped UTF-16
// surrogates, which encoding/json otherwise silently replaces with U+FFFD.
func StrictDecode(data []byte, dst any) error {
	if !utf8.Valid(data) || !validEscapes(data) {
		return Failure(400, "invalid_request")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := walk(d, 0); err != nil {
		return Failure(400, "invalid_request")
	}
	if _, err := d.Token(); err != io.EOF {
		return Failure(400, "invalid_request")
	}
	if len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		return Failure(400, "invalid_request")
	}
	var shape any
	if json.Unmarshal(data, &shape) != nil || !requiredShape(shape, reflect.TypeOf(dst).Elem()) {
		return Failure(400, "invalid_request")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field") {
			return Failure(422, "unsupported_parameter")
		}
		return Failure(400, "invalid_request")
	}
	return nil
}
func walk(d *json.Decoder, depth int) error {
	if depth > 32 {
		return io.ErrUnexpectedEOF
	}
	tok, err := d.Token()
	if err != nil {
		return err
	}
	if tok == nil {
		return nil
	}
	if delim, ok := tok.(json.Delim); ok {
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return err
				}
				s, ok := k.(string)
				if !ok || seen[s] {
					return io.ErrUnexpectedEOF
				}
				seen[s] = true
				if err = walk(d, depth+1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err = walk(d, depth+1); err != nil {
					return err
				}
			}
		default:
			return io.ErrUnexpectedEOF
		}
		_, err = d.Token()
		return err
	}
	return nil
}
func validEscapes(b []byte) bool {
	for i := 0; i < len(b); i++ {
		if b[i] != '\\' {
			continue
		}
		i++
		if i >= len(b) {
			return false
		}
		if b[i] != 'u' {
			continue
		}
		if i+4 >= len(b) {
			return false
		}
		v, err := strconv.ParseUint(string(b[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if v >= 0xDC00 && v <= 0xDFFF {
			return false
		}
		if v >= 0xD800 && v <= 0xDBFF {
			if i+6 >= len(b) || b[i+1] != '\\' || b[i+2] != 'u' {
				return false
			}
			w, e := strconv.ParseUint(string(b[i+3:i+7]), 16, 16)
			if e != nil || w < 0xDC00 || w > 0xDFFF {
				return false
			}
			i += 6
		}
	}
	return true
}

func requiredShape(v any, t reflect.Type) bool {
	if t == reflect.TypeFor[json.RawMessage]() {
		return true
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if v == nil {
		return false
	}
	switch t.Kind() {
	case reflect.Struct:
		m, ok := v.(map[string]any)
		if !ok {
			return false
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, opts, _ := strings.Cut(tag, ",")
			if name == "" {
				name = f.Name
			}
			value, exists := m[name]
			if !exists {
				if !strings.Contains(opts, "omitempty") {
					return false
				}
				continue
			}
			if !requiredShape(value, f.Type) {
				return false
			}
		}
	case reflect.Slice:
		a, ok := v.([]any)
		if !ok {
			return false
		}
		for _, x := range a {
			if !requiredShape(x, t.Elem()) {
				return false
			}
		}
	}
	return true
}

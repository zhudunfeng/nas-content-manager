package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// decodeStrictJSON 拒绝重复键、未知字段、额外对象与过深结构，避免配置身份发生歧义。
func decodeStrictJSON(data []byte, target any) error {
	tokens := json.NewDecoder(bytes.NewReader(data))
	tokens.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 16 {
			return errors.New("配置嵌套层数超出限制")
		}
		token, err := tokens.Token()
		if err != nil {
			return err
		}
		opening, compound := token.(json.Delim)
		if !compound {
			return nil
		}
		switch opening {
		case '{':
			seen := make(map[string]bool)
			for tokens.More() {
				key, readErr := tokens.Token()
				if readErr != nil {
					return readErr
				}
				name, ok := key.(string)
				name = strings.ToLower(name)
				if !ok || seen[name] {
					return errors.New("配置包含重复或无效字段")
				}
				seen[name] = true
				if err = value(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for tokens.More() {
				if err = value(depth + 1); err != nil {
					return err
				}
			}
		default:
			return errors.New("配置结构无效")
		}
		closing, err := tokens.Token()
		if err != nil || (opening == '{' && closing != json.Delim('}')) || (opening == '[' && closing != json.Delim(']')) {
			return errors.New("配置结构未正确结束")
		}
		return nil
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := tokens.Token(); err != io.EOF {
		return errors.New("配置只能包含一个 JSON 对象")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("配置结尾无效")
	}
	return nil
}

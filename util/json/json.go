package json

import (
	"encoding/json"
)

// Marshal 使用标准库序列化对象。旧版 sonic 与 Go 1.26 不兼容；CLI 和 API
// 共享这里的兼容层，因此统一回退到标准库以确保所有插件可以编译。
func Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal 使用sonic反序列化JSON到对象
func Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// MarshalString 序列化对象到JSON字符串
func MarshalString(v interface{}) (string, error) {
	bytes, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// UnmarshalString 反序列化JSON字符串到对象
func UnmarshalString(str string, v interface{}) error {
	return json.Unmarshal([]byte(str), v)
}

// MarshalIndent 序列化对象到格式化的JSON
func MarshalIndent(v interface{}, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

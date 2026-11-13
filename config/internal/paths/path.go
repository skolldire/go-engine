package paths

import "strings"

func Valid(path string) bool {
	if path == "" {
		return false
	}
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return false
		}
		for _, r := range part {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
				return false
			}
		}
	}
	return true
}
func Join(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

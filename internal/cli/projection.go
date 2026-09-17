package cli

import "strings"

func validateSelection(selection string) error {
	if len(selection) > 8192 || strings.Count(selection, ",") >= 128 {
		return invalid("select supports at most 128 paths and 8192 bytes.")
	}
	for _, path := range strings.Split(selection, ",") {
		for _, part := range strings.Split(path, ".") {
			if part == "" {
				return invalid("select requires nonempty comma-separated paths.")
			}
		}
	}
	return nil
}
func selectData(data any, paths []string) any {
	compiled := make([][]string, len(paths))
	for i, path := range paths {
		compiled[i] = strings.Split(path, ".")
	}
	return projectData(data, paths, compiled)
}

func projectData(data any, paths []string, compiled [][]string) any {
	if list, ok := data.([]any); ok {
		out := make([]any, len(list))
		for i, item := range list {
			out[i] = projectData(item, paths, compiled)
		}
		return out
	}
	result := make(map[string]any, len(paths))
	for i, path := range paths {
		var value any = data
		for _, part := range compiled[i] {
			m, ok := value.(map[string]any)
			if !ok {
				value = nil
				break
			}
			value = m[part]
		}
		result[path] = value
	}
	return result
}

func wantsField(selection, field string) bool {
	if selection == "" {
		return true
	}
	for _, path := range strings.Split(selection, ",") {
		if path == field || strings.HasPrefix(path, field+".") {
			return true
		}
	}
	return false
}

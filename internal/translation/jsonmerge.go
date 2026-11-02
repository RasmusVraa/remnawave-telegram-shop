package translation

import "encoding/json"

// DeepMergeJSON мержит overlay поверх base.
// Объекты — рекурсивно; массивы объектов с полем "id" — по id
// (элементы только из base остаются); остальные массивы и скаляры —
// побеждает overlay, если ключ есть.
func DeepMergeJSON(base, overlay []byte) ([]byte, error) {
	var baseVal, overVal any
	if err := json.Unmarshal(base, &baseVal); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(overlay, &overVal); err != nil {
		return nil, err
	}
	merged := mergeValues(baseVal, overVal)
	return json.Marshal(merged)
}

func mergeValues(base, overlay any) any {
	baseMap, baseIsMap := base.(map[string]any)
	overMap, overIsMap := overlay.(map[string]any)
	if baseIsMap && overIsMap {
		out := make(map[string]any, len(baseMap)+len(overMap))
		for k, v := range baseMap {
			out[k] = v
		}
		for k, v := range overMap {
			if existing, ok := out[k]; ok {
				out[k] = mergeValues(existing, v)
			} else {
				out[k] = v
			}
		}
		return out
	}

	baseArr, baseIsArr := base.([]any)
	overArr, overIsArr := overlay.([]any)
	if baseIsArr && overIsArr {
		return mergeArrays(baseArr, overArr)
	}
	return overlay
}

func mergeArrays(base, overlay []any) []any {
	if len(base) == 0 {
		return overlay
	}
	if len(overlay) == 0 {
		return base
	}
	if !arrayItemsHaveID(base) || !arrayItemsHaveID(overlay) {
		return overlay
	}
	index := make(map[string]int, len(base))
	out := make([]any, len(base))
	copy(out, base)
	for i, item := range base {
		m := item.(map[string]any)
		index[asString(m["id"])] = i
	}
	for _, item := range overlay {
		m := item.(map[string]any)
		id := asString(m["id"])
		if i, ok := index[id]; ok {
			out[i] = mergeValues(out[i], m)
			continue
		}
		index[id] = len(out)
		out = append(out, m)
	}
	return out
}

func arrayItemsHaveID(arr []any) bool {
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return false
		}
		if asString(m["id"]) == "" {
			return false
		}
	}
	return true
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

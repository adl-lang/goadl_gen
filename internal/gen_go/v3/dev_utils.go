package gen_go_v2

import "encoding/json"

func toJ(p string, v any) string {
	b, _ := json.MarshalIndent(v, p, "  ")
	return string(b)
}

package audit

// MaskArgs returns a new map containing only the keys listed in whitelist.
// Values are passed through unchanged — callers are responsible for ensuring
// that no listed field carries sensitive content.
func MaskArgs(args map[string]interface{}, whitelist []string) map[string]interface{} {
	out := make(map[string]interface{}, len(whitelist))
	for _, k := range whitelist {
		if v, ok := args[k]; ok {
			out[k] = v
		}
	}
	return out
}

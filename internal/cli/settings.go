package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
)

// hookCommandPrefix marks Lyceum's own hook in a harness settings file.
const hookCommandPrefix = "lyceum hook pre-tool"

// mergeHook returns the JSON settings file data with Lyceum's PreToolUse hook,
// matcher and command, in it (distribution.md, "lyceum init" step 4).
// Every other key, value and hook entry is kept, in its order. Lyceum's
// existing entry, found by its command, is replaced in place; any other is
// dropped, so a re-run gives the same bytes. Empty data is an empty object.
func mergeHook(data []byte, matcher, command string) ([]byte, error) {
	top := jsonObject{}
	if len(bytes.TrimSpace(data)) > 0 {
		var err error
		if top, err = parseObject(data); err != nil {
			return nil, err
		}
	}
	hooks := jsonObject{}
	if raw, ok := top.get("hooks"); ok {
		var err error
		if hooks, err = parseObject(raw); err != nil {
			return nil, errors.New(`"hooks" is not an object`)
		}
	}
	var entries []json.RawMessage
	if raw, ok := hooks.get("PreToolUse"); ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, errors.New(`"hooks"."PreToolUse" is not an array`)
		}
	}
	type hook struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}
	ours := rawJSON(struct {
		Matcher string `json:"matcher"`
		Hooks   []hook `json:"hooks"`
	}{matcher, []hook{{"command", command}}})

	var kept []json.RawMessage
	placed := false
	for _, e := range entries {
		entry, err := parseObject(e)
		if err != nil {
			kept = append(kept, e)
			continue
		}
		raw, _ := entry.get("hooks")
		var hs, others []json.RawMessage
		if json.Unmarshal(raw, &hs) != nil {
			kept = append(kept, e)
			continue
		}
		for _, h := range hs {
			if !lyceumHook(h) {
				others = append(others, h)
			}
		}
		if len(others) == len(hs) {
			kept = append(kept, e)
			continue
		}
		if !placed {
			kept, placed = append(kept, ours), true
		}
		if len(others) > 0 {
			entry.set("hooks", rawJSON(others))
			kept = append(kept, rawJSON(entry))
		}
	}
	if !placed {
		kept = append(kept, ours)
	}
	hooks.set("PreToolUse", rawJSON(kept))
	top.set("hooks", rawJSON(hooks))
	return encodeSettings(top)
}

// unmergeHook returns the JSON settings file data with Lyceum's PreToolUse
// hooks taken out, the inverse of mergeHook (distribution.md, "Uninstall"). Every other key, value and hook entry is kept, in its order. An entry
// left with no hooks goes, then a PreToolUse left empty, then a hooks left
// empty. It returns nil when nothing else is left.
func unmergeHook(data []byte) ([]byte, error) {
	top, err := parseObject(data)
	if err != nil {
		return nil, err
	}
	raw, ok := top.get("hooks")
	if !ok {
		return data, nil
	}
	hooks, err := parseObject(raw)
	if err != nil {
		return nil, errors.New(`"hooks" is not an object`)
	}
	if raw, ok = hooks.get("PreToolUse"); !ok {
		return data, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, errors.New(`"hooks"."PreToolUse" is not an array`)
	}
	var kept []json.RawMessage
	for _, e := range entries {
		entry, err := parseObject(e)
		if err != nil {
			kept = append(kept, e)
			continue
		}
		raw, _ := entry.get("hooks")
		var hs, others []json.RawMessage
		if json.Unmarshal(raw, &hs) != nil {
			kept = append(kept, e)
			continue
		}
		for _, h := range hs {
			if !lyceumHook(h) {
				others = append(others, h)
			}
		}
		switch {
		case len(others) == len(hs):
			kept = append(kept, e)
		case len(others) > 0:
			entry.set("hooks", rawJSON(others))
			kept = append(kept, rawJSON(entry))
		}
	}
	if len(kept) > 0 {
		hooks.set("PreToolUse", rawJSON(kept))
	} else {
		hooks.remove("PreToolUse")
	}
	if len(hooks) > 0 {
		top.set("hooks", rawJSON(hooks))
	} else {
		top.remove("hooks")
	}
	if len(top) == 0 {
		return nil, nil
	}
	return encodeSettings(top)
}

// encodeSettings is top as a settings file: indented, with <, > and & kept.
func encodeSettings(top jsonObject) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(top); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// lyceumHook reports whether hook entry h runs Lyceum's pre-tool hook.
func lyceumHook(h json.RawMessage) bool {
	var v struct {
		Command string `json:"command"`
	}
	return json.Unmarshal(h, &v) == nil && strings.Contains(v.Command, hookCommandPrefix)
}

// rawJSON is v encoded, with <, > and & kept as they are.
func rawJSON(v any) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err) // only values read from JSON, and fixed structs, are encoded
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// jsonObject is a JSON object that keeps its keys in order and each value's
// text as it was read.
type jsonObject []jsonMember

type jsonMember struct {
	key   string
	value json.RawMessage
}

// parseObject reads data, which must be exactly one JSON object.
func parseObject(data []byte) (jsonObject, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil {
		return nil, err
	} else if t != json.Delim('{') {
		return nil, errors.New("not a JSON object")
	}
	o := jsonObject{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		o = append(o, jsonMember{t.(string), v})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("more than one JSON value")
	}
	return o, nil
}

// get returns the value of key.
func (o jsonObject) get(key string) (json.RawMessage, bool) {
	for _, m := range o {
		if m.key == key {
			return m.value, true
		}
	}
	return nil, false
}

// set replaces the value of key, or adds key at the end.
func (o *jsonObject) set(key string, value json.RawMessage) {
	for i, m := range *o {
		if m.key == key {
			(*o)[i].value = value
			return
		}
	}
	*o = append(*o, jsonMember{key, value})
}

// remove deletes key.
func (o *jsonObject) remove(key string) {
	*o = slices.DeleteFunc(*o, func(m jsonMember) bool { return m.key == key })
}

// MarshalJSON writes the members in order.
func (o jsonObject) MarshalJSON() ([]byte, error) {
	b := []byte{'{'}
	for i, m := range o {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(append(append(b, rawJSON(m.key)...), ':'), m.value...)
	}
	return append(b, '}'), nil
}

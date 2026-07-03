package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

func TestConfigSchemaValidatesExample(t *testing.T) {
	schema := loadConfigSchema(t)
	doc := loadYAMLDocument(t, "examples/tea-dash.yml")

	if err := schema.Validate(doc); err != nil {
		t.Fatalf("schema should validate examples/tea-dash.yml: %v", err)
	}
}

func TestConfigSchemaRejectsUnknownRootField(t *testing.T) {
	schema := loadConfigSchema(t)
	doc := loadYAMLBytes(t, []byte(`
instance:
  login: ""
notARealTeaDashSetting: true
`))

	err := schema.Validate(doc)
	if err == nil {
		t.Fatal("schema should reject unknown root fields")
	}
	if !strings.Contains(err.Error(), "notARealTeaDashSetting") {
		t.Fatalf("schema error = %v, want it to mention the unknown field", err)
	}
}

func TestConfigSchemaCoversDocumentedTopLevelKeys(t *testing.T) {
	raw, err := os.ReadFile("schema.json")
	if err != nil {
		t.Fatalf("read schema.json: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema.json should be valid JSON: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %#v, want object", schema["properties"])
	}
	for _, key := range []string{
		"instance",
		"smartFilteringAtLaunch",
		"confirmQuit",
		"defaults",
		"repos",
		"localRepos",
		"pager",
		"repoPaths",
		"git",
		"theme",
		"prSections",
		"issuesSections",
		"notificationsSections",
		"actionsSections",
		"branchSections",
		"keybindings",
	} {
		if _, ok := props[key]; !ok {
			t.Fatalf("schema is missing top-level property %q", key)
		}
	}
}

func loadConfigSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	f, err := os.Open("schema.json")
	if err != nil {
		t.Fatalf("open schema.json: %v", err)
	}
	defer f.Close()

	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatalf("parse schema.json: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", doc); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		t.Fatalf("compile schema.json: %v", err)
	}
	return schema
}

func loadYAMLDocument(t *testing.T, path string) any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return loadYAMLBytes(t, raw)
}

func loadYAMLBytes(t *testing.T, raw []byte) any {
	t.Helper()
	var doc any
	if err := yaml.NewDecoder(bytes.NewReader(raw)).Decode(&doc); err != nil {
		t.Fatalf("parse YAML: %v", err)
	}
	return normalizeYAML(doc)
}

func normalizeYAML(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[k] = normalizeYAML(val)
		}
		return out
	case []any:
		for i := range v {
			v[i] = normalizeYAML(v[i])
		}
		return v
	default:
		return v
	}
}

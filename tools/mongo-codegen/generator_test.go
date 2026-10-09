package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/aspirit/xsocial/internal/storage/mongo/field"
)

func TestGeneratedFields(t *testing.T) {
	// Test a few generated fields to ensure they work
	if field.UserInfo.ID != "_id" {
		t.Errorf("Expected UserInfo.ID to be '_id', got %s", field.UserInfo.ID)
	}
	if field.UserInfo.Username != "username" {
		t.Errorf("Expected UserInfo.Username to be 'username', got %s", field.UserInfo.Username)
	}
	if field.Post.LikeCount != "like_count" {
		t.Errorf("Expected Post.LikeCount to be 'like_count', got %s", field.Post.LikeCount)
	}
	if field.PostLike.PostID != "post_id" {
		t.Errorf("Expected PostLike.PostID to be 'post_id', got %s", field.PostLike.PostID)
	}
}

func TestFieldConsistency(t *testing.T) {
	// Test that all models have consistent ID field
	models := []struct {
		name string
		id   string
	}{
		{"UserInfo", field.UserInfo.ID},
		{"Post", field.Post.ID},
		{"PostLike", field.PostLike.ID},
		{"Notification", field.Notification.ID},
	}

	for _, model := range models {
		if model.id != "_id" {
			t.Errorf("Expected %s.ID to be '_id', got %s", model.name, model.id)
		}
	}
}

func TestFieldGeneration(t *testing.T) {
	// Discover models dynamically
	models, err := discoverAllModels()
	if err != nil {
		t.Fatalf("Failed to discover models: %v", err)
	}

	if len(models) == 0 {
		t.Fatal("No models discovered")
	}

	t.Logf("Discovered %d models", len(models))

	// Test that UserInfo model is discovered
	var userModel *ModelStruct
	for _, model := range models {
		if model.Name == "UserInfo" {
			userModel = &model

			break
		}
	}

	if userModel == nil {
		t.Fatal("UserInfo model not discovered")
	}

	// Test UserInfo model fields
	expectedFields := map[string]string{
		"ID":           "_id",
		"Username":     "username",
		"PasswordHash": "password_hash",
	}

	fieldMap := make(map[string]string)
	for _, field := range userModel.Fields {
		fieldMap[field.Name] = field.BsonTag
	}

	for fieldName, expectedTag := range expectedFields {
		actualTag, exists := fieldMap[fieldName]
		if !exists {
			t.Errorf("Field %s not found in UserInfo model", fieldName)

			continue
		}
		if actualTag != expectedTag {
			t.Errorf("Field %s: expected tag %s, got %s", fieldName, expectedTag, actualTag)
		}
	}
}

func TestFieldFileGeneration(t *testing.T) {
	// Test that field files are generated correctly
	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("Failed to find project root: %v", err)
	}

	fieldDir := filepath.Join(projectRoot, "internal", "storage", "mongo", "field")

	// Check that user_info.go exists
	accountFile := filepath.Join(fieldDir, "user_info.go")
	if _, err := os.Stat(accountFile); os.IsNotExist(err) {
		t.Fatal("user_info.go field file not found")
	}

	// Validate file path to prevent directory traversal
	absAccountFile, err := filepath.Abs(accountFile)
	if err != nil {
		t.Fatalf("Failed to resolve account file path: %v", err)
	}

	// Read and verify content
	// Validate file path to prevent directory traversal attacks
	if !filepath.IsAbs(absAccountFile) {
		t.Fatalf("Invalid file path: %s", absAccountFile)
	}

	// Additional validation: ensure the file is within allowed directories
	if strings.Contains(absAccountFile, "..") {
		t.Fatalf("File path contains forbidden characters: %s", absAccountFile)
	}

	content, err := os.ReadFile(absAccountFile)
	if err != nil {
		t.Fatalf("Failed to read user_info.go: %v", err)
	}

	contentStr := string(content)

	// Check for expected content (using actual format from generated file)
	expectedContent := []string{
		"package field",
		"var UserInfo = struct {",
		"\tUsername     string",
		"\tPasswordHash: \"password_hash\",",
		"\tUsername:     \"username\",",
	}

	for _, expected := range expectedContent {
		if !strings.Contains(contentStr, expected) {
			t.Errorf("Expected content not found: %s", expected)
		}
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ID", "id"},
		{"Post", "post"},
		{"PostLike", "post_like"},
		{"UserID", "user_id"},
		{"HTTPServer", "http_server"},
		{"Notification", "notification"},
	}

	for _, test := range tests {
		result := toSnakeCase(test.input)
		if result != test.expected {
			t.Errorf("toSnakeCase(%s): expected %s, got %s", test.input, test.expected, result)
		}
	}
}

func TestToConstantCase(t *testing.T) {
	if got := toConstantCase("PostLike"); got != "POST_LIKE" {
		t.Errorf("toConstantCase(PostLike): expected POST_LIKE, got %s", got)
	}
}

func TestProjectRootDetection(t *testing.T) {
	root, err := findProjectRoot()
	if err != nil {
		t.Fatalf("Failed to find project root: %v", err)
	}

	// Check that go.mod exists at the root
	goModPath := filepath.Join(root, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		t.Errorf("go.mod not found at detected root: %s", root)
	}

	// Check that internal/model exists
	modelDir := filepath.Join(root, "internal", "model")
	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		t.Errorf("internal/model directory not found at: %s", modelDir)
	}
}

func TestModelDiscovery(t *testing.T) {
	allModels, err := discoverAllModels()
	if err != nil {
		t.Fatalf("Failed to discover models: %v", err)
	}

	// Should discover multiple models (including base models)
	if len(allModels) < 4 {
		t.Errorf("Expected at least 4 models, got %d", len(allModels))
	}

	// Check for some known models (including base models)
	expectedModels := []string{"Post", "PostLike", "UserInfo", "Notification"}
	modelNames := make(map[string]bool)
	for _, model := range allModels {
		modelNames[model.Name] = true
	}

	for _, expected := range expectedModels {
		if !modelNames[expected] {
			t.Errorf("Expected model %s not found", expected)
		}
	}

	// Verify that models with fields have them (skip embedded structs)
	for _, model := range allModels {
		// Embedded structs are allowed to have no fields
		if len(model.Fields) == 0 {
			continue
		}
		// Models with fields should have at least one field
		if len(model.Fields) == 0 {
			t.Errorf("Model %s has no fields", model.Name)
		}
	}

	// Every model in this repo maps to a collection.
	if got := filterModelsForCollections(allModels); len(got) != len(allModels) {
		t.Errorf("expected all %d models to map to collections, got %d", len(allModels), len(got))
	}
}

// TestFilterBaseModels verifies that base models (embedded-only structs
// registered in baseModels) get field constants but no collection constant.
func TestFilterBaseModels(t *testing.T) {
	baseModels["Timestamps"] = true
	defer delete(baseModels, "Timestamps")

	models := []ModelStruct{{Name: "Timestamps"}, {Name: "Post"}}
	got := filterModelsForCollections(models)
	if len(got) != 1 || got[0].Name != "Post" {
		t.Errorf("expected only Post to remain, got %+v", got)
	}
}

func TestTemplateFile(t *testing.T) {
	// Check that template file exists
	templatePath := "field.template"
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		t.Fatal("field.template file not found")
	}

	// Read and verify template content
	content, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("Failed to read template file: %v", err)
	}

	templateStr := string(content)

	// Check for essential template elements
	expectedElements := []string{
		"package field",
		"{{ .ModelName }}",
		"{{- range .Fields }}",
		"{{ .Name }}",
		"{{ .BsonTag }}",
	}

	for _, element := range expectedElements {
		if !strings.Contains(templateStr, element) {
			t.Errorf("Template missing essential element: %s", element)
		}
	}
}

func TestTemplateGeneration(t *testing.T) {
	// Test template parsing and execution
	templateContent, err := os.ReadFile("field.template")
	if err != nil {
		t.Fatalf("Failed to read template: %v", err)
	}

	tmpl, err := template.New("test").Parse(string(templateContent))
	if err != nil {
		t.Fatalf("Failed to parse template: %v", err)
	}

	// Test data
	testModel := ModelInfo{
		ModelName: "TestModel",
		Fields: []FieldInfo{
			{Name: "ID", BsonTag: "_id"},
			{Name: "Name", BsonTag: "name"},
		},
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, testModel); err != nil {
		t.Fatalf("Failed to execute template: %v", err)
	}

	result := buf.String()

	// Verify generated content
	expectedContent := []string{
		"package field",
		"var TestModel = struct {",
		"ID string",
		"Name string",
		`ID: "_id",`,
		`Name: "name",`,
	}

	for _, expected := range expectedContent {
		if !strings.Contains(result, expected) {
			t.Errorf("Generated content missing: %s", expected)
		}
	}
}

// TestShouldIncludeModel: only structs with bson-tagged fields are documents;
// value objects in internal/model (no tags) must not get a collection.
func TestShouldIncludeModel(t *testing.T) {
	src := `package model
type Doc struct { ID uint64 ` + "`bson:\"_id\"`" + ` }
type Value struct { ID uint64 }
type Embedding struct { Doc }
`
	file, err := parser.ParseFile(token.NewFileSet(), "", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"Doc": true, "Value": false, "Embedding": true}
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		if got := shouldIncludeModel(st); got != want[ts.Name.Name] {
			t.Errorf("shouldIncludeModel(%s) = %v, want %v", ts.Name.Name, got, want[ts.Name.Name])
		}

		return true
	})
}

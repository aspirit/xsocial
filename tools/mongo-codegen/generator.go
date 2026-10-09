// Package main provides a code generator for MongoDB field constants and collection constants.
// It scans Go model files and generates corresponding field and collection constant files.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

// Generator-specific errors.
var (
	ErrGoModNotFound    = errors.New("go.mod not found")
	ErrNoFieldsFound    = errors.New("no fields found for model")
	ErrTemplateNotFound = errors.New("template file not found")
	ErrInvalidFilePath  = errors.New("invalid file path")
	ErrForbiddenPath    = errors.New("path contains forbidden characters")
)

type ModelInfo struct {
	ModelName string
	Fields    []FieldInfo
}

type FieldInfo struct {
	Name    string
	BsonTag string
}

// CollectionInfo represents collection data for template generation.
type CollectionInfo struct {
	Models []CollectionModel
}

type CollectionModel struct {
	ModelName      string
	CollectionName string
	ConstantName   string
}

func main() {
	fmt.Println("🚀 Starting MongoDB code generation...")

	allModels, err := discoverAllModels()
	if err != nil {
		log.Fatalf("Failed to discover models: %v", err)
	}

	fmt.Printf("📁 Found %d models in internal/model/\n", len(allModels))

	// Generate field constants for every model with fields (base models included)
	// Base models need field constants for embedded structs to use
	for _, model := range allModels {
		if len(model.Fields) == 0 {
			fmt.Printf("⏭️  Skipped field constants for %s (embedded struct, uses parent fields)\n", model.Name)

			continue
		}

		if err := generateModelFromAST(model); err != nil {
			log.Printf("❌ Error generating model %s: %v", model.Name, err)
		} else {
			fmt.Printf("✅ Generated field constants for %s\n", model.Name)
		}
	}

	// Generate collection constants (filter out base models that don't correspond to tables)
	modelsForCollections := filterModelsForCollections(allModels)
	if err := generateCollectionConstants(modelsForCollections); err != nil {
		log.Printf("❌ Error generating collection constants: %v", err)
	} else {
		fmt.Printf("✅ Generated collection constants for %d models\n", len(modelsForCollections))
	}

	fmt.Println("✅ MongoDB code generation completed!")
}

// filterModelsForCollections filters out base models that don't correspond to database tables.
func filterModelsForCollections(allModels []ModelStruct) []ModelStruct {
	var filtered []ModelStruct
	for _, model := range allModels {
		if !shouldSkipModel(model.Name) {
			filtered = append(filtered, model)
		}
	}

	return filtered
}

type ModelStruct struct {
	Name   string
	Fields []ModelField
}

type ModelField struct {
	Name    string
	Type    string
	BsonTag string
}

// discoverAllModels discovers all models without filtering (including base models that need field constants).
func discoverAllModels() ([]ModelStruct, error) {
	// Get the project root directory
	projectRoot, err := findProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to find project root: %w", err)
	}

	modelDir := filepath.Join(projectRoot, "internal", "model")

	// Parse all Go files in the model directory
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, modelDir, func(info os.FileInfo) bool {
		return strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse model directory: %w", err)
	}

	var models []ModelStruct

	// Extract struct definitions from all packages (don't skip base models here)
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			structs := extractStructs(file, false) // false = don't skip base models
			models = append(models, structs...)
		}
	}

	// Sort models by name to ensure consistent order
	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})

	return models, nil
}

func findProjectRoot() (string, error) {
	// Start from current directory and walk up to find go.mod
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w", ErrGoModNotFound)
		}
		dir = parent
	}
}

// baseModels are structs that exist only to be embedded by other models. They
// get field constants (their fields appear in the embedding documents) but no
// collection constant, because no collection stores them directly. Register a
// model here before adding it to internal/model.
var baseModels = map[string]bool{}

// shouldSkipModel reports whether a model is a base model (no collection).
func shouldSkipModel(modelName string) bool {
	return baseModels[modelName]
}

// shouldIncludeModel reports whether a struct is a MongoDB document: it carries
// at least one bson-tagged field, directly or through an embedded base model.
// Plain value objects that also live in internal/model are left alone.
func shouldIncludeModel(structType *ast.StructType) bool {
	return hasBSONTaggedField(structType) || hasEmbeddedFields(structType)
}

func hasBSONTaggedField(structType *ast.StructType) bool {
	if structType.Fields == nil {
		return false
	}
	for _, field := range structType.Fields.List {
		if len(field.Names) > 0 && extractBsonTagFromAST(field.Tag) != "" {
			return true
		}
	}

	return false
}

func extractStructs(file *ast.File, skipBaseModels bool) []ModelStruct {
	var models []ModelStruct

	ast.Inspect(file, func(n ast.Node) bool {
		// Look for type declarations
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}

		// Check if it's a struct type
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		// Only process exported structs (capitalized names)
		if !ast.IsExported(typeSpec.Name.Name) {
			return true
		}

		// Skip base models only if skipBaseModels is true
		if skipBaseModels && shouldSkipModel(typeSpec.Name.Name) {
			return true
		}

		model := ModelStruct{
			Name:   typeSpec.Name.Name,
			Fields: extractFields(structType),
		}

		if shouldIncludeModel(structType) {
			models = append(models, model)
		}

		return true
	})

	return models
}

// hasEmbeddedFields checks if a struct has embedded fields (anonymous fields).
func hasEmbeddedFields(structType *ast.StructType) bool {
	if structType.Fields == nil {
		return false
	}

	for _, field := range structType.Fields.List {
		// Embedded fields have no names (len(field.Names) == 0)
		if len(field.Names) == 0 {
			return true
		}
	}

	return false
}

func extractFields(structType *ast.StructType) []ModelField {
	var fields []ModelField

	for _, field := range structType.Fields.List {
		// Skip unexported fields
		if len(field.Names) == 0 || !ast.IsExported(field.Names[0].Name) {
			continue
		}

		fieldName := field.Names[0].Name
		fieldType := getTypeString(field.Type)
		bsonTag := extractBsonTagFromAST(field.Tag)

		// Skip fields with bson:"-"
		if bsonTag == "-" {
			continue
		}

		// If no bson tag, use snake_case of field name
		if bsonTag == "" {
			bsonTag = toSnakeCase(fieldName)
		}

		fields = append(fields, ModelField{
			Name:    fieldName,
			Type:    fieldType,
			BsonTag: bsonTag,
		})
	}

	// Sort fields by name to ensure consistent order
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Name < fields[j].Name
	})

	return fields
}

func getTypeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + getTypeString(t.X)
	case *ast.ArrayType:
		return "[]" + getTypeString(t.Elt)
	case *ast.SelectorExpr:
		return getTypeString(t.X) + "." + t.Sel.Name
	default:
		return "any"
	}
}

func extractBsonTagFromAST(tag *ast.BasicLit) string {
	if tag == nil {
		return ""
	}

	// Remove backticks from tag
	tagValue := strings.Trim(tag.Value, "`")

	// Parse struct tag
	parts := strings.Fields(tagValue)
	for _, part := range parts {
		if strings.HasPrefix(part, "bson:") {
			// Extract bson tag value
			bsonPart := strings.TrimPrefix(part, "bson:")
			bsonPart = strings.Trim(bsonPart, `"`)

			// Get the field name (before first comma)
			if idx := strings.Index(bsonPart, ","); idx != -1 {
				return bsonPart[:idx]
			}

			return bsonPart
		}
	}

	return ""
}

func generateModelFromAST(model ModelStruct) error {
	modelInfo := ModelInfo{
		ModelName: model.Name,
	}

	for _, field := range model.Fields {
		fieldInfo := FieldInfo{
			Name:    field.Name,
			BsonTag: field.BsonTag,
		}
		modelInfo.Fields = append(modelInfo.Fields, fieldInfo)
	}

	if len(modelInfo.Fields) == 0 {
		return fmt.Errorf("%w: %s", ErrNoFieldsFound, modelInfo.ModelName)
	}

	return generateFile(modelInfo)
}

func generateFile(modelInfo ModelInfo) error {
	// Find template file path relative to project root or current directory
	templatePath, err := findTemplatePath("field.template")
	if err != nil {
		return fmt.Errorf("failed to find field template: %w", err)
	}

	// Validate template path to prevent directory traversal
	absTemplatePath, err := filepath.Abs(templatePath)
	if err != nil {
		return fmt.Errorf("failed to resolve template path: %w", err)
	}

	// Validate template path to prevent directory traversal attacks
	if !filepath.IsAbs(absTemplatePath) {
		return fmt.Errorf("%w: %s", ErrInvalidFilePath, absTemplatePath)
	}

	// Additional validation: ensure the file is within allowed directories
	if strings.Contains(absTemplatePath, "..") {
		return fmt.Errorf("%w: %s", ErrForbiddenPath, absTemplatePath)
	}

	templateContent, err := os.ReadFile(absTemplatePath)
	if err != nil {
		return fmt.Errorf("failed to read template file %s: %w", absTemplatePath, err)
	}

	// Parse template
	tmpl, err := template.New("field").Parse(string(templateContent))
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, modelInfo); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// Format the generated code
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format generated code: %w", err)
	}

	// Write to file
	outputPath := getOutputPath(modelInfo.ModelName)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(outputPath, formatted, 0o600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// findTemplatePath finds the template file in current directory or tools/mongo-codegen directory.
func findTemplatePath(templateName string) (string, error) {
	// Try current directory first
	if _, err := os.Stat(templateName); err == nil {
		return templateName, nil
	}

	// Try tools/mongo-codegen directory relative to project root
	projectRoot, err := findProjectRoot()
	if err != nil {
		return "", fmt.Errorf("failed to find project root: %w", err)
	}

	templatePath := filepath.Join(projectRoot, "tools", "mongo-codegen", templateName)
	if _, err := os.Stat(templatePath); err == nil {
		return templatePath, nil
	}

	// Try relative to current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	// If we're already in tools/mongo-codegen, try current directory
	if filepath.Base(cwd) == "mongo-codegen" && filepath.Base(filepath.Dir(cwd)) == "tools" {
		return templateName, nil
	}

	return "", fmt.Errorf("%w: %s", ErrTemplateNotFound, templateName)
}

func getOutputPath(modelName string) string {
	root, _ := findProjectRoot()
	fileName := toSnakeCase(modelName) + ".go"

	return filepath.Join(root, "internal", "storage", "mongo", "field", fileName)
}

// toSnakeCase converts PascalCase to snake_case, keeping acronyms together:
// UserID -> user_id, HTTPServer -> http_server, PostLike -> post_like.
func toSnakeCase(str string) string {
	runes := []rune(str)
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			startsWord := unicode.IsLower(prev) || unicode.IsDigit(prev) ||
				(unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]))
			if startsWord {
				b.WriteRune('_')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}

	return b.String()
}

// generateCollectionConstants generates the collection constants file.
func generateCollectionConstants(models []ModelStruct) error {
	collectionInfo := CollectionInfo{
		Models: make([]CollectionModel, 0, len(models)),
	}

	// Convert models to collection models
	for _, model := range models {
		collectionName := toSnakeCase(model.Name)
		constantName := toConstantCase(model.Name)

		collectionModel := CollectionModel{
			ModelName:      model.Name,
			CollectionName: collectionName,
			ConstantName:   constantName,
		}
		collectionInfo.Models = append(collectionInfo.Models, collectionModel)
	}

	// Sort models by name to ensure consistent order
	sort.Slice(collectionInfo.Models, func(i, j int) bool {
		return collectionInfo.Models[i].ModelName < collectionInfo.Models[j].ModelName
	})

	return generateCollectionFile(collectionInfo)
}

// generateCollectionFile generates the collection constants file from template.
func generateCollectionFile(collectionInfo CollectionInfo) error {
	// Find template file path relative to project root or current directory
	templatePath, err := findTemplatePath("collection.template")
	if err != nil {
		return fmt.Errorf("failed to find collection template: %w", err)
	}

	// Validate template path to prevent directory traversal
	absTemplatePath, err := filepath.Abs(templatePath)
	if err != nil {
		return fmt.Errorf("failed to resolve collection template path: %w", err)
	}

	// Validate template path to prevent directory traversal attacks
	if !filepath.IsAbs(absTemplatePath) {
		return fmt.Errorf("%w: %s", ErrInvalidFilePath, absTemplatePath)
	}

	// Additional validation: ensure the file is within allowed directories
	if strings.Contains(absTemplatePath, "..") {
		return fmt.Errorf("%w: %s", ErrForbiddenPath, absTemplatePath)
	}

	templateContent, err := os.ReadFile(absTemplatePath)
	if err != nil {
		return fmt.Errorf("failed to read collection template file %s: %w", absTemplatePath, err)
	}

	// Parse template
	tmpl, err := template.New("collection").Parse(string(templateContent))
	if err != nil {
		return fmt.Errorf("failed to parse collection template: %w", err)
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, collectionInfo); err != nil {
		return fmt.Errorf("failed to execute collection template: %w", err)
	}

	// Format the generated code
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format generated collection code: %w", err)
	}

	// Write to file
	outputPath := getCollectionOutputPath()
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return fmt.Errorf("failed to create collection directory: %w", err)
	}

	if err := os.WriteFile(outputPath, formatted, 0o600); err != nil {
		return fmt.Errorf("failed to write collection file: %w", err)
	}

	return nil
}

// getCollectionOutputPath returns the output path for the collection constants file.
func getCollectionOutputPath() string {
	root, _ := findProjectRoot()

	return filepath.Join(root, "internal", "storage", "mongo", "collections.go")
}

// toConstantCase converts PascalCase to CONSTANT_CASE (UserInfo -> USER_INFO).
func toConstantCase(str string) string {
	return strings.ToUpper(toSnakeCase(str))
}

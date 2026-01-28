package main

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestProject creates a temporary test project structure
func setupTestProject(t *testing.T) string {
	tmpDir := t.TempDir()

	// Create package.json
	packageJSON := filepath.Join(tmpDir, "package.json")
	if err := os.WriteFile(packageJSON, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	return tmpDir
}

// createFeatureFolder creates a feature folder with the required structure
func createFeatureFolder(t *testing.T, basePath, featureName string) {
	featurePath := filepath.Join(basePath, featureName)

	// Create required folders
	for _, folder := range requiredFolders {
		folderPath := filepath.Join(featurePath, folder)
		if err := os.MkdirAll(folderPath, 0755); err != nil {
			t.Fatalf("Failed to create folder %s: %v", folderPath, err)
		}

		// Create required files
		for filename := range requiredFiles {
			filePath := filepath.Join(folderPath, filename)
			if err := os.WriteFile(filePath, []byte(""), 0644); err != nil {
				t.Fatalf("Failed to create file %s: %v", filePath, err)
			}
		}
	}

	// Create main barrel export
	mainIndex := filepath.Join(featurePath, "index.ts")
	if err := os.WriteFile(mainIndex, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create main index: %v", err)
	}
}

func TestFindProjectRoot(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() string
		wantError bool
	}{
		{
			name: "finds project root with package.json",
			setup: func() string {
				tmpDir := t.TempDir()
				packageJSON := filepath.Join(tmpDir, "package.json")
				_ = os.WriteFile(packageJSON, []byte("{}"), 0644)
				subDir := filepath.Join(tmpDir, "sub", "dir")
				_ = os.MkdirAll(subDir, 0755)
				return subDir
			},
			wantError: false,
		},
		{
			name: "returns error when no package.json found",
			setup: func() string {
				tmpDir := t.TempDir()
				return tmpDir
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startPath := tt.setup()
			_, err := findProjectRoot(startPath)

			if tt.wantError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestCheckFeatureStructure(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(string)
		wantIssues int
	}{
		{
			name: "valid feature structure",
			setup: func(basePath string) {
				createFeatureFolder(t, basePath, "test-feature")
			},
			wantIssues: 0,
		},
		{
			name: "missing create folder",
			setup: func(basePath string) {
				featurePath := filepath.Join(basePath, "test-feature")
				_ = os.MkdirAll(featurePath, 0755)

				// Create all folders except 'create'
				for _, folder := range requiredFolders {
					if folder == "create" {
						continue
					}
					folderPath := filepath.Join(featurePath, folder)
					_ = os.MkdirAll(folderPath, 0755)

					for filename := range requiredFiles {
						filePath := filepath.Join(folderPath, filename)
						_ = os.WriteFile(filePath, []byte(""), 0644)
					}
				}

				// Create main index
				mainIndex := filepath.Join(featurePath, "index.ts")
				_ = os.WriteFile(mainIndex, []byte(""), 0644)
			},
			wantIssues: 1, // Missing create folder
		},
		{
			name: "missing index.ts in folder",
			setup: func(basePath string) {
				featurePath := filepath.Join(basePath, "test-feature")
				_ = os.MkdirAll(featurePath, 0755)

				// Create all folders but skip index.ts in first folder
				for i, folder := range requiredFolders {
					folderPath := filepath.Join(featurePath, folder)
					_ = os.MkdirAll(folderPath, 0755)

					for filename := range requiredFiles {
						if i == 0 && filename == "index.ts" {
							continue // Skip first index.ts
						}
						filePath := filepath.Join(folderPath, filename)
						_ = os.WriteFile(filePath, []byte(""), 0644)
					}
				}

				// Create main index
				mainIndex := filepath.Join(featurePath, "index.ts")
				_ = os.WriteFile(mainIndex, []byte(""), 0644)
			},
			wantIssues: 1, // Missing index.ts in create folder
		},
		{
			name: "missing main barrel export",
			setup: func(basePath string) {
				featurePath := filepath.Join(basePath, "test-feature")
				_ = os.MkdirAll(featurePath, 0755)

				// Create all folders with files but no main index
				for _, folder := range requiredFolders {
					folderPath := filepath.Join(featurePath, folder)
					_ = os.MkdirAll(folderPath, 0755)

					for filename := range requiredFiles {
						filePath := filepath.Join(folderPath, filename)
						_ = os.WriteFile(filePath, []byte(""), 0644)
					}
				}
			},
			wantIssues: 1, // Missing main index.ts
		},
		{
			name: "loose component file in feature root",
			setup: func(basePath string) {
				createFeatureFolder(t, basePath, "test-feature")

				// Add a loose component file
				looseFile := filepath.Join(basePath, "test-feature", "LooseComponent.tsx")
				_ = os.WriteFile(looseFile, []byte("export const Loose = () => null;"), 0644)
			},
			wantIssues: 1, // Loose component file
		},
		{
			name: "multiple issues",
			setup: func(basePath string) {
				featurePath := filepath.Join(basePath, "test-feature")
				_ = os.MkdirAll(featurePath, 0755)

				// Create only one folder (missing others)
				folderPath := filepath.Join(featurePath, "create")
				_ = os.MkdirAll(folderPath, 0755)

				// Add loose component
				looseFile := filepath.Join(featurePath, "Loose.tsx")
				_ = os.WriteFile(looseFile, []byte(""), 0644)
			},
			wantIssues: 11, // 2 missing files in create folder + 7 missing folders + 1 missing main index + 1 loose file
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(tmpDir)

			featurePath := filepath.Join(tmpDir, "test-feature")
			issues := checkFeatureStructure(featurePath)

			if len(issues) != tt.wantIssues {
				t.Errorf("Expected %d issues, got %d: %v", tt.wantIssues, len(issues), issues)
			}
		})
	}
}

func TestCheckNoLooseComponents(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(string)
		wantIssues int
	}{
		{
			name: "no loose components",
			setup: func(basePath string) {
				componentsDir := filepath.Join(basePath, "components")
				_ = os.MkdirAll(componentsDir, 0755)

				// Create feature folder (not loose)
				featureDir := filepath.Join(componentsDir, "feature")
				_ = os.MkdirAll(featureDir, 0755)
				_ = os.WriteFile(filepath.Join(featureDir, "Component.tsx"), []byte(""), 0644)
			},
			wantIssues: 0,
		},
		{
			name: "loose tsx file in components root",
			setup: func(basePath string) {
				componentsDir := filepath.Join(basePath, "components")
				_ = os.MkdirAll(componentsDir, 0755)

				// Create loose component
				looseFile := filepath.Join(componentsDir, "LooseComponent.tsx")
				_ = os.WriteFile(looseFile, []byte(""), 0644)
			},
			wantIssues: 1,
		},
		{
			name: "index.ts is allowed in components root",
			setup: func(basePath string) {
				componentsDir := filepath.Join(basePath, "components")
				_ = os.MkdirAll(componentsDir, 0755)

				// Create index.ts (allowed)
				indexFile := filepath.Join(componentsDir, "index.ts")
				_ = os.WriteFile(indexFile, []byte(""), 0644)
			},
			wantIssues: 0,
		},
		{
			name: "components directory doesn't exist",
			setup: func(basePath string) {
				// Don't create components directory
			},
			wantIssues: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(tmpDir)

			componentsDir := filepath.Join(tmpDir, "components")
			issues := checkNoLooseComponents(componentsDir)

			if len(issues) != tt.wantIssues {
				t.Errorf("Expected %d issues, got %d: %v", tt.wantIssues, len(issues), issues)
			}
		})
	}
}

func TestValidateStructure(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(string)
		wantIssues int
	}{
		{
			name: "valid structure with routes and shared",
			setup: func(basePath string) {
				componentsDir := filepath.Join(basePath, "components")
				routesDir := filepath.Join(componentsDir, "routes")

				createFeatureFolder(t, routesDir, "feature1")
				createFeatureFolder(t, routesDir, "feature2")
				createFeatureFolder(t, componentsDir, "shared")
			},
			wantIssues: 0,
		},
		{
			name: "apps/web/components structure",
			setup: func(basePath string) {
				componentsDir := filepath.Join(basePath, "apps", "web", "components")
				routesDir := filepath.Join(componentsDir, "routes")

				createFeatureFolder(t, routesDir, "feature1")
			},
			wantIssues: 0,
		},
		{
			name: "no components directory",
			setup: func(basePath string) {
				// Don't create components directory
			},
			wantIssues: 0, // Should return no issues if no components dir
		},
		{
			name: "invalid feature in routes",
			setup: func(basePath string) {
				componentsDir := filepath.Join(basePath, "components")
				routesDir := filepath.Join(componentsDir, "routes")
				featurePath := filepath.Join(routesDir, "bad-feature")

				// Create incomplete feature (missing folders)
				_ = os.MkdirAll(featurePath, 0755)
			},
			wantIssues: 9, // 8 missing folders + 1 main index (files not checked when folder is missing)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := setupTestProject(t)
			tt.setup(tmpDir)

			issues := validateStructure(tmpDir)

			if len(issues) != tt.wantIssues {
				t.Errorf("Expected %d issues, got %d: %v", tt.wantIssues, len(issues), issues)
			}
		})
	}
}

func TestIsStructureModifyingOperation(t *testing.T) {
	tests := []struct {
		name     string
		data     ToolUseData
		expected bool
	}{
		{
			name: "Write operation in components",
			data: ToolUseData{
				ToolName: "Write",
				ToolInput: map[string]interface{}{
					"file_path": "/path/to/components/Feature.tsx",
				},
			},
			expected: true,
		},
		{
			name: "Edit operation in components",
			data: ToolUseData{
				ToolName: "Edit",
				ToolInput: map[string]interface{}{
					"file_path": "/path/to/components/Feature.ts",
				},
			},
			expected: true,
		},
		{
			name: "Bash mkdir in components",
			data: ToolUseData{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "mkdir -p /path/to/components/routes/feature",
				},
			},
			expected: true,
		},
		{
			name: "Bash rm in components (should be false)",
			data: ToolUseData{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "rm -rf /path/to/components/routes/feature",
				},
			},
			expected: false,
		},
		{
			name: "Read operation (should be false)",
			data: ToolUseData{
				ToolName: "Read",
				ToolInput: map[string]interface{}{
					"file_path": "/path/to/components/Feature.tsx",
				},
			},
			expected: false,
		},
		{
			name: "Write outside components (should be false)",
			data: ToolUseData{
				ToolName: "Write",
				ToolInput: map[string]interface{}{
					"file_path": "/path/to/other/Feature.tsx",
				},
			},
			expected: false,
		},
		{
			name: "Write non-tsx/ts file (should be false)",
			data: ToolUseData{
				ToolName: "Write",
				ToolInput: map[string]interface{}{
					"file_path": "/path/to/components/config.json",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isStructureModifyingOperation(tt.data)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestLoadProjectConfig(t *testing.T) {
	// Save original working directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWd)
	}()

	tests := []struct {
		name    string
		setup   func(string)
		wantErr bool
	}{
		{
			name: "no config file",
			setup: func(tmpDir string) {
				// No config file created
			},
			wantErr: false,
		},
		{
			name: "valid config file",
			setup: func(tmpDir string) {
				configFile := filepath.Join(tmpDir, ".claude-hooks-config.sh")
				content := "export TEST_VAR=test_value\n"
				_ = os.WriteFile(configFile, []byte(content), 0644)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(tmpDir)

			// Change to temp directory
			if err := os.Chdir(tmpDir); err != nil {
				t.Fatalf("Failed to change directory: %v", err)
			}

			err := loadProjectConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("loadProjectConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
